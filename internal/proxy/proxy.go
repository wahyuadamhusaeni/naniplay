// Package proxy is the HLS proxy: source CDNs expect a browser-like
// User-Agent and a specific Referer, and browsers cannot call them
// cross-origin, so playlist and segment traffic flows through here.
// Playlists are rewritten so every nested URL comes back through the proxy.
package proxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// blockedIP reports whether an address must never be dialed by the
// proxy: loopback, private, link-local, multicast, or unspecified.
// Without this guard the proxy is an open SSRF door — anyone who can
// reach the instance could make the server fetch internal services.
func blockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

// guardedTransport resolves the host itself and refuses to dial
// blocked addresses, so the check also covers HTTP redirects (every
// redirect re-dials through here) and cannot be skipped by a hostname
// that resolves to an internal IP.
func guardedTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 12 * time.Second}
	return &http.Transport{
		ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if blockedIP(ip) {
					return nil, fmt.Errorf("alamat tujuan diblokir: %s", host)
				}
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("host tidak ditemukan: %s", host)
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
		},
		TLSHandshakeTimeout: 12 * time.Second,
	}
}

// GuardedClient returns an HTTP client that refuses to dial
// internal addresses (see guardedTransport). For server-side fetches
// of URLs that are not fully trusted.
func GuardedClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: guardedTransport()}
}

// Handler returns the /api/proxy handler. Query params: u (target URL),
// r (Referer to present to the origin, optional).
func Handler() http.Handler {
	client := &http.Client{Timeout: 30 * time.Second, Transport: guardedTransport()}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := r.URL.Query().Get("u")
		referer := r.URL.Query().Get("r")
		if target == "" {
			http.Error(w, "parameter u wajib ada", http.StatusBadRequest)
			return
		}
		parsed, err := url.Parse(target)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			http.Error(w, "url tidak valid", http.StatusBadRequest)
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			http.Error(w, "gagal membuat request", http.StatusBadRequest)
			return
		}
		req.Header.Set("User-Agent", browserUA)
		if referer != "" {
			req.Header.Set("Referer", referer)
		}
		if rng := r.Header.Get("Range"); rng != "" {
			req.Header.Set("Range", rng)
		}
		resp, err := client.Do(req)
		if err != nil {
			http.Error(w, "gagal menghubungi asal stream", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		propagate := r.URL.Query().Get("pq") == "1"
		ct := resp.Header.Get("Content-Type")
		isPlaylist := strings.Contains(ct, "mpegurl") || strings.HasSuffix(parsed.Path, ".m3u8")
		var sniffed io.Reader
		if resp.StatusCode == http.StatusOK {
			// Some packagers serve playlists under non-playlist names
			// (e.g. config-*.json); sniff the body prefix as well.
			br := bufio.NewReader(resp.Body)
			if head, err := br.Peek(7); err == nil && string(head) == "#EXTM3U" {
				isPlaylist = true
			}
			sniffed = br
		}
		if isPlaylist && resp.StatusCode == http.StatusOK {
			src := sniffed
			if src == nil {
				src = resp.Body
			}
			body, err := io.ReadAll(io.LimitReader(src, 8<<20))
			if err != nil {
				http.Error(w, "gagal membaca playlist", http.StatusBadGateway)
				return
			}
			rewritten := rewritePlaylistQ(string(body), parsed, referer, propagate)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, rewritten)
			return
		}
		// Some source CDNs disguise MPEG-TS segments behind a fixed
		// 252-byte junk prefix. Detect that exact shape (first byte is
		// not the TS sync byte, but byte 252 is) and strip the prefix;
		// anything else streams through untouched.
		var body io.Reader = resp.Body
		if sniffed != nil {
			body = sniffed
		}
		strip := 0
		if r.Header.Get("Range") == "" && resp.StatusCode == http.StatusOK {
			br := bufio.NewReader(body)
			if head, err := br.Peek(253); err == nil && len(head) >= 253 && head[0] != 0x47 && head[252] == 0x47 {
				strip = 252
			}
			body = br
		}
		for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if strip > 0 && h == "Content-Length" {
				continue // length changes after stripping
			}
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(resp.StatusCode)
		if strip > 0 {
			io.CopyN(io.Discard, body, int64(strip))
		}
		io.Copy(w, body)
	})
}

// proxyURL wraps an absolute target URL in a call back to this proxy.
func proxyURL(abs, referer string) string {
	return proxyURLQ(abs, referer, false)
}

// proxyURLQ additionally keeps the pq flag when propagate is set:
// some packagers (Shaka) authorize every child request with the
// master URL's query token, so children without their own query
// inherit the parent's.
func proxyURLQ(abs, referer string, propagate bool) string {
	q := url.Values{"u": {abs}}
	if referer != "" {
		q.Set("r", referer)
	}
	if propagate {
		q.Set("pq", "1")
	}
	return "/api/proxy?" + q.Encode()
}

// rewritePlaylist rewrites segment and nested playlist URLs in an m3u8 body
// so the player keeps talking to the proxy (which adds the right headers).
func rewritePlaylist(body string, base *url.URL, referer string) string {
	return rewritePlaylistQ(body, base, referer, false)
}

func rewritePlaylistQ(body string, base *url.URL, referer string, propagate bool) string {
	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// keep blank lines out
		case strings.HasPrefix(trimmed, "#"):
			out.WriteString(rewriteTagURIQ(trimmed, base, referer, propagate))
			out.WriteByte('\n')
		default:
			abs := resolveQ(base, trimmed, propagate)
			out.WriteString(proxyURLQ(abs, referer, propagate))
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// rewriteTagURI rewrites URI="..." attributes inside playlist tags
// (encryption keys, subtitle tracks, rendition reports).
func rewriteTagURI(line string, base *url.URL, referer string) string {
	return rewriteTagURIQ(line, base, referer, false)
}

func rewriteTagURIQ(line string, base *url.URL, referer string, propagate bool) string {
	const marker = `URI="`
	i := strings.Index(line, marker)
	if i < 0 {
		return line
	}
	start := i + len(marker)
	end := strings.Index(line[start:], `"`)
	if end < 0 {
		return line
	}
	raw := line[start : start+end]
	if strings.HasPrefix(raw, "/api/proxy?") {
		return line
	}
	return line[:start] + proxyURLQ(resolveQ(base, raw, propagate), referer, propagate) + line[start+end:]
}

func resolve(base *url.URL, ref string) string {
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(r).String()
}

// resolveQ resolves ref against base; with propagate, a child URL
// without its own query inherits the parent's query token.
func resolveQ(base *url.URL, ref string, propagate bool) string {
	abs := resolve(base, ref)
	if !propagate || base.RawQuery == "" {
		return abs
	}
	u, err := url.Parse(abs)
	if err != nil || u.RawQuery != "" {
		return abs
	}
	u.RawQuery = base.RawQuery
	return u.String()
}

var _ = fmt.Sprintf // keep fmt for future diagnostics
