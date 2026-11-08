package web

// Cinema sections (Film & Series) backed by the IDLIX catalog API.
// Added in v0.10.0 on the v0.9.0 foundation: shared media keys let
// movies and series use the same favorites/history system as anime.
// Playback arrives in v0.11.0; until then play buttons are visibly
// marked as upcoming instead of pretending to work.

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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
	return mediaHref(h.Key)
}

// favView / histView wrap store rows with their URLs for templates.
type favView struct {
	store.Favorite
	Href string
}

type histView struct {
	store.HistoryEntry
	Href string
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
		out = append(out, histView{HistoryEntry: h, Href: watchHref(h)})
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
	Href   string
	Poster string
	Title  string
	Year   string
	Rating float64
	Badge  string // quality (movies) or season count (series)
}

func movieCards(items []idlix.MovieItem) []cinemaCard {
	out := make([]cinemaCard, 0, len(items))
	for _, m := range items {
		out = append(out, cinemaCard{
			Href: "/film/" + m.Slug, Poster: m.Poster(), Title: m.Title,
			Year: m.Year(), Rating: m.Rating.Float(), Badge: m.Quality,
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
		s.render(w, "notfound", s.base())
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
		s.render(w, "notfound", s.base())
		return
	}
	data := seriesDetailData{
		baseData:   s.base(),
		Series:     sr,
		Season:     sr.DefaultSeason,
		IsFavorite: s.db.IsFavorite("series:" + slug),
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
