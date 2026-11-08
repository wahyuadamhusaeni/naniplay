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
	"github.com/wahyuadamhusaeni/naniplay/internal/proxy"
	"github.com/wahyuadamhusaeni/naniplay/internal/source"
	"github.com/wahyuadamhusaeni/naniplay/internal/source/otakudesu"
	"github.com/wahyuadamhusaeni/naniplay/internal/store"
)

//go:embed templates static
var assets embed.FS

// Version is the running build version; main overrides it via -ldflags.
var Version = "0.2.0"

// Server bundles the HTTP handlers and their dependencies.
type Server struct {
	anilist *anilist.Client
	aniskip *aniskip.Client
	otaku   *otakudesu.Client
	db      *store.Store
	tpl     map[string]*template.Template

	mu       sync.Mutex
	matches  map[int]matchEntry // anilist id -> otakudesu match
	episodes map[int][]source.Episode

	relMu     sync.Mutex
	latestTag string
	latestAt  time.Time
}

type matchEntry struct {
	series *source.Series
	at     time.Time
}

// New builds a Server. A saved otakudesu_base setting overrides the
// configured default, so a source domain change never needs a rebuild.
func New(db *store.Store, otakuBase string) (*Server, error) {
	if saved := db.GetSetting("otakudesu_base"); saved != "" {
		otakuBase = saved
	}
	s := &Server{
		anilist:  anilist.New(),
		aniskip:  aniskip.New(),
		otaku:    otakudesu.New(otakuBase),
		db:       db,
		tpl:      map[string]*template.Template{},
		matches:  map[int]matchEntry{},
		episodes: map[int][]source.Episode{},
	}
	pages := []string{"home", "detail", "watch", "search", "list", "notfound", "jadwal", "settings"}
	for _, p := range pages {
		t, err := template.New("base").Funcs(template.FuncMap{
			"pct": func(f float64) int { return int(f * 100) },
		}).ParseFS(assets, "templates/base.html", "templates/"+p+".html")
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", p, err)
		}
		s.tpl[p] = t
	}
	return s, nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(assets))

	mux.HandleFunc("GET /", s.home)
	mux.HandleFunc("GET /anime/{id}", s.detail)
	mux.HandleFunc("GET /watch/{id}/{ep}", s.watch)
	mux.HandleFunc("GET /search", s.searchPage)
	mux.HandleFunc("GET /jadwal", s.schedulePage)
	mux.HandleFunc("GET /daftar-saya", s.myList)
	mux.HandleFunc("GET /riwayat", s.historyPage)
	mux.HandleFunc("GET /pengaturan", s.settingsPage)
	mux.HandleFunc("POST /pengaturan", s.saveSettings)

	mux.HandleFunc("POST /api/history", s.apiSaveHistory)
	mux.HandleFunc("GET /api/history/{id}/{ep}", s.apiGetHistory)
	mux.HandleFunc("GET /api/skiptimes/{id}/{ep}", s.apiSkipTimes)
	mux.HandleFunc("GET /api/subtitles/{id}/{ep}", s.apiSubtitleList)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/fetch", s.apiSubtitleFetch)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/external", s.apiSubtitleExternal)
	mux.HandleFunc("POST /api/subtitles/{id}/{ep}/upload", s.apiSubtitleUpload)
	mux.HandleFunc("GET /api/subtitles/track/{trackID}", s.apiSubtitleTrack)
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

type homeData struct {
	Hero       *anilist.Anime
	Trending   []anilist.Anime
	Season     []anilist.Anime
	SeasonName string
	Year       int
	Top        []anilist.Anime
	Continue   []store.HistoryEntry
	Favorites  []store.Favorite
	Err        string
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.render(w, "notfound", nil)
		return
	}
	ctx := r.Context()
	data := homeData{}
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
		data.Continue = cw
	}
	if favs, err := s.db.Favorites(); err == nil {
		data.Favorites = favs
	}
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
	Anime      *anilist.Anime
	IsFavorite bool
	Series     *source.Series
	Episodes   []source.Episode
	SourceErr  string
	Continue   *store.HistoryEntry
	Relations  []anilist.Relation
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a, err := s.anilist.Detail(r.Context(), id)
	if err != nil {
		s.render(w, "notfound", nil)
		return
	}
	data := detailData{Anime: a, IsFavorite: s.db.IsFavorite(id)}
	if rels, err := s.anilist.Relations(r.Context(), id); err == nil {
		data.Relations = rels
	}
	series, eps := s.matchEpisodes(r.Context(), a)
	if series != nil {
		data.Series = series
		data.Episodes = eps
	} else {
		data.SourceErr = "Belum ditemukan di sumber streaming (Otakudesu)."
	}
	if cw, err := s.db.ContinueWatching(50); err == nil {
		for i := range cw {
			if cw[i].AnilistID == id {
				data.Continue = &cw[i]
				break
			}
		}
	}
	s.render(w, "detail", data)
}

