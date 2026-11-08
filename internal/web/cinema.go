package web

// Cinema sections (Film & Series) backed by the IDLIX catalog API.
// Added in v0.10.0 on the v0.9.0 foundation: shared media keys let
// movies and series use the same favorites/history system as anime.
// Playback arrives in v0.11.0; until then play buttons are visibly
// marked as upcoming instead of pretending to work.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/anilist"
	"github.com/wahyuadamhusaeni/naniplay/internal/idlix"
	"github.com/wahyuadamhusaeni/naniplay/internal/store"
)

// mediaHref returns the detail-page URL for a media key.
func mediaHref(key string) string {
	if id, ok := store.ParseAnimeKey(key); ok {
		return fmt.Sprintf("/anime/%d", id)
	}
	if slug, ok := strings.CutPrefix(key, "movie:"); ok {
		return "/film/" + slug
	}
	if slug, ok := strings.CutPrefix(key, "series:"); ok {
		return "/series/" + slug
	}
	return "/"
}

// watchHref returns the resume/watch URL for a history entry: anime
// plays in place; cinema titles point at their detail page until the
// cinema player lands in v0.11.0.
func watchHref(h store.HistoryEntry) string {
	if id, ok := store.ParseAnimeKey(h.Key); ok {
		return fmt.Sprintf("/watch/%d/%d", id, h.Episode)
	}
	if strings.HasPrefix(h.Key, "series:") && h.Episode >= 1000 {
		slug := strings.TrimPrefix(h.Key, "series:")
		return fmt.Sprintf("/series/%s/putar/%d/%d", slug, h.Episode/1000, h.Episode%1000)
	}
	return mediaHref(h.Key)
}

// epLabel renders the history badge: "Ep 3" for anime, "S1 E3" for
// series (episode stored as season*1000+episode), "" for movies.
func epLabel(h store.HistoryEntry) string {
	if _, ok := store.ParseAnimeKey(h.Key); ok {
		return "Ep " + strconv.Itoa(h.Episode)
	}
	if strings.HasPrefix(h.Key, "series:") && h.Episode >= 1000 {
		return "S" + strconv.Itoa(h.Episode/1000) + " E" + strconv.Itoa(h.Episode%1000)
	}
	return ""
}

// favView / histView wrap store rows with their URLs for templates.
type favView struct {
	store.Favorite
	Href      string
	NextLabel string
}

type histView struct {
	store.HistoryEntry
	Href      string
	EpLabel   string
	NextLabel string
}

func favViews(favs []store.Favorite) []favView {
	out := make([]favView, 0, len(favs))
	for _, f := range favs {
		out = append(out, favView{Favorite: f, Href: mediaHref(f.Key)})
	}
	return out
}

func histViews(hist []store.HistoryEntry) []histView {
	out := make([]histView, 0, len(hist))
	for _, h := range hist {
		out = append(out, histView{HistoryEntry: h, Href: watchHref(h), EpLabel: epLabel(h)})
	}
	return out
}

// ---------- browse (film & series catalog grids) ----------

type browseData struct {
	baseData
	Kind     string // "Film" atau "Series"
	Emoji    string
	Path     string // "/film" atau "/series"
	Cards    []cinemaCard
	Query    string
	Genre    string
	Genres   []idlix.Genre
	Page     int
	TotalPag int
	Total    int
	Disabled bool
	Err      string
}

// cinemaCard unifies movies, series, and search hits into one card
// shape for the cinemacard template define.
type cinemaCard struct {
	Href    string
	Poster  string
	Title   string
	Year    string
	Rating  float64
	Badge   string // quality (movies) or season count (series)
	NoVideo bool   // catalog entry without a playable video
}

func movieCards(items []idlix.MovieItem) []cinemaCard {
	out := make([]cinemaCard, 0, len(items))
	for _, m := range items {
		out = append(out, cinemaCard{
			Href: "/film/" + m.Slug, Poster: m.Poster(), Title: m.Title,
			Year: m.Year(), Rating: m.Rating.Float(), Badge: m.Quality,
			NoVideo: !m.HasVideo,
		})
	}
	return out
}

func seriesCards(items []idlix.SeriesItem) []cinemaCard {
	out := make([]cinemaCard, 0, len(items))
	for _, m := range items {
		badge := ""
		if m.NumberOfSeasons > 0 {
			badge = fmt.Sprintf("%d Season", m.NumberOfSeasons)
		}
		out = append(out, cinemaCard{
			Href: "/series/" + m.Slug, Poster: m.Poster(), Title: m.Title,
			Year: m.Year(), Rating: m.Rating.Float(), Badge: badge,
			NoVideo: !m.HasVideo,
		})
	}
	return out
}

