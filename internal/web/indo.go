package web

// Film Indo section (v1.8.0): Indonesian movies & series scraped
// from a KitaNonton deployment. Catalog/detail/play pages mirror the
// IDLIX cinema sections, but playback is different: the JuicyCodes
// CDN binds its identity token to the VIEWER's IP + User-Agent, so
// the server resolves the embed and hands the browser a master URL
// signed for the viewer; playlist and segments then flow directly
// from the CDN to the viewer's device (player.js re-applies the
// token to child requests via hls.js xhrSetup).

import (
	"context"
	"net"
	"net/http"
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
	ip := clientIP(r)
	ua := r.UserAgent()
	data.ProxyURL = kitanonton.SignForViewer(stream.MasterURL, ip, ua)
	data.CdnToken = kitanonton.MintToken(ip, ua)
	s.render(w, "cinemawatch", data)
}