// matchEpisodes resolves (and caches) the Otakudesu series + episodes
// for an AniList anime.
func (s *Server) matchEpisodes(ctx context.Context, a *anilist.Anime) (*source.Series, []source.Episode) {
	s.mu.Lock()
	if m, ok := s.matches[a.ID]; ok && time.Since(m.at) < 30*time.Minute {
		eps := s.episodes[a.ID]
		s.mu.Unlock()
		return m.series, eps
	}
	s.mu.Unlock()

	series, eps, err := s.otaku.Match(ctx, []string{a.Title.Romaji, a.Title.English}, a.Episodes)
	if err != nil || series == nil {
		return nil, nil
	}
	s.mu.Lock()
	s.matches[a.ID] = matchEntry{series: series, at: time.Now()}
	s.episodes[a.ID] = eps
	s.mu.Unlock()
	return series, eps
}

type watchData struct {
	Anime     *anilist.Anime
	Episode   int
	Stream    *source.Stream
	ProxyURL  string
	Episodes  []source.Episode
	PrevEp    int
	NextEp    int
	SavedPos  float64
	SourceErr string
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
		s.render(w, "notfound", nil)
		return
	}
	data := watchData{Anime: a, Episode: ep}
	series, eps := s.matchEpisodes(r.Context(), a)
	if series == nil {
		data.SourceErr = "Sumber streaming untuk judul ini belum ditemukan."
		s.render(w, "watch", data)
		return
	}
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
	stream, err := s.otaku.Resolve(r.Context(), target.URL)
	if err != nil {
		data.SourceErr = "Gagal menyiapkan stream: " + err.Error()
		s.render(w, "watch", data)
		return
	}
	data.Stream = stream
	data.ProxyURL = proxyURLFor(stream)
	if h, err := s.db.HistoryFor(id, ep); err == nil && h != nil {
		data.SavedPos = h.Position
	}
	s.render(w, "watch", data)
}

func proxyURLFor(st *source.Stream) string {
	q := "u=" + strings.ReplaceAll(strings.ReplaceAll(st.MasterURL, "%", "%25"), "&", "%26")
	return "/api/proxy?" + q + "&r=" + strings.ReplaceAll(st.Referer, "&", "%26")
}

type searchData struct {
	Query   string
	Results []anilist.Anime
	Err     string
}

func (s *Server) searchPage(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := searchData{Query: q}
	if q != "" {
		res, err := s.anilist.Search(r.Context(), q, 24)
		if err != nil {
			data.Err = err.Error()
		} else {
			data.Results = res
		}
	}
	s.render(w, "search", data)
}

type listData struct {
	Title     string
	Favorites []store.Favorite
	History   []store.HistoryEntry
}

func (s *Server) myList(w http.ResponseWriter, r *http.Request) {
	favs, _ := s.db.Favorites()
	s.render(w, "list", listData{Title: "Daftar Saya", Favorites: favs})
}

func (s *Server) historyPage(w http.ResponseWriter, r *http.Request) {
	hist, _ := s.db.AllHistory(60)
	s.render(w, "list", listData{Title: "Riwayat Tontonan", History: hist})
}

// ---------- actions & API ----------

func (s *Server) toggleFavorite(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form tidak valid", http.StatusBadRequest)
		return
	}
	id, _ := strconv.Atoi(r.FormValue("anilist_id"))
	if id == 0 {
		http.Error(w, "id tidak valid", http.StatusBadRequest)
		return
	}
	_, err := s.db.ToggleFavorite(store.Favorite{
		AnilistID: id,
		Title:     r.FormValue("title"),
		Poster:    r.FormValue("poster"),
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
	AnilistID int     `json:"anilistId"`
	Title     string  `json:"title"`
	Poster    string  `json:"poster"`
	Episode   int     `json:"episode"`
	Position  float64 `json:"position"`
	Duration  float64 `json:"duration"`
}

func (s *Server) apiSaveHistory(w http.ResponseWriter, r *http.Request) {
	var p historyPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.AnilistID == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload tidak valid"})
		return
	}
	err := s.db.SaveHistory(store.HistoryEntry{
		AnilistID: p.AnilistID, Title: p.Title, Poster: p.Poster,
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
	Days []schedDay
	Err  string
}

func (s *Server) schedulePage(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(wib)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	to := from.AddDate(0, 0, 7)
	entries, err := s.anilist.AiringSchedule(r.Context(), from, to)
	data := jadwalData{}
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
	OtakuBase string
	Version   string
	Latest    string
	Update    bool
	Saved     bool
	Err       string
}

func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	latest := s.latestRelease(r.Context())
	data := settingsData{
		OtakuBase: s.otaku.Base(),
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
	base := strings.TrimSpace(r.FormValue("otakudesu_base"))
	if base != "" && !strings.HasPrefix(base, "https://") && !strings.HasPrefix(base, "http://") {
		data := settingsData{OtakuBase: s.otaku.Base(), Version: Version, Err: "Alamat sumber harus diawali http:// atau https://"}
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
	// A new source domain invalidates cached matches.
	s.mu.Lock()
	s.matches = map[int]matchEntry{}
	s.episodes = map[int][]source.Episode{}
	s.mu.Unlock()
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
	h, err := s.db.HistoryFor(id, ep)
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
