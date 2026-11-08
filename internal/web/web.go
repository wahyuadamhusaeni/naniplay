// Package web serves the NaniPlay UI and its JSON API.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/anilist"
	"github.com/wahyuadamhusaeni/naniplay/internal/aniskip"
	"github.com/wahyuadamhusaeni/naniplay/internal/idlix"
	"github.com/wahyuadamhusaeni/naniplay/internal/proxy"
	"github.com/wahyuadamhusaeni/naniplay/internal/source"
	"github.com/wahyuadamhusaeni/naniplay/internal/source/anichan"
	"github.com/wahyuadamhusaeni/naniplay/internal/source/hianime"
	"github.com/wahyuadamhusaeni/naniplay/internal/source/otakudesu"
	"github.com/wahyuadamhusaeni/naniplay/internal/store"
	"github.com/wahyuadamhusaeni/naniplay/internal/subtitle"
)

//go:embed templates static
var assets embed.FS

// Version is the running build version; main overrides it via -ldflags.
var Version = "0.2.0"

// Server bundles the HTTP handlers and their dependencies.
type Server struct {
	anilist   *anilist.Client
	aniskip   *aniskip.Client
	otaku     *otakudesu.Client
	hi        *hianime.Client
	ani       *anichan.Client
	providers map[string]source.Provider
	db        *store.Store
	idlixBase string
	idlix     *idlix.Client
	tpl       map[string]*template.Template

	mu       sync.Mutex
	matches  map[string]matchEntry // "id|src" -> source match
	episodes map[string][]source.Episode

	relMu     sync.Mutex
	latestTag string
	latestAt  time.Time
}

type matchEntry struct {
	series   *source.Series
	provider string // provider id that produced the match
	at       time.Time
}

// New builds a Server. Saved otakudesu_base / hianime_base settings
// override the configured defaults, so a source domain change never
// needs a rebuild.
func New(db *store.Store, otakuBase, hiBase, idlixBase string) (*Server, error) {
	if saved := db.GetSetting("otakudesu_base"); saved != "" {
		otakuBase = saved
	}
	if saved := db.GetSetting("hianime_base"); saved != "" {
		hiBase = saved
	}
	if saved := db.GetSetting("idlix_base"); saved != "" {
		idlixBase = saved
	}
	idlixClient, err := idlix.New(idlixBase)
	if err != nil {
		return nil, err
	}
	otaku := otakudesu.New(otakuBase)
	hi := hianime.New(hiBase)
	ani := anichan.New("")
	s := &Server{
		anilist: anilist.New(),
		aniskip: aniskip.New(),
		otaku:   otaku,
		hi:      hi,
		ani:     ani,
		providers: map[string]source.Provider{
			otaku.Name(): otaku,
			hi.Name():    hi,
			ani.Name():   ani,
		},
		db:        db,
		idlixBase: idlixBase,
		idlix:     idlixClient,
		tpl:       map[string]*template.Template{},
		matches:   map[string]matchEntry{},
		episodes:  map[string][]source.Episode{},
	}
	pages := []string{"home", "detail", "watch", "search", "list", "notfound", "jadwal", "settings", "genre", "genrelist", "browse", "moviedetail", "seriesdetail", "cinemawatch", "genrecinema"}
	for _, p := range pages {
		t, err := template.New("base").Funcs(template.FuncMap{
			"pct": func(f float64) int { return int(f * 100) },
			"add": func(a, b int) int { return a + b },
			"sub": func(a, b int) int { return a - b },
		}).ParseFS(assets, "templates/base.html", "templates/"+p+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", p, err)
		}
		s.tpl[p] = t
	}
	return s, nil
}

// baseData is embedded in every page's data so the shared base
// template can render feature-gated navigation (Film/Series sections
// appear only when the IDLIX catalog is enabled).
type baseData struct {
	IdlixOn bool
}

func (s *Server) base() baseData {
	return baseData{IdlixOn: s.idlixEnabled()}
}

