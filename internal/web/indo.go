package web

// Film Indo section (v1.8.0): Indonesian movies & series scraped
// from a KitaNonton deployment. Catalog/detail/play pages mirror the
// IDLIX cinema sections, but playback is proxied through the server
// (v1.8.1): the JuicyCodes CDN pins its CORS allowance to the embed
// origin and its identity token is bound to the fetcher, so a
// browser on this site can never read the stream directly. The
// server resolves the embed, signs requests with its OWN identity
// (egress IP + fetcher UA), fetches playlist and segments with the
// TLS-fingerprint client, and rewrites playlists so the browser
// only ever talks to NaniPlay — the same posture as IDLIX playback.

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/kitanonton"
)

// indoEnabled reports whether the Film Indo catalog is on.
func (s *Server) indoEnabled() bool {
	return s.db.GetSetting("indo_on") == "1"
}

// clientIP determines the viewer's public IP as this server sees it:
// Cloudflare's header first, then the proxy chain, then the socket.
func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first := strings.TrimSpace(strings.Split(xff, ",")[0]); first != "" {
			return first
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// indoCardOf maps a catalog item onto the shared cinema card shape.
func indoCardOf(it kitanonton.Item) cinemaCard {
	year := ""
	if it.Year > 0 {
		year = strconv.Itoa(it.Year)
	}
	if it.Series {
		badge := "Series"
		if it.Episodes > 0 {
			badge = strconv.Itoa(it.Episodes) + " Episode"
		}
		return cinemaCard{Href: "/series-indo/" + it.Slug, Poster: it.Poster, Title: it.Title, Year: year, Rating: it.Rating, Badge: badge}
	}
	return cinemaCard{Href: "/film-indo/" + it.Slug, Poster: it.Poster, Title: it.Title, Year: year, Rating: it.Rating, Badge: it.Quality}
}

func indoCards(items []kitanonton.Item) []cinemaCard {
	out := make([]cinemaCard, 0, len(items))
	for _, it := range items {
		out = append(out, indoCardOf(it))
	}
	return out
}

// ---------- browse ----------

type indoBrowseData struct {
	baseData
	Cards    []cinemaCard
	Page     int
	TotalPag int
	Disabled bool
	Err      string
}

func (s *Server) indoBrowse(w http.ResponseWriter, r *http.Request) {
	data := indoBrowseData{baseData: s.base(), Page: 1}
	if !s.indoEnabled() {
		data.Disabled = true
		s.render(w, "indobrowse", data)
		return
	}
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		data.Page = p
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	items, total, err := s.kita.Catalog(ctx, data.Page)
	if err != nil {
		data.Err = "Katalog Film Indo tidak bisa dimuat: " + err.Error()
		s.render(w, "indobrowse", data)
		return
	}
	data.Cards = indoCards(items)
	data.TotalPag = total
	s.render(w, "indobrowse", data)
}

// ---------- detail ----------

type indoDetailData struct {
	baseData
	D        *kitanonton.Detail
	Key      string
	Meta     string
	Fav      bool
	Episodes int   // series only: number of episodes with servers
	EpList   []int // series only: 1..Episodes
}

func (s *Server) indoDetail(w http.ResponseWriter, r *http.Request, series bool) {
	if !s.indoEnabled() {
		http.Redirect(w, r, "/film-indo", http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	d, err := s.kita.Detail(ctx, slug, series)
	if err != nil || d == nil {
		s.renderNotFound(w)
		return
	}
	key := "indo:" + slug
	if series {
		key = "indoser:" + slug
	}
	meta := metaJoin(yearOf(d.ReleaseDate), durText(d.DurationMin), d.Quality, ratingTextF(d.Rating))
	if meta == "" {
		parts := []string{}
		if d.Year > 0 {
			parts = append(parts, strconv.Itoa(d.Year))
		}
		if d.DurationMin > 0 {
			parts = append(parts, durText(d.DurationMin))
		}
		if d.Quality != "" {
			parts = append(parts, d.Quality)
		}
		if d.Rating > 0 {
			parts = append(parts, "★ "+strconv.FormatFloat(d.Rating, 'f', 1, 64))
		}
		meta = metaJoin(parts...)
	}
	data := indoDetailData{baseData: s.base(), D: d, Key: key, Meta: meta, Fav: s.db.IsFavorite(key)}
	if series {
		if servers, err := s.kita.PlayServers(ctx, slug, true); err == nil {
			data.Episodes = kitanonton.EpisodeCount(servers)
			for i := 1; i <= data.Episodes; i++ {
				data.EpList = append(data.EpList, i)
			}
		}
	}
	s.render(w, "indodetail", data)
}

func (s *Server) indoMovieDetail(w http.ResponseWriter, r *http.Request) {
	s.indoDetail(w, r, false)
}

func (s *Server) indoSeriesDetail(w http.ResponseWriter, r *http.Request) {
	s.indoDetail(w, r, true)
}

// ratingTextF formats a plain float rating like the IDLIX helper
// does for its Rating type.
func ratingTextF(f float64) string {
	if f <= 0 {
		return ""
	}
	return "★ " + strconv.FormatFloat(f, 'f', 1, 64)
}

// ---------- watch ----------

func (s *Server) indoWatchMovie(w http.ResponseWriter, r *http.Request) {
	if !s.indoEnabled() {
		http.Redirect(w, r, "/film-indo", http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	s.playIndo(w, r, slug, false, 1)
}

func (s *Server) indoWatchSeries(w http.ResponseWriter, r *http.Request) {
	if !s.indoEnabled() {
		http.Redirect(w, r, "/film-indo", http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	ep, err := strconv.Atoi(r.PathValue("episode"))
	if err != nil || ep < 1 {
		s.renderNotFound(w)
		return
	}
	s.playIndo(w, r, slug, true, ep)
}

// playIndo resolves one title's JuicyCodes stream and renders the
// shared cinema player with a viewer-signed direct CDN URL.
func (s *Server) playIndo(w http.ResponseWriter, r *http.Request, slug string, series bool, ep int) {
	key := "indo:" + slug
	back := "/film-indo/" + slug
	if series {
		key = "indoser:" + slug
		back = "/series-indo/" + slug
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	d, _ := s.kita.Detail(ctx, slug, series)
	title := slug
	poster, overview, meta := "", "", ""
	if d != nil {
		title = d.Title
		poster = d.Poster
		overview = d.Overview
		meta = metaJoin(yearOf(d.ReleaseDate), durText(d.DurationMin), d.Quality, ratingTextF(d.Rating))
	}
	data := cinemaWatchData{
		baseData: s.base(), PrefQuality: s.qualityPref(), PrefSubLang: s.subPref(),
		Key: key, Title: title, Meta: meta, Overview: overview, Poster: poster,
		Back: back, BackText: title, Episode: ep,
		Server: "KitaNonton", Fav: s.db.IsFavorite(key), Self: r.URL.Path,
	}
	if saved, err := s.db.HistoryFor(key, ep); err == nil && saved != nil {
		data.SavedPos = saved.Position
	}
	servers, err := s.kita.PlayServers(ctx, slug, series)
	if err != nil {
		data.Err = "Daftar server tidak bisa dimuat: " + err.Error()
		s.render(w, "cinemawatch", data)
		return
	}
	if series {
		total := kitanonton.EpisodeCount(servers)
		if ep > total && total > 0 {
			data.Err = "Episode " + strconv.Itoa(ep) + " belum tersedia (baru sampai episode " + strconv.Itoa(total) + ")."
			s.render(w, "cinemawatch", data)
			return
		}
		if ep > 1 {
			data.Prev = "/series-indo/" + slug + "/putar/" + strconv.Itoa(ep-1)
		}
		if ep < total {
			data.Next = "/series-indo/" + slug + "/putar/" + strconv.Itoa(ep+1)
		}
	}
	var chosen *kitanonton.Server
	abyssOnly := false
	for _, sv := range kitanonton.ServersForEpisode(servers, ep) {
		sv := sv
		if sv.Type == "dv" {
			chosen = &sv
			break
		}
		abyssOnly = true
	}
	if chosen == nil {
		if abyssOnly {
			data.Err = "Server yang tersedia untuk judul ini (AbyssPlayer) belum didukung NaniPlay."
		} else {
			data.Err = "Tidak ada server untuk episode ini."
		}
		s.render(w, "cinemawatch", data)
		return
	}
	stream, err := s.kita.Resolve(ctx, chosen.URL)
	if err != nil {
		data.Err = "Stream gagal disiapkan: " + err.Error()
		s.render(w, "cinemawatch", data)
		return
	}
	data.ProxyURL = "/api/indo-proxy?u=" + url.QueryEscape(stream.MasterURL) + "&r=" + url.QueryEscape(chosen.URL)
	data.Embed = chosen.URL
	// Mode embed memakai server AbyssPlayer ("ex") bila ada: embed
	// JuicyCodes terkunci referer + gerbang token yang agresif,
	// sedangkan Abyss terbuka dan justru lawan yang terbukti untuk
	// pola embed terproksi (elka21). Cadangan: embed dv yang sama.
	embedURL := chosen.URL
	for _, sv := range kitanonton.ServersForEpisode(servers, ep) {
		if sv.Type == "ex" {
			embedURL = sv.URL
			break
		}
	}
	data.EmbedProxy = "/api/indo-embed?u=" + url.QueryEscape(embedURL)
	s.render(w, "cinemawatch", data)
}

// indoProxy serves one JuicyCodes CDN resource (master/variant
// playlist or segment) through the server: it signs the URL with the
// server's identity token, fetches it with the fingerprint client
// and the embed page as referer, rewrites playlists so children come
// back through this endpoint, and streams everything else through.
// Only groovy.monster hosts (the JuicyCodes CDN) are allowed.
func (s *Server) indoProxy(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	referer := r.URL.Query().Get("r")
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Host, ".groovy.monster") {
		http.Error(w, "URL tidak diizinkan", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	signed := s.kita.SignForServer(ctx, raw)
	status, ctype, body, err := s.kita.FetchStream(ctx, signed, referer, r.Header.Get("Range"))
	if err != nil {
		http.Error(w, "Stream tidak terjangkau: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()
	br := bufio.NewReader(body)
	isPlaylist := strings.Contains(ctype, "mpegurl")
	if !isPlaylist {
		if head, err := br.Peek(7); err == nil && string(head) == "#EXTM3U" {
			isPlaylist = true
		}
	}
	if isPlaylist {
		b, err := io.ReadAll(io.LimitReader(br, 4<<20))
		if err != nil {
			http.Error(w, "Playlist tidak terbaca", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(rewriteIndoPlaylist(string(b), signed, referer)))
		return
	}
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, br)
}

// rewriteIndoPlaylist points every child URL of a JuicyCodes
// playlist back at /api/indo-proxy. The handler re-signs each child
// request itself, so no token is embedded in the rewritten URLs.
func rewriteIndoPlaylist(body, base, referer string) string {
	baseURL, err := url.Parse(base)
	if err != nil {
		return body
	}
	wrap := func(child string) string {
		cu, err := url.Parse(child)
		if err != nil {
			return child
		}
		abs := baseURL.ResolveReference(cu).String()
		return "/api/indo-proxy?u=" + url.QueryEscape(abs) + "&r=" + url.QueryEscape(referer)
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
