package web

// Mode embed terproksi untuk Film Indo (v1.9.0) — port Go dari
// pendekatan yang terbukti di proyek Astro lama Adam (elka21,
// src/pages/player.ts). Embed JuicyCodes terkunci referer ke domain
// sumber dan CDN-nya terkunci CORS ke origin embed, jadi iframe
// langsung selalu 403. Yang bekerja: dokumen embed diambil server
// (referer sumber dipalsukan — server bisa), URL aset & upstream di
// HTML ditulis ulang ke proxy ini, dan sebuah skrip penambal
// (netPatch) menyadap fetch + XMLHttpRequest di dalam halaman embed
// sehingga SEMUA lalu lintas player (API, playlist, segmen) ikut
// lewat proxy dengan referer embed yang benar. JS player asli yang
// berjalan; identitas jaringannya konsisten di server.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/proxy"
)

var (
	embedAttrRe = regexp.MustCompile(`(?i)(src|href|poster|data-src)=("[^"]*"|'[^']*')`)
	embedAbsRe  = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// upstreamHost reports whether a host belongs to the player/CDN
// families whose traffic must go through the embed proxy.
func upstreamHost(host string) bool {
	h := strings.ToLower(host)
	if ip := net.ParseIP(h); ip != nil {
		return true
	}
	for _, frag := range []string{"groovy", "monster", "abyssplayer", "iamcdn", "playcdn", "mfw09", "videonode", "turbonewvid"} {
		if strings.Contains(h, frag) {
			return true
		}
	}
	return strings.HasSuffix(h, ".xyz")
}

// publicEmbedHost rejects private/loopback targets (SSRF guard in
// the spirit of the original's isPublicHost).
func publicEmbedHost(host string) bool {
	h := strings.ToLower(host)
	if h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".lan") {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return !(ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
	}
	return true
}

func embedRawURL(src, ref string) string {
	return "/api/indo-embed-raw?src=" + url.QueryEscape(src) + "&ref=" + url.QueryEscape(ref)
}

// indoEmbedDoc serves one JuicyCodes embed document, rewritten so
// its assets and runtime traffic flow through indoEmbedRaw, with
// the fetch/XHR patch injected.
func (s *Server) indoEmbedDoc(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !publicEmbedHost(u.Hostname()) {
		http.Error(w, "URL embed tidak diizinkan", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	html, err := s.kita.FetchEmbedDoc(ctx, raw)
	if err != nil {
		http.Error(w, "Embed tidak terjangkau: "+err.Error(), http.StatusBadGateway)
		return
	}
	origin := u.Scheme + "://" + u.Host
	prox := func(abs string) string { return embedRawURL(abs, raw) }
	// Atribut relatif → absolut dari origin embed, lalu diproksi.
	html = embedAttrRe.ReplaceAllStringFunc(html, func(m string) string {
		i := strings.Index(m, "=")
		attr, quoted := m[:i+1], m[i+1:]
		val := quoted[1 : len(quoted)-1]
		if !strings.HasPrefix(val, "/") || strings.HasPrefix(val, "//") {
			return m
		}
		return attr + quoted[:1] + prox(origin+val) + quoted[:1]
	})
	// URL absolut ke host upstream/IP → diproksi.
	html = embedAbsRe.ReplaceAllStringFunc(html, func(m string) string {
		cu, err := url.Parse(m)
		if err != nil || !upstreamHost(cu.Hostname()) {
			return m
		}
		return prox(m)
	})
	patch := embedNetPatch(origin)
	if strings.Contains(html, "</head>") {
		html = strings.Replace(html, "</head>", patch+"</head>", 1)
	} else {
		html = patch + html
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, html)
}

// embedNetPatch builds the fetch/XHR interception script. Relative
// URLs resolve against the embed origin; ad/tracker hosts are
// denied (their requests fall through unproxied and die quietly).
func embedNetPatch(origin string) string {
	eb, _ := json.Marshal(origin)
	return fmt.Sprintf(`<script>(function(){var P="/api/indo-embed-raw";var EB=%s;var DENY=/googletagmanager|google-analytics|facebook\.net|popads|propellerads|adsterra|exoclick|popcash/i;function abs(u){try{return new URL(u,EB).href;}catch(e){return u;}}function prox(f){return P+"?src="+encodeURIComponent(f)+"&ref="+encodeURIComponent(EB);}var _f=window.fetch;function myfetch(u,o){var s;try{s=typeof u==="string"?u:(u&&u.url);}catch(e){return _f(u,o);}try{if(typeof s==="string"){if(!/^https?:/i.test(s))s=abs(s);if(!DENY.test(s))return _f(prox(s),o);}}catch(e){}return _f(u,o);}window.fetch=myfetch;try{window.fetch.toString=function(){return "function fetch() { [native code] }";};}catch(e){}var _o=XMLHttpRequest.prototype.open;function myopen(m,u){try{if(!/^https?:/i.test(u))u=abs(u);if(!DENY.test(u))u=prox(u);}catch(e){}return _o.call(this,m,u);}XMLHttpRequest.prototype.open=myopen;try{XMLHttpRequest.prototype.open.toString=function(){return "function open() { [native code] }";};}catch(e){}})();</script>`, eb)
}

// indoEmbedRaw proxies one upstream resource for the embed player:
// the fetch the patched page would have made directly, done by the
// server with the embed page as referer. Playlists are rewritten so
// their children come back through here.
func (s *Server) indoEmbedRaw(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("src")
	ref := r.URL.Query().Get("ref")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !publicEmbedHost(u.Hostname()) {
		http.Error(w, "URL tidak diizinkan", http.StatusBadRequest)
		return
	}
	referer := ref
	if pu, err := url.Parse(ref); err != nil || (pu.Scheme != "https" && pu.Scheme != "http") {
		referer = s.kita.Base() + "/"
	}
	// Ambil upstream persis seperti route player elka21 yang terbukti:
	// klien polos (tanpa sidik jari TLS khusus), UA Chrome/120, dan
	// hanya Referer. Tumpukan header browser penuh + Origin yang
	// dipakai mode native justru ditolak gerbang CDN untuk rantai ini.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		http.Error(w, "URL tidak valid", http.StatusBadRequest)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", referer)
	if rng := r.Header.Get("Range"); rng != "" {
		req.Header.Set("Range", rng)
	}
	resp, err := proxy.GuardedClient(60 * time.Second).Do(req)
	if err != nil {
		http.Error(w, "Upstream tidak terjangkau: "+err.Error(), http.StatusBadGateway)
		return
	}
	status, ctype, body := resp.StatusCode, resp.Header.Get("Content-Type"), resp.Body
	defer body.Close()
	br := bufio.NewReader(body)
	isPlaylist := strings.Contains(ctype, "mpegurl")
	if !isPlaylist {
		if head, err := br.Peek(7); err == nil && string(head) == "#EXTM3U" {
			isPlaylist = true
		}
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if isPlaylist {
		b, err := io.ReadAll(io.LimitReader(br, 4<<20))
		if err != nil {
			http.Error(w, "Playlist tidak terbaca", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(rewriteEmbedPlaylist(string(b), raw, referer)))
		return
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, br)
}

// rewriteEmbedPlaylist resolves a playlist's children against its
// own URL and wraps them through indoEmbedRaw (absolute URLs too —
// groovy playlists mix both).
func rewriteEmbedPlaylist(body, base, referer string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return body
	}
	wrap := func(child string) string {
		cu, err := url.Parse(child)
		if err != nil {
			return child
		}
		return embedRawURL(baseURL.ResolveReference(cu).String(), referer)
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "#") {
			if idx := strings.Index(trim, `URI="`); idx >= 0 {
				rest := trim[idx+5:]
				if end := strings.Index(rest, `"`); end >= 0 {
					lines[i] = strings.Replace(line, `URI="`+rest[:end]+`"`, `URI="`+wrap(rest[:end])+`"`, 1)
				}
			}
			continue
		}
		lines[i] = wrap(trim)
	}
	return strings.Join(lines, "\n")
}