// idlixEnabled reports whether the IDLIX movie/series catalog is on.
func (s *Server) idlixEnabled() bool {
	return s.db.GetSetting("idlix_on") == "1"
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))

	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /anime/{id}", s.detail)
	mux.HandleFunc("GET /watch/{id}/{ep}", s.watch)
	mux.HandleFunc("GET /search", s.searchPage)
	mux.HandleFunc("GET /genre", s.genreIndex)
	mux.HandleFunc("GET /genre-sinema", s.cinemaGenreIndex)
	mux.HandleFunc("GET /genre/{name}", s.genrePage)
	mux.HandleFunc("GET /jadwal", s.schedulePage)
	mux.HandleFunc("GET /film", s.filmPage)
	mux.HandleFunc("GET /series", s.seriesPage)
	mux.HandleFunc("GET /film/{slug}", s.filmDetailPage)
	mux.HandleFunc("GET /film/{slug}/putar", s.cinemaWatchMovie)
	mux.HandleFunc("GET /series/{slug}", s.seriesDetailPage)
	mux.HandleFunc("GET /series/{slug}/putar/{season}/{episode}", s.cinemaWatchSeries)
	mux.HandleFunc("GET /daftar-saya", s.myList)
	mux.HandleFunc("GET /riwayat", s.historyPage)
	mux.HandleFunc("GET /pengaturan", s.settingsPage)
	mux.HandleFunc("POST /pengaturan", s.saveSettings)

	mux.HandleFunc("POST /api/history", s.apiSaveHistory)
	mux.HandleFunc("GET /api/history/{id}/{ep}", s.apiGetHistory)
	mux.HandleFunc("GET /api/skiptimes/{id}/{ep}", s.apiSkipTimes)
	mux.HandleFunc("GET /api/subtitles/{id}/{ep}", s.apiSubtitleList)
	mux.HandleFunc("GET /api/subtitles/k/{key}/{ep}", s.apiSubtitleList)
	mux.HandleFunc("POST /api/subtitles/k/{key}/{ep}/external", s.apiSubtitleExternal)
	mux.HandleFunc("POST /api/subtitles/k/{key}/{ep}/upload", s.apiSubtitleUpload)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/fetch", s.apiSubtitleFetch)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/external", s.apiSubtitleExternal)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/upload", s.apiSubtitleUpload)
	mux.HandleFunc("GET /api/subtitles/track/{trackID}", s.apiSubtitleTrack)
	mux.HandleFunc("POST /api/subtitles/track/{trackID}/translate", s.apiSubtitleTranslate)
	mux.HandleFunc("POST /api/subtitles/track/{trackID}/merge", s.apiSubtitleMerge)
	mux.HandleFunc("POST /api/subtitles/track/{trackID}/autosync", s.apiSubtitleAutosync)
	mux.HandleFunc("POST /api/subtitles/track/{trackID}/translate-llm", s.apiSubtitleTranslateAI)
	mux.HandleFunc("DELETE /api/subtitles/track/{trackID}", s.apiSubtitleDelete)
	mux.HandleFunc("GET /api/health", s.apiHealth)
	mux.HandleFunc("GET /api/version", s.apiVersion)
	mux.HandleFunc("POST /favorites/toggle", s.toggleFavorite)
	mux.Handle("GET /api/proxy", proxy.Handler())
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tpl[page].ExecuteTemplate(w, "base", data); err != nil {
		log.Printf("render %s: %v", page, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ---------- pages ----------

// heroSlide is one entry of the mixed home hero rotation (anime,
// movies, series side by side since v0.12.0).
type heroSlide struct {
	Kicker string
	Title  string
	Meta   string
	Desc   string
	Image  string
	Href   string
	Play   string
}

type homeData struct {
	baseData
	Slides     []heroSlide
	Hero       *anilist.Anime
	Trending   []anilist.Anime
	Season     []anilist.Anime
	SeasonName string
	Year       int
	Top        []anilist.Anime
	Continue   []histView
	Favorites  []favView
	CinemaFilm []cinemaCard
	CinemaSeri []cinemaCard
	Err        string
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.render(w, "notfound", s.base())
		return
	}
	ctx := r.Context()
	data := homeData{
		baseData: s.base()}
	trending, err := s.anilist.Trending(ctx, 18)
	if err != nil {
		data.Err = err.Error()
	} else {
		data.Trending = trending
		if len(trending) > 0 {
			data.Hero = &trending[0]
		}
	}
	if season, err := s.anilist.SeasonNow(ctx, 18); err == nil {
		data.Season = season
	}
	if top, err := s.anilist.TopAllTime(ctx, 18); err == nil {
		data.Top = top
	}
	name, year := anilist.CurrentSeason()
	data.SeasonName, data.Year = seasonLabel(name), year
	if cw, err := s.db.ContinueWatching(12); err == nil {
		data.Continue = histViews(cw)
	}
	if favs, err := s.db.Favorites(); err == nil {
		data.Favorites = favViews(favs)
	}
	var topMovie *idlix.MovieItem
	var topSeries *idlix.SeriesItem
	if s.idlixEnabled() {
		if items, _, err := s.idlix.Movies(ctx, 1, 12, "popularity", ""); err == nil {
			data.CinemaFilm = movieCards(items)
			if len(items) > 0 {
				topMovie = &items[0]
			}
		}
		if items, _, err := s.idlix.SeriesList(ctx, 1, 12, "popularity", ""); err == nil {
			data.CinemaSeri = seriesCards(items)
			if len(items) > 0 {
				topSeries = &items[0]
			}
		}
	}
	data.Slides = buildHeroSlides(data.Hero, topMovie, topSeries, func(slug string) string {
		sd, err := s.idlix.SeriesDetail(ctx, slug)
		if err == nil && sd != nil && sd.DefaultSeason != nil && len(sd.DefaultSeason.Episodes) > 0 {
			ep := sd.DefaultSeason.Episodes[0]
			return "/series/" + slug + "/putar/" + strconv.Itoa(sd.DefaultSeason.SeasonNumber) + "/" + strconv.Itoa(ep.EpisodeNumber)
		}
		return "/series/" + slug
	})
	s.render(w, "home", data)
}

func seasonLabel(s string) string {
	switch s {
	case "WINTER":
		return "Musim Dingin"
	case "SPRING":
		return "Musim Semi"
	case "SUMMER":
		return "Musim Panas"
	case "FALL":
		return "Musim Gugur"
	}
	return s
}

type detailData struct {
	baseData
	Anime        *anilist.Anime
	IsFavorite   bool
	Series       *source.Series
	Episodes     []source.Episode
	SourceErr    string
	Continue     *store.HistoryEntry
	Relations    []anilist.Relation
	SrcSel       string      // forced provider id from ?src= ("" = automatic)
	ProviderName string      // display name of the resolved provider
	PagePath     string      // canonical path of this page (for source chips)
	CurrentEp    int         // episode to highlight in the picker (continue point)
	NextLabel    string      // next-airing episode info, "" if none
	Watched      map[int]int // episode -> watched percent
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.anilist.Detail(r.Context(), id)
	if err != nil {
		s.render(w, "notfound", s.base())
		return
	}
	forced := validSrc(r.URL.Query().Get("src"))
	data := detailData{
		baseData:   s.base(),
		Anime:      a,
		NextLabel:  nextLabel(a),
		IsFavorite: s.db.IsFavorite(store.AnimeKey(id)),
		SrcSel:     forced,
		PagePath:   fmt.Sprintf("/anime/%d", id),
	}
	if rels, err := s.anilist.Relations(r.Context(), id); err == nil {
		data.Relations = rels
	}
	if watched, err := s.db.EpisodeProgress(store.AnimeKey(id)); err == nil {
		data.Watched = watched
	}
	series, eps, provider := s.matchEpisodes(r.Context(), a, forced)
	if series != nil {
		data.Series = series
		data.Episodes = eps
		data.ProviderName = providerDisplay(provider)
	} else if forced != "" {
		data.SourceErr = fmt.Sprintf("Judul ini tidak tersedia di %s. Coba sumber lain di pemilih sumber.", providerDisplay(forced))
	} else {
		data.SourceErr = "Belum ditemukan di sumber streaming mana pun (Otakudesu/HiAnime/AniChan)."
	}
	if cw, err := s.db.ContinueWatching(50); err == nil {
		for i := range cw {
			if cw[i].AnilistID == id {
				data.Continue = &cw[i]
				data.CurrentEp = cw[i].Episode
				break
			}
		}
	}
	s.render(w, "detail", data)
}

// matchEpisodes resolves (and caches) the streaming series +
// episodes for an AniList anime. With forced == "" the chain is
// Otakudesu (Sub Indo hardsub) -> HiAnime -> AniChan; a forced
// provider id ("otakudesu"/"hianime"/"anichan") uses only that
// source, for the manual source picker. The third return value is
// the winning provider's id.
func (s *Server) matchEpisodes(ctx context.Context, a *anilist.Anime, forced string) (*source.Series, []source.Episode, string) {
	key := fmt.Sprintf("%d|%s", a.ID, forced)
	s.mu.Lock()
	if m, ok := s.matches[key]; ok && time.Since(m.at) < 30*time.Minute {
		eps := s.episodes[key]
		s.mu.Unlock()
		return m.series, eps, m.provider
	}
	s.mu.Unlock()

	titles := []string{a.Title.Romaji, a.Title.English}
	var series *source.Series
	var eps []source.Episode
	var err error
	provider := ""
	try := func(name string, fn func() (*source.Series, []source.Episode, error)) bool {
		if forced != "" && forced != name {
			return false
		}
		series, eps, err = fn()
		if err == nil && series != nil {
			provider = name
			return true
		}
		return false
	}
	_ = try(s.otaku.Name(), func() (*source.Series, []source.Episode, error) { return s.otaku.Match(ctx, titles, a.Episodes) }) ||
		try(s.hi.Name(), func() (*source.Series, []source.Episode, error) { return s.hi.Match(ctx, titles, a.Episodes) }) ||
		try(s.ani.Name(), func() (*source.Series, []source.Episode, error) {
			return s.ani.Lookup(ctx, a.ID, a.Title.Romaji, a.Episodes)
		})
	if series == nil {
		return nil, nil, ""
	}
	s.mu.Lock()
	s.matches[key] = matchEntry{series: series, provider: provider, at: time.Now()}
	s.episodes[key] = eps
	s.mu.Unlock()
	return series, eps, provider
}

// validSrc normalizes the ?src= picker value.
func validSrc(v string) string {
	switch v {
	case "otakudesu", "hianime", "anichan":
		return v
	default:
		return ""
	}
}

// providerDisplay maps a provider id to its display name.
func providerDisplay(id string) string {
	switch id {
	case "otakudesu":
		return "Otakudesu"
	case "hianime":
		return "HiAnime"
	case "anichan":
		return "AniChan"
	default:
		return id
	}
}

type watchData struct {
	baseData
	Anime        *anilist.Anime
	Episode      int
	CurrentEp    int
	Stream       *source.Stream
	ProxyURL     string
	Episodes     []source.Episode
	PrevEp       int
	NextEp       int
	SavedPos     float64
	SourceErr    string
	SrcSel       string
	ProviderName string
	PagePath     string
	Watched      map[int]int
}

func (s *Server) watch(w http.ResponseWriter, r *http.Request) {
	id, err1 := strconv.Atoi(r.PathValue("id"))
	ep, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.anilist.Detail(r.Context(), id)
	if err != nil {
		s.render(w, "notfound", s.base())
		return
	}
	forced := validSrc(r.URL.Query().Get("src"))
	data := watchData{
		baseData:  s.base(),
		Anime:     a,
		Episode:   ep,
		CurrentEp: ep,
		SrcSel:    forced,
		PagePath:  fmt.Sprintf("/watch/%d/%d", id, ep),
	}
	if watched, err := s.db.EpisodeProgress(store.AnimeKey(id)); err == nil {
		data.Watched = watched
	}
	series, eps, providerName := s.matchEpisodes(r.Context(), a, forced)
	if series == nil {
		if forced != "" {
			data.SourceErr = fmt.Sprintf("Judul ini tidak tersedia di %s. Coba sumber lain di pemilih sumber.", providerDisplay(forced))
		} else {
			data.SourceErr = "Sumber streaming untuk judul ini belum ditemukan."
		}
		s.render(w, "watch", data)
		return
	}
	data.ProviderName = providerDisplay(providerName)
	data.Episodes = eps
	var target *source.Episode
	for i := range eps {
		if eps[i].Number == ep {
			target = &eps[i]
		}
		if eps[i].Number == ep-1 {
			data.PrevEp = ep - 1
		}
		if eps[i].Number == ep+1 {
			data.NextEp = ep + 1
		}
	}
	if target == nil {
		data.SourceErr = fmt.Sprintf("Episode %d belum tersedia di sumber.", ep)
		s.render(w, "watch", data)
		return
	}
	provider := s.providers[providerName]
	if provider == nil {
		provider = s.otaku
	}
	stream, err := provider.Resolve(r.Context(), target.URL)
	if err != nil {
		data.SourceErr = "Gagal menyiapkan stream: " + err.Error()
		s.render(w, "watch", data)
		return
	}
	data.Stream = stream
	if len(stream.Subtitles) > 0 {
		s.importStreamSubtitles(r.Context(), a, ep, stream)
	}
	data.ProxyURL = proxyURLFor(stream)
	if h, err := s.db.HistoryFor(store.AnimeKey(id), ep); err == nil && h != nil {
		data.SavedPos = h.Position
	}
	s.render(w, "watch", data)
}

func proxyURLFor(st *source.Stream) string {
	q := "u=" + strings.ReplaceAll(strings.ReplaceAll(st.MasterURL, "%", "%25"), "&", "%26")
	return "/api/proxy?" + q + "&r=" + strings.ReplaceAll(st.Referer, "&", "%26")
}

type searchData struct {
	baseData
	Query      string
	Results    []anilist.Anime
	Err        string
	CinemaFilm []cinemaCard
	CinemaSeri []cinemaCard
	CinemaNote string
}

type genreIndexData struct {
	baseData
	Genres []string
}

func (s *Server) genreIndex(w http.ResponseWriter, r *http.Request) {
	genres := []string{}
	for _, g := range anilist.Genres {
		if g == "Hentai" {
			continue // katalog NaniPlay selalu non-dewasa (isAdult=false)
		}
		genres = append(genres, g)
	}
	s.render(w, "genre", genreIndexData{
		baseData: s.base(), Genres: genres})
}

type genreListData struct {
	baseData
	Genre   string
	Results []anilist.Anime
	Page    int
	HasNext bool
	Err     string
}

func (s *Server) genrePage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	valid := false
	for _, g := range anilist.Genres {
		if g == name {
			valid = true
			break
		}
	}
	if !valid {
		http.NotFound(w, r)
		return
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	data := genreListData{
		baseData: s.base(), Genre: name, Page: page}
	res, hasNext, err := s.anilist.ByGenre(r.Context(), name, page)
	if err != nil {
		data.Err = err.Error()
	} else {
		data.Results = res
		data.HasNext = hasNext
	}
	s.render(w, "genrelist", data)
}

func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := searchData{
		baseData: s.base(), Query: q}
	if q != "" {
		res, err := s.anilist.Search(r.Context(), q, 24)
		if err != nil {
			data.Err = err.Error()
		} else {
			data.Results = res
		}
		if s.idlixEnabled() {
			results, _, err := s.idlix.Search(r.Context(), q)
			if err != nil {
				data.CinemaNote = cinemaErr(err)
			} else {
				var films, series []idlix.SearchResult
				for _, it := range results {
					if it.IsSeries() {
						series = append(series, it)
					} else {
						films = append(films, it)
					}
				}
				data.CinemaFilm = resultCards(films)
				data.CinemaSeri = resultCards(series)
			}
		}
	}
	s.render(w, "search", data)
}