func resultCards(items []idlix.SearchResult) []cinemaCard {
	out := make([]cinemaCard, 0, len(items))
	for _, m := range items {
		href := "/film/" + m.Slug
		if m.IsSeries() {
			href = "/series/" + m.Slug
		}
		out = append(out, cinemaCard{
			Href: href, Poster: m.Poster(), Title: m.Title,
			Year: m.Year(), Rating: m.Rating.Float(), Badge: m.Quality,
		})
	}
	return out
}

func (s *Server) filmPage(w http.ResponseWriter, r *http.Request) {
	s.browse(w, r, false)
}

func (s *Server) seriesPage(w http.ResponseWriter, r *http.Request) {
	s.browse(w, r, true)
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request, series bool) {
	kind, emoji, path := "Film", "🎬", "/film"
	if series {
		kind, emoji, path = "Series", "📺", "/series"
	}
	data := browseData{baseData: s.base(), Kind: kind, Emoji: emoji, Path: path, Page: 1}
	if !s.idlixEnabled() {
		data.Disabled = true
		s.render(w, "browse", data)
		return
	}
	ctx := r.Context()
	if genres, err := s.idlix.Genres(ctx); err == nil {
		data.Genres = genres
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data.Query = q
	data.Genre = r.URL.Query().Get("genre")
	if q != "" {
		results, total, err := s.idlix.Search(ctx, q)
		if err != nil {
			data.Err = cinemaErr(err)
		} else {
			data.Total = total
			var filtered []idlix.SearchResult
			for _, res := range results {
				if res.IsSeries() == series {
					filtered = append(filtered, res)
				}
			}
			data.Cards = resultCards(filtered)
		}
		s.render(w, "browse", data)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	data.Page = page
	if series {
		items, pg, err := s.idlix.SeriesList(ctx, page, 24, "", data.Genre)
		if err != nil {
			data.Err = cinemaErr(err)
		} else {
			data.Cards, data.TotalPag, data.Total = seriesCards(items), pg.TotalPages, pg.Total
		}
	} else {
		items, pg, err := s.idlix.Movies(ctx, page, 24, "", data.Genre)
		if err != nil {
			data.Err = cinemaErr(err)
		} else {
			data.Cards, data.TotalPag, data.Total = movieCards(items), pg.TotalPages, pg.Total
		}
	}
	s.render(w, "browse", data)
}

func cinemaErr(err error) string {
	if err == idlix.ErrChallenged {
		return "Sumber IDLIX sedang menantang akses otomatis (Cloudflare). Coba lagi sebentar lagi."
	}
	return "Katalog IDLIX sedang tidak dapat dijangkau. Coba lagi sebentar lagi."
}

// ---------- movie detail ----------

type movieDetailData struct {
	baseData
	Movie      *idlix.Movie
	IsFavorite bool
}

func (s *Server) filmDetailPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !s.idlixEnabled() {
		http.Redirect(w, r, "/film", http.StatusSeeOther)
		return
	}
	m, err := s.idlix.Movie(r.Context(), slug)
	if err != nil {
		s.renderNotFound(w)
		return
	}
	s.render(w, "moviedetail", movieDetailData{
		baseData:   s.base(),
		Movie:      m,
		IsFavorite: s.db.IsFavorite("movie:" + slug),
	})
}

// ---------- series detail ----------

type seriesDetailData struct {
	baseData
	Series     *idlix.Series
	Season     *idlix.SeasonDetail
	SeasonNum  int
	IsFavorite bool
}

func (s *Server) seriesDetailPage(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !s.idlixEnabled() {
		http.Redirect(w, r, "/series", http.StatusSeeOther)
		return
	}
	sr, err := s.idlix.SeriesDetail(r.Context(), slug)
	if err != nil {
		s.renderNotFound(w)
		return
	}
	data := seriesDetailData{
		baseData:   s.base(),
		Series:     sr,
		Season:     sr.DefaultSeason,
		IsFavorite: s.db.IsFavorite("series:" + slug),
	}
	if data.Season == nil && len(sr.Seasons) > 0 {
		// Some series ship without a default season; fall back to the
		// first season's episodes instead of an empty episode section.
		if sd, err := s.idlix.SeasonEpisodes(r.Context(), slug, sr.Seasons[0].SeasonNumber); err == nil && sd != nil {
			data.Season = sd
		}
	}
	if data.Season != nil {
		data.SeasonNum = data.Season.SeasonNumber
	}
	if want, _ := strconv.Atoi(r.URL.Query().Get("season")); want > 0 && want != data.SeasonNum {
		if sd, err := s.idlix.SeasonEpisodes(r.Context(), slug, want); err == nil && sd != nil {
			data.Season = sd
			data.SeasonNum = want
		}
	}
	s.render(w, "seriesdetail", data)
}

// ---------- halaman putar sinema ----------

type cinemaWatchData struct {
	baseData
	PrefQuality string
	PrefSubLang string
	Key         string
	Title       string
	Sub         string
	Meta        string
	Overview    string
	Fav         bool
	Self        string
	Poster      string
	Back        string
	BackText    string
	ProxyURL    string
	Episode     int // encoded (series: season*1000+episode; movie: 1)
	SavedPos    float64
	Next        string
	Prev        string
	Server      string
	Err         string
}

// playCinema resolves the stream (the provider gate takes ~15 s)
// and renders the shared player for one movie or episode.
func metaJoin(parts ...string) string {
	out := []string{}
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func durText(min int) string {
	if min <= 0 {
		return ""
	}
	if min >= 60 {
		return fmt.Sprintf("%dj %dm", min/60, min%60)
	}
	return fmt.Sprintf("%d mnt", min)
}

func yearOf(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

func ratingText(r idlix.Rating) string {
	if f := r.Float(); f > 0 {
		return "★ " + strconv.FormatFloat(f, 'f', 1, 64)
	}
	return ""
}

func (s *Server) playCinema(w http.ResponseWriter, r *http.Request, key string, encEp int, title, sub, meta, overview, poster, back, backText, ctype, contentID, next, prev string) {
	data := cinemaWatchData{
		baseData: s.base(), PrefQuality: s.qualityPref(), PrefSubLang: s.subPref(), Key: key, Title: title, Sub: sub, Meta: meta, Overview: overview, Poster: poster,
		Back: back, BackText: backText, Episode: encEp, Next: next, Prev: prev,
		Server: "IDLIX", Fav: s.db.IsFavorite(key), Self: r.URL.Path,
	}
	if saved, err := s.db.HistoryFor(key, encEp); err == nil && saved != nil {
		data.SavedPos = saved.Position
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	pb, err := s.idlix.ResolvePlayback(ctx, ctype, contentID)
	if err != nil {
		data.Err = "Putar gagal disiapkan: " + cinemaErr(err)
		s.render(w, "cinemawatch", data)
		return
	}
	s.importCinemaSubtitles(ctx, key, encEp, pb.Subtitles)
	data.ProxyURL = "/api/proxy?u=" + url.QueryEscape(pb.MasterURL) + "&r=" + url.QueryEscape(s.idlix.Base()+"/") + "&pq=1"
	s.render(w, "cinemawatch", data)
}

func (s *Server) cinemaWatchMovie(w http.ResponseWriter, r *http.Request) {
	if !s.idlixEnabled() {
		http.Redirect(w, r, "/film", http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	mv, err := s.idlix.Movie(r.Context(), slug)
	if err != nil || mv == nil {
		s.renderNotFound(w)
		return
	}
	meta := metaJoin(yearOf(mv.ReleaseDate), durText(mv.Runtime), mv.Quality, ratingText(mv.Rating))
	s.playCinema(w, r, "movie:"+slug, 1, mv.Title, "", meta, mv.Overview, mv.Poster(), "/film/"+slug, mv.Title, "movie", mv.ID, "", "")
}

func (s *Server) cinemaWatchSeries(w http.ResponseWriter, r *http.Request) {
	if !s.idlixEnabled() {
		http.Redirect(w, r, "/series", http.StatusSeeOther)
		return
	}
	slug := r.PathValue("slug")
	season, err1 := strconv.Atoi(r.PathValue("season"))
	epNum, err2 := strconv.Atoi(r.PathValue("episode"))
	if err1 != nil || err2 != nil || season < 0 || epNum < 1 {
		s.renderNotFound(w)
		return
	}
	resp, err := s.idlix.Episode(r.Context(), slug, season, epNum)
	if err != nil || resp == nil || resp.Episode == nil || resp.Series == nil {
		s.renderNotFound(w)
		return
	}
	next, prev := "", ""
	if resp.Season != nil {
		eps := resp.Season.Episodes
		for i, e := range eps {
			if e.EpisodeNumber == epNum {
				if i+1 < len(eps) {
					next = "/series/" + slug + "/putar/" + strconv.Itoa(season) + "/" + strconv.Itoa(eps[i+1].EpisodeNumber)
				}
				if i-1 >= 0 {
					prev = "/series/" + slug + "/putar/" + strconv.Itoa(season) + "/" + strconv.Itoa(eps[i-1].EpisodeNumber)
				}
				break
			}
		}
	}
	sub := "Season " + strconv.Itoa(season) + " · Episode " + strconv.Itoa(epNum)
	if resp.Episode.Name != "" {
		sub += " — " + resp.Episode.Name
	}
	title := resp.Series.Title
	meta := metaJoin(yearOf(resp.Series.FirstAirDate), durText(resp.Episode.Runtime), ratingText(resp.Series.Rating))
	s.playCinema(w, r, "series:"+slug, season*1000+epNum, title, sub, meta, resp.Series.Overview, resp.Series.Poster(), "/series/"+slug, title, "episode", resp.Episode.ID, next, prev)
}

// buildHeroSlides assembles the mixed home hero: top trending anime,
// most popular movie, most popular series. Missing sources simply
// drop out; seriesPlay resolves a series slug to its first-episode
// playback URL (or its detail page when unknown).
func buildHeroSlides(hero *anilist.Anime, movie *idlix.MovieItem, series *idlix.SeriesItem, seriesPlay func(string) string) []heroSlide {
	var slides []heroSlide
	if hero != nil {
		meta := []string{}
		if hero.Score > 0 {
			meta = append(meta, "★ "+strconv.Itoa(hero.Score)+"%")
		}
		if hero.SeasonYear > 0 {
			meta = append(meta, strconv.Itoa(hero.SeasonYear))
		}
		if hero.Format != "" {
			meta = append(meta, hero.Format)
		}
		if hero.Episodes > 0 {
			meta = append(meta, strconv.Itoa(hero.Episodes)+" episode")
		}
		img := hero.Banner
		if img == "" {
			img = hero.Cover
		}
		slides = append(slides, heroSlide{
			Kicker: "#1 SEDANG TRENDING — ANIME",
			Title:  hero.Display(),
			Meta:   strings.Join(meta, " · "),
			Desc:   hero.Description,
			Image:  img,
			Href:   "/anime/" + strconv.Itoa(hero.ID),
			Play:   "/watch/" + strconv.Itoa(hero.ID) + "/1",
			Key:    "anime:" + strconv.Itoa(hero.ID),
			Poster: hero.Cover,
		})
	}
	if movie != nil {
		meta := []string{}
		if movie.Rating.Float() > 0 {
			meta = append(meta, "★ "+fmt.Sprintf("%.1f", movie.Rating.Float()))
		}
		if movie.Year() != "" {
			meta = append(meta, movie.Year())
		}
		if movie.Quality != "" {
			meta = append(meta, movie.Quality)
		}
		img := movie.BackdropURL()
		if img == "" {
			img = movie.Poster()
		}
		slides = append(slides, heroSlide{
			Kicker: "FILM TERPOPULER",
			Title:  movie.Title,
			Meta:   strings.Join(meta, " · "),
			Image:  img,
			Href:   "/film/" + movie.Slug,
			Play:   "/film/" + movie.Slug + "/putar",
			Key:    "movie:" + movie.Slug,
			Poster: movie.Poster(),
		})
	}
	if series != nil {
		meta := []string{}
		if series.Rating.Float() > 0 {
			meta = append(meta, "★ "+fmt.Sprintf("%.1f", series.Rating.Float()))
		}
		if series.Year() != "" {
			meta = append(meta, series.Year())
		}
		if series.NumberOfSeasons > 0 {
			meta = append(meta, strconv.Itoa(series.NumberOfSeasons)+" season")
		}
		img := series.BackdropURL()
		if img == "" {
			img = series.Poster()
		}
		slides = append(slides, heroSlide{
			Kicker: "SERIES TERPOPULER",
			Title:  series.Title,
			Meta:   strings.Join(meta, " · "),
			Image:  img,
			Href:   "/series/" + series.Slug,
			Play:   seriesPlay(series.Slug),
			Key:    "series:" + series.Slug,
			Poster: series.Poster(),
		})
	}
	return slides
}

type cinemaGenreData struct {
	baseData
	Genres []idlix.Genre
	Err    string
}

// cinemaGenreIndex is the cinema genre index (/genre-sinema): every
// IDLIX genre as a tile linking into both browse sections.
func (s *Server) cinemaGenreIndex(w http.ResponseWriter, r *http.Request) {
	if !s.idlixEnabled() {
		http.Redirect(w, r, "/film", http.StatusSeeOther)
		return
	}
	data := cinemaGenreData{baseData: s.base()}
	genres, err := s.idlix.Genres(r.Context())
	if err != nil {
		data.Err = cinemaErr(err)
	} else {
		data.Genres = genres
	}
	s.render(w, "genrecinema", data)
}
