// Package proxy is the HLS proxy: source CDNs expect a browser-like
// User-Agent and a specific Referer, and browsers cannot call them
// cross-origin, so playlist and segment traffic flows through here.
// Playlists are rewritten so every nested URL comes back through the proxy.
package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Handler returns the /api/proxy handler. Query params: u (target URL),
// r (Referer to present to the origin, optional).
func Handler() http.Handler {
	client := &http.Client{Timeout: 30 * time.Second}
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

		ct := resp.Header.Get("Content-Type")
		isPlaylist := strings.Contains(ct, "mpegurl") || strings.HasSuffix(parsed.Path, ".m3u8")
		if isPlaylist && resp.StatusCode == http.StatusOK {
			body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			if err != nil {
				http.Error(w, "gagal membaca playlist", http.StatusBadGateway)
				return
			}
			rewritten := rewritePlaylist(string(body), parsed, referer)
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, rewritten)
			return
		}
		for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	})
}

// proxyURL wraps an absolute target URL in a call back to this proxy.
func proxyURL(abs, referer string) string {
	q := url.Values{"u": {abs}}
	if referer != "" {
		q.Set("r", referer)
	}
	return "/api/proxy?" + q.Encode()
}

// rewritePlaylist rewrites segment and nested playlist URLs in an m3u8 body
// so the player keeps talking to the proxy (which adds the right headers).
func rewritePlaylist(body string, base *url.URL, referer string) string {
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
			out.WriteString(rewriteTagURI(trimmed, base, referer))
			out.WriteByte('\n')
		default:
			abs := resolve(base, trimmed)
			out.WriteString(proxyURL(abs, referer))
			out.WriteByte('\n')
		}
	}
	return out.String()
}

// rewriteTagURI rewrites URI="..." attributes inside playlist tags
// (encryption keys, subtitle tracks, rendition reports).
func rewriteTagURI(line string, base *url.URL, referer string) string {
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
	return line[:start] + proxyURL(resolve(base, raw), referer) + line[start+end:]
}

func resolve(base *url.URL, ref string) string {
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return base.ResolveReference(r).String()
}

var _ = fmt.Sprintf // keep fmt for future diagnostics