// nextLabel formats an anime's next-airing info for badges, e.g.
// "Ep 12 · Sabtu, 15 November 2026 · 22:30 WIB" ("" when none).
func nextLabel(a *anilist.Anime) string {
	if a == nil || a.NextEpisode == 0 || a.NextAiringAt == 0 {
		return ""
	}
	t := time.Unix(a.NextAiringAt, 0).In(wib)
	return fmt.Sprintf("Ep %d · %s, %d %s %d · %02d:%02d WIB",
		a.NextEpisode, dayNames[t.Weekday()], t.Day(), monthNames[t.Month()], t.Year(), t.Hour(), t.Minute())
}

// fillNextLabels enriches list views with next-episode info for
// anime entries (favorites/history), fetched in parallel with a
// bounded timeout; failures simply leave labels empty.
func (s *Server) fillNextLabels(r *http.Request, favs []favView, hist []histView) {
	ids := map[int]bool{}
	collect := func(key string) {
		if id, ok := store.ParseAnimeKey(key); ok {
			ids[id] = true
		}
	}
	for _, f := range favs {
		collect(f.Key)
	}
	for _, h := range hist {
		collect(h.Key)
	}
	if len(ids) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	labels := map[int]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for id := range ids {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if a, err := s.anilist.Detail(ctx, id); err == nil {
				if lbl := nextLabel(a); lbl != "" {
					mu.Lock()
					labels[id] = lbl
					mu.Unlock()
				}
			}
		}(id)
	}
	wg.Wait()
	for i := range favs {
		if id, ok := store.ParseAnimeKey(favs[i].Key); ok {
			favs[i].NextLabel = labels[id]
		}
	}
	for i := range hist {
		if id, ok := store.ParseAnimeKey(hist[i].Key); ok {
			hist[i].NextLabel = labels[id]
		}
	}
}

type listData struct {
	baseData
	Title     string
	Favorites []favView
	History   []histView
}

func (s *Server) myList(w http.ResponseWriter, r *http.Request) {
	favs, _ := s.db.Favorites()
	views := favViews(favs)
	s.fillNextLabels(r, views, nil)
	s.render(w, "list", listData{
		baseData: s.base(), Title: "Daftar Saya", Favorites: views})
}

func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	hist, _ := s.db.AllHistory(60)
	views := histViews(hist)
	s.fillNextLabels(r, nil, views)
	s.render(w, "list", listData{
		baseData: s.base(), Title: "Riwayat Tontonan", History: views})
}

// ---------- actions & API ----------

func (s *Server) toggleFavorite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form tidak valid", http.StatusBadRequest)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("anilist_id"))
	key := r.FormValue("media_key")
	if key == "" && id != 0 {
		key = store.AnimeKey(id)
	}
	if key == "" {
		http.Error(w, "id tidak valid", http.StatusBadRequest)
		return
	}
	_, err := s.db.ToggleFavorite(store.Favorite{
		Key:    key,
		Title:  r.FormValue("title"),
		Poster: r.FormValue("poster"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	back := r.FormValue("return")
	if back == "" {
		back = fmt.Sprintf("/anime/%d", id)
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type historyPayload struct {
	Key       string  `json:"key"`
	AnilistID int     `json:"anilistId"`
	Title     string  `json:"title"`
	Poster    string  `json:"poster"`
	Episode   int     `json:"episode"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
}

func (s *Server) apiSaveHistory(w http.ResponseWriter, r *http.Request) {
	var p historyPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload tidak valid"})
		return
	}
	key := p.Key
	if key == "" && p.AnilistID > 0 {
		key = store.AnimeKey(p.AnilistID)
	}
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload tidak valid"})
		return
	}
	err := s.db.SaveHistory(store.HistoryEntry{
		Key: key, Title: p.Title, Poster: p.Poster,
		Episode: p.Episode, Position: p.Position, Duration: p.Duration,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- schedule ----------

var wib = time.FixedZone("WIB", 7*3600)

var dayNames = []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
var monthNames = []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

type schedItem struct {
	Time  string
	Entry anilist.ScheduleEntry
}

type schedDay struct {
	Label string
	Items []schedItem
}

type jadwalData struct {
	baseData
	Days []schedDay
	Err  string
}

func (s *Server) schedulePage(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(wib)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	to := from.AddDate(0, 0, 7)
	entries, err := s.anilist.AiringSchedule(r.Context(), from, to)
	data := jadwalData{
		baseData: s.base()}
	if err != nil {
		data.Err = err.Error()
		s.render(w, "jadwal", data)
		return
	}
	byDay := map[string]*schedDay{}
	var order []string
	for _, e := range entries {
		local := e.AiringAt.In(wib)
		key := local.Format("2006-01-02")
		day, ok := byDay[key]
		if !ok {
			label := fmt.Sprintf("%s, %d %s", dayNames[int(local.Weekday())], local.Day(), monthNames[int(local.Month())])
			if key == from.Format("2006-01-02") {
				label = "Hari ini — " + label
			} else if key == from.AddDate(0, 0, 1).Format("2006-01-02") {
				label = "Besok — " + label
			}
			day = &schedDay{Label: label}
			byDay[key] = day
			order = append(order, key)
		}
		day.Items = append(day.Items, schedItem{Time: local.Format("15:04"), Entry: e})
	}
	for _, k := range order {
		data.Days = append(data.Days, *byDay[k])
	}
	s.render(w, "jadwal", data)
}

// ---------- settings ----------

type settingsData struct {
	baseData
	OtakuBase string
	HiBase    string
	IdlixBase string
	IdlixOn   bool
	LLMBase   string
	LLMModel  string
	LLMKeySet bool
	Version   string
	Latest    string
	Update    bool
	Saved     bool
	Err       string
}

// llmConfig reads the saved AI fansub (BYOK) endpoint settings.
func (s *Server) llmConfig() subtitle.LLMConfig {
	return subtitle.LLMConfig{
		BaseURL: s.db.GetSetting("llm_base"),
		APIKey:  s.db.GetSetting("llm_key"),
		Model:   s.db.GetSetting("llm_model"),
	}
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	latest := s.latestRelease(r.Context())
	data := settingsData{
		baseData:  s.base(),
		OtakuBase: s.otaku.Base(),
		HiBase:    s.hi.Base(),
		IdlixBase: s.idlixBase,
		IdlixOn:   s.idlixEnabled(),
		LLMBase:   s.db.GetSetting("llm_base"),
		LLMModel:  s.db.GetSetting("llm_model"),
		LLMKeySet: s.db.GetSetting("llm_key") != "",
		Version:   Version,
		Latest:    latest,
		Update:    latest != "" && latest != "v"+Version,
		Saved:     r.URL.Query().Get("saved") == "1",
	}
	s.render(w, "settings", data)
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form tidak valid", http.StatusBadRequest)
		return
	}
	// The settings page has one form per section; only persist the
	// section whose fields were actually submitted.
	if _, present := r.Form["otakudesu_base"]; present {
		base := strings.TrimSpace(r.FormValue("otakudesu_base"))
		if base != "" && !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "http://") {
			data := settingsData{
				baseData: s.base(), OtakuBase: s.otaku.Base(), Version: Version, Err: "Alamat sumber harus diawali http:// atau https://"}
			s.render(w, "settings", data)
			return
		}
		if err := s.db.SetSetting("otakudesu_base", base); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if base != "" {
			s.otaku.SetBase(base)
		}
		hiBase := strings.TrimSpace(r.FormValue("hianime_base"))
		if hiBase != "" && !strings.HasPrefix(hiBase, "https://") && !strings.HasPrefix(hiBase, "http://") {
			data := settingsData{baseData: s.base(), OtakuBase: s.otaku.Base(), HiBase: s.hi.Base(), IdlixBase: s.idlixBase, IdlixOn: s.idlixEnabled(), LLMBase: s.db.GetSetting("llm_base"), LLMModel: s.db.GetSetting("llm_model"), LLMKeySet: s.db.GetSetting("llm_key") != "", Version: Version, Err: "Alamat HiAnime harus diawali http:// atau https://"}
			s.render(w, "settings", data)
			return
		}
		_ = s.db.SetSetting("hianime_base", hiBase)
		if hiBase != "" {
			s.hi.SetBase(hiBase)
		}
		// A new source domain invalidates cached matches.
		s.mu.Lock()
		s.matches = map[string]matchEntry{}
		s.episodes = map[string][]source.Episode{}
		s.mu.Unlock()
	}
	if _, present := r.Form["llm_base"]; present {
		// AI fansub (BYOK) endpoint settings. The API key is only
		// overwritten when a new one is typed; an empty field keeps
		// the saved key, and the clear checkbox removes it.
		llmBase := strings.TrimSpace(r.FormValue("llm_base"))
		if llmBase != "" && !strings.HasPrefix(llmBase, "https://") && !strings.HasPrefix(llmBase, "http://") {
			data := settingsData{
				baseData: s.base(), OtakuBase: s.otaku.Base(), LLMBase: llmBase, LLMModel: r.FormValue("llm_model"), LLMKeySet: s.db.GetSetting("llm_key") != "", Version: Version, Err: "Alamat endpoint AI harus diawali http:// atau https://"}
			s.render(w, "settings", data)
			return
		}
		_ = s.db.SetSetting("llm_base", llmBase)
		_ = s.db.SetSetting("llm_model", strings.TrimSpace(r.FormValue("llm_model")))
		if r.FormValue("llm_key_clear") == "1" {
			_ = s.db.SetSetting("llm_key", "")
		} else if key := strings.TrimSpace(r.FormValue("llm_key")); key != "" {
			_ = s.db.SetSetting("llm_key", key)
		}
	}
	if _, present := r.Form["idlix_base"]; present {
		// IDLIX catalog (movies & series). The catalog itself arrives
		// in v0.10.0; the toggle already gates the Film/Series nav.
		idlixBase := strings.TrimSpace(r.FormValue("idlix_base"))
		if idlixBase != "" && !strings.HasPrefix(idlixBase, "https://") && !strings.HasPrefix(idlixBase, "http://") {
			data := settingsData{baseData: s.base(), OtakuBase: s.otaku.Base(), IdlixBase: idlixBase, IdlixOn: s.idlixEnabled(), LLMBase: s.db.GetSetting("llm_base"), LLMModel: s.db.GetSetting("llm_model"), LLMKeySet: s.db.GetSetting("llm_key") != "", Version: Version, Err: "Alamat IDLIX harus diawali http:// atau https://"}
			s.render(w, "settings", data)
			return
		}
		_ = s.db.SetSetting("idlix_base", idlixBase)
		if idlixBase != "" {
			s.idlixBase = idlixBase
			s.idlix.SetBase(idlixBase)
		}
		on := ""
		if r.FormValue("idlix_on") == "1" {
			on = "1"
		}
		_ = s.db.SetSetting("idlix_on", on)
	}
	http.Redirect(w, r, "/pengaturan?saved=1", http.StatusSeeOther)
}

// latestRelease returns the newest published release tag (cached 6h).
func (s *Server) latestRelease(ctx context.Context) string {
	s.relMu.Lock()
	defer s.relMu.Unlock()
	if s.latestTag != "" && time.Since(s.latestAt) < 6*time.Hour {
		return s.latestTag
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/wahyuadamhusaeni/naniplay/releases/latest", nil)
	if err != nil {
		return s.latestTag
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "NaniPlay/"+Version)
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return s.latestTag
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return s.latestTag
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err == nil && body.TagName != "" {
		s.latestTag = body.TagName
		s.latestAt = time.Now()
	}
	return s.latestTag
}

// ---------- small APIs ----------

func (s *Server) apiSkipTimes(w http.ResponseWriter, r *http.Request) {
	id, err1 := strconv.Atoi(r.PathValue("id"))
	ep, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parameter tidak valid"})
		return
	}
	a, err := s.anilist.Detail(r.Context(), id)
	if err != nil || a.IDMal == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "results": []any{}})
		return
	}
	results, err := s.aniskip.SkipTimes(r.Context(), a.IDMal, ep)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"found": false, "results": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"found": len(results) > 0, "results": results})
}

func (s *Server) apiHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": Version})
}

func (s *Server) apiVersion(w http.ResponseWriter, r *http.Request) {
	latest := s.latestRelease(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version,
		"latest":  latest,
		"update":  latest != "" && latest != "v"+Version,
	})
}

func (s *Server) apiGetHistory(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	ep, _ := strconv.Atoi(r.PathValue("ep"))
	h, err := s.db.HistoryFor(store.AnimeKey(id), ep)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if h == nil {
		writeJSON(w, http.StatusOK, map[string]any{"position": 0})
		return
	}
	writeJSON(w, http.StatusOK, h)
}
