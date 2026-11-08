package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/anilist"
	"github.com/wahyuadamhusaeni/naniplay/internal/idlix"
	"github.com/wahyuadamhusaeni/naniplay/internal/proxy"
	"github.com/wahyuadamhusaeni/naniplay/internal/source"
	"github.com/wahyuadamhusaeni/naniplay/internal/store"
	"github.com/wahyuadamhusaeni/naniplay/internal/subtitle"
)

// Subtitle endpoints: softsub tracks are fetched or uploaded per
// episode, stored raw in SQLite, and served as WebVTT with an
// optional timing offset (manual desync correction).

// importStreamSubtitles downloads a stream's bundled soft-subtitle
// tracks (HiAnime) into the local track store — best-effort, English
// first, at most 4 — so they behave like any other saved track:
// selectable in the player and translatable via GTX. Tracks already
// imported for the episode are not fetched again.
func (s *Server) importStreamSubtitles(ctx context.Context, a *anilist.Anime, episode int, stream *source.Stream) {
	srcName := stream.SubSource
	if srcName == "" {
		srcName = "HiAnime"
	}
	srcKey := strings.ToLower(srcName)
	existing, err := s.db.SubtitleTracks(store.AnimeKey(a.ID), episode)
	if err != nil {
		return
	}
	have := map[string]bool{}
	for _, t := range existing {
		if t.Source == srcKey {
			have[t.Label] = true
		}
	}
	tracks := append([]source.SubTrack{}, stream.Subtitles...)
	sort.SliceStable(tracks, func(i, j int) bool {
		return tracks[i].Lang == "en" && tracks[j].Lang != "en"
	})
	imported := 0
	for _, tr := range tracks {
		if imported >= 4 {
			break
		}
		label := srcName + " · " + tr.Label
		if have[label] {
			continue
		}
		dlCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, tr.URL, nil)
		if err != nil {
			cancel()
			continue
		}
		req.Header.Set("User-Agent", "NaniPlay/"+Version)
		if stream.Referer != "" {
			req.Header.Set("Referer", stream.Referer)
		}
		resp, err := proxy.GuardedClient(30 * time.Second).Do(req)
		if err != nil {
			cancel()
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		cancel()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		text := string(raw)
		if _, err := subtitle.Parse(text); err != nil {
			continue
		}
		_ = s.db.SaveSubtitleTrack(store.SubtitleTrack{
			ID: newTrackID(), AnilistID: a.ID, Episode: episode,
			Label: label, Lang: tr.Lang, Source: srcKey, Content: text,
		})
		imported++
	}
}

func newTrackID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "sub_" + hex.EncodeToString(b[:])
}

func (s *Server) saveTrack(w http.ResponseWriter, anilistID, episode int, label, lang, source, content string) {
	if _, err := subtitle.Parse(content); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Isi file tidak terbaca sebagai subtitle: " + err.Error()})
		return
	}
	s.saveTrackKey(w, store.AnimeKey(anilistID), episode, label, lang, source, content)
}

// mediaKeyEp resolves the media key + episode for both route
// families: /api/subtitles/{id}/{ep} (anime) and
// /api/subtitles/k/{key}/{ep} (any media key, e.g. movie:/series:).
func mediaKeyEp(r *http.Request) (string, int, bool) {
	ep, err := strconv.Atoi(r.PathValue("ep"))
	if err != nil {
		return "", 0, false
	}
	if k := r.PathValue("key"); k != "" {
		return k, ep, true
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		return "", 0, false
	}
	return store.AnimeKey(id), ep, true
}

func (s *Server) saveTrackKey(w http.ResponseWriter, key string, episode int, label, lang, src, content string) {
	// Idempotent: fetching the same online candidate twice must not
	// stack duplicate rows in the panel.
	if existing, err := s.db.SubtitleTracks(key, episode); err == nil {
		for _, t := range existing {
			if t.Label == label && t.Source == src {
				writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "label": t.Label, "lang": t.Lang, "source": t.Source})
				return
			}
		}
	}
	if _, err := subtitle.Parse(content); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Isi file tidak terbaca sebagai subtitle: " + err.Error()})
		return
	}
	anilistID, _ := store.ParseAnimeKey(key)
	t := store.SubtitleTrack{
		ID: newTrackID(), Key: key, AnilistID: anilistID, Episode: episode,
		Label: label, Lang: lang, Source: src, Content: content,
	}
	if err := s.db.SaveSubtitleTrack(t); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "label": t.Label, "lang": t.Lang, "source": t.Source})
}

// importCinemaSubtitles downloads the subtitle files a resolved
// cinema stream offers into the store under the media key, so the
// player panel (and GTX/AI translation) works exactly like anime.
func (s *Server) importCinemaSubtitles(ctx context.Context, key string, episode int, subs []idlix.SubFile) {
	if len(subs) == 0 {
		return
	}
	existing, _ := s.db.SubtitleTracks(key, episode)
	seen := map[string]bool{}
	for _, t := range existing {
		if t.Source == "majorplay" {
			seen[t.Label] = true
		}
	}
	client := &http.Client{Timeout: 25 * time.Second}
	imported := 0
	for _, sub := range subs {
		if imported >= 4 || sub.Path == "" {
			continue
		}
		label := sub.Label
		if label == "" {
			label = sub.Lang
		}
		if label == "" || seen[label] {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.Path, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "NaniPlay/"+Version)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(raw))
		if _, err := subtitle.Parse(text); err != nil {
			continue
		}
		anilistID, _ := store.ParseAnimeKey(key)
		t := store.SubtitleTrack{
			ID: newTrackID(), Key: key, AnilistID: anilistID, Episode: episode,
			Label: label, Lang: sub.Lang, Source: "majorplay", Content: text,
		}
		if err := s.db.SaveSubtitleTrack(t); err == nil {
			seen[label] = true
			imported++
		}
	}
}

// apiSubtitleList returns saved tracks for an episode; with
// ?q=online it also searches OpenSubtitles (no API key needed).
func (s *Server) apiSubtitleList(w http.ResponseWriter, r *http.Request) {
	key, episode, ok := mediaKeyEp(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
	saved, err := s.db.SubtitleTracks(key, episode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	resp := map[string]any{"saved": saved}
	if r.URL.Query().Get("q") == "online" {
		if anilistID, isAnime := store.ParseAnimeKey(key); isAnime {
			resp["results"] = s.searchOnlineSubtitles(r.Context(), anilistID, episode)
		} else if s.db.GetSetting("subdl_key") == "" {
			resp["results"] = []subtitle.Candidate{}
			resp["note"] = "Pencarian subtitle film/series memakai SubDL — atur kunci API SubDL (gratis, subdl.com) di Pengaturan dulu."
		} else {
			resp["results"] = s.searchSubDLSubtitles(r.Context(), key, episode)
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) searchOnlineSubtitles(ctx context.Context, anilistID, episode int) []subtitle.Candidate {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	anime, err := s.anilist.Detail(ctx, anilistID)
	if err != nil || anime == nil {
		return []subtitle.Candidate{}
	}
	titles := []string{}
	if anime.Title.Romaji != "" {
		titles = append(titles, anime.Title.Romaji)
	}
	if anime.Title.English != "" && anime.Title.English != anime.Title.Romaji {
		titles = append(titles, anime.Title.English)
	}
	merged := map[string]subtitle.Candidate{}
	for _, title := range titles {
		_, season := subtitle.BaseTitle(title)
		for _, c := range subtitle.SearchOpenSubtitles(ctx, title, season, episode) {
			if old, ok := merged[c.ID]; !ok || c.Score > old.Score {
				merged[c.ID] = c
			}
		}
		if len(merged) > 0 {
			break
		}
	}
	out := []subtitle.Candidate{}
	for _, c := range merged {
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Lang == "id") != (out[j].Lang == "id") {
			return out[i].Lang == "id"
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// searchSubDLSubtitles finds cinema subtitles on SubDL using the
// IDLIX detail's IMDb id (falling back to the title). Cinema history
// encodes series episodes as season*1000+episode.
func (s *Server) searchSubDLSubtitles(ctx context.Context, key string, episode int) []subtitle.Candidate {
	apiKey := s.db.GetSetting("subdl_key")
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	q := subtitle.SubDLQuery{}
	switch {
	case strings.HasPrefix(key, "movie:"):
		d, err := s.idlix.Movie(ctx, strings.TrimPrefix(key, "movie:"))
		if err != nil || d == nil {
			return []subtitle.Candidate{}
		}
		q.Type = "movie"
		q.IMDb = d.ImdbID
		q.Name = d.Title
	case strings.HasPrefix(key, "indo:"):
		d, err := s.kita.Detail(ctx, strings.TrimPrefix(key, "indo:"), false)
		if err != nil || d == nil {
			return []subtitle.Candidate{}
		}
		q.Type = "movie"
		q.Name = d.Title
	case strings.HasPrefix(key, "indoser:"):
		d, err := s.kita.Detail(ctx, strings.TrimPrefix(key, "indoser:"), true)
		if err != nil || d == nil {
			return []subtitle.Candidate{}
		}
		q.Type = "tv"
		q.Name = d.Title
		q.Season = 1
		q.Episode = episode
	case strings.HasPrefix(key, "series:"):
		d, err := s.idlix.SeriesDetail(ctx, strings.TrimPrefix(key, "series:"))
		if err != nil || d == nil {
			return []subtitle.Candidate{}
		}
		q.Type = "tv"
		q.IMDb = d.ImdbID
		q.Name = d.Title
		q.Season = episode / 1000
		q.Episode = episode % 1000
	default:
		return []subtitle.Candidate{}
	}
	out, err := subtitle.SearchSubDL(ctx, apiKey, q)
	if err != nil {
		return []subtitle.Candidate{}
	}
	return out
}

// apiSubtitleFetch downloads one online candidate and saves it:
// OpenSubtitles for anime keys, SubDL for cinema keys.
func (s *Server) apiSubtitleFetch(w http.ResponseWriter, r *http.Request) {
	key, episode, ok := mediaKeyEp(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
	_, isAnime := store.ParseAnimeKey(key)
	var body struct {
		Source string `json:"source"`
		ID     string `json:"id"`
		Label  string `json:"label"`
		Lang   string `json:"lang"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body tidak valid"})
		return
	}
	if body.ID == "" || (body.Source != "opensubtitles" && body.Source != "subdl") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sumber tidak didukung"})
		return
	}
	if body.Source == "opensubtitles" && !isAnime {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "OpenSubtitles hanya untuk anime"})
		return
	}
	if body.Source == "subdl" && isAnime {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SubDL hanya untuk film & series"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	var text string
	var err error
	if body.Source == "subdl" {
		text, err = subtitle.DownloadSubDL(ctx, body.ID)
	} else {
		text, err = subtitle.DownloadOpenSubtitles(ctx, body.ID)
	}
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	label := body.Label
	if label == "" {
		label = body.Source + " " + body.ID
	}
	s.saveTrackKey(w, key, episode, label, body.Lang, body.Source, text)
}

// apiSubtitleExternal fetches a subtitle from a user-supplied URL.
func (s *Server) apiSubtitleExternal(w http.ResponseWriter, r *http.Request) {
	key, episode, ok := mediaKeyEp(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "body tidak valid"})
		return
	}
	if !strings.HasPrefix(body.URL, "https://") && !strings.HasPrefix(body.URL, "http://") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "URL harus diawali http:// atau https://"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, body.URL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "URL tidak valid"})
		return
	}
	req.Header.Set("User-Agent", "NaniPlay/"+Version)
	resp, err := proxy.GuardedClient(30 * time.Second).Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Gagal mengambil URL: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": fmt.Sprintf("URL membalas HTTP %d", resp.StatusCode)})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	text := string(raw)
	if !subtitle.LooksLikeSubtitle(text) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Isi URL bukan file subtitle"})
		return
	}
	label := body.URL
	if idx := strings.LastIndex(label, "/"); idx >= 0 && idx < len(label)-1 {
		label = label[idx+1:]
	}
	s.saveTrackKey(w, key, episode, label, "", "external", text)
}

// apiSubtitleUpload saves a user-uploaded subtitle file (max 2 MiB).
func (s *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request) {
	key, episode, ok := mediaKeyEp(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unggahan tidak valid"})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file tidak ditemukan"})
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil || len(raw) > 2<<20 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "file terlalu besar (maks 2 MiB)"})
		return
	}
	text := string(raw)
	if !subtitle.LooksLikeSubtitle(text) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "file bukan subtitle (.srt/.vtt/.ass)"})
		return
	}
	label := header.Filename
	if len(label) > 120 {
		label = label[:120]
	}
	s.saveTrackKey(w, key, episode, label, "", "upload", text)
}

// apiSubtitleTrack serves one saved track converted to WebVTT.
// ?offset=<seconds> shifts all timings (desync correction).
func (s *Server) apiSubtitleTrack(w http.ResponseWriter, r *http.Request) {
	track, err := s.db.SubtitleTrack(r.PathValue("trackID"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if track == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "track tidak ditemukan"})
		return
	}
	offset, _ := strconv.ParseFloat(strings.Replace(r.URL.Query().Get("offset"), ",", ".", 1), 64)
	cues, err := subtitle.Parse(track.Content)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	if track.Source == "opensubtitles" {
		cues = subtitle.StripAds(cues)
	}
	w.Header().Set("Content-Type", "text/vtt; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, subtitle.ToVTT(cues, offset))
}

// apiSubtitleTranslateAI translates a saved track into Indonesian
// with the user's own OpenAI-compatible endpoint (BYOK AI fansub)
// and saves the result as a new track.
func (s *Server) apiSubtitleTranslateAI(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("trackID")
	cfg := s.llmConfig()
	if !cfg.Configured() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Endpoint AI belum diatur — isi alamat endpoint dan nama model dulu di Pengaturan."})
		return
	}
	track, err := s.db.SubtitleTrack(trackID)
	if err != nil || track == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "Track tidak ditemukan"})
		return
	}
	cues, err := subtitle.Parse(track.Content)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Isi track tidak terbaca"})
		return
	}
	if track.Source == "opensubtitles" {
		cues = subtitle.StripAds(cues)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 600*time.Second)
	defer cancel()
	translated, err := subtitle.TranslateCuesLLM(ctx, cues, cfg)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Fansub AI gagal: " + err.Error()})
		return
	}
	key := track.Key
	if key == "" {
		key = store.AnimeKey(track.AnilistID)
	}
	newTrack := store.SubtitleTrack{
		ID:        newTrackID(),
		Key:       key,
		AnilistID: track.AnilistID,
		Episode:   track.Episode,
		Label:     track.Label + " — Indonesia (AI Fansub)",
		Lang:      "id",
		Source:    "llm",
		Content:   subtitle.ToVTT(translated, 0),
	}
	if err := s.db.SaveSubtitleTrack(newTrack); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": newTrack.ID, "label": newTrack.Label, "lang": newTrack.Lang, "source": newTrack.Source,
	})
}

// apiSubtitleTranslate machine-translates a saved track into
// Indonesian via the zero-key Google GTX endpoint and saves the
// result as a new track, leaving the original untouched.
func (s *Server) apiSubtitleTranslate(w http.ResponseWriter, r *http.Request) {
	track, err := s.db.SubtitleTrack(r.PathValue("trackID"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if track == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "track tidak ditemukan"})
		return
	}
	cues, err := subtitle.Parse(track.Content)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": err.Error()})
		return
	}
	if track.Source == "opensubtitles" {
		cues = subtitle.StripAds(cues)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 150*time.Second)
	defer cancel()
	translated, err := subtitle.TranslateCues(ctx, cues, "auto", "id")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "Terjemahan gagal: " + err.Error()})
		return
	}
	key := track.Key
	if key == "" {
		key = store.AnimeKey(track.AnilistID)
	}
	nt := store.SubtitleTrack{
		ID: newTrackID(), Key: key, AnilistID: track.AnilistID, Episode: track.Episode,
		Label: track.Label + " — Indonesia (GTX)", Lang: "id", Source: "gtx",
		Content: subtitle.ToVTT(translated, 0),
	}
	if err := s.db.SaveSubtitleTrack(nt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": nt.ID, "label": nt.Label, "lang": nt.Lang, "source": nt.Source})
}

// apiSubtitleDelete removes a saved track.
// apiSubtitleMerge combines the addressed track with another track
// of the same episode into a new bilingual track.
func (s *Server) apiSubtitleMerge(w http.ResponseWriter, r *http.Request) {
	track, err := s.db.SubtitleTrack(r.PathValue("trackID"))
	if err != nil || track == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "trek tidak ditemukan"})
		return
	}
	var body struct {
		OtherID string `json:"otherId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || body.OtherID == "" || body.OtherID == track.ID {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pilih trek kedua yang berbeda"})
		return
	}
	other, err := s.db.SubtitleTrack(body.OtherID)
	if err != nil || other == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "trek kedua tidak ditemukan"})
		return
	}
	if other.Key != track.Key || other.Episode != track.Episode {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "kedua trek harus dari episode yang sama"})
		return
	}
	a, err1 := subtitle.Parse(track.Content)
	b, err2 := subtitle.Parse(other.Content)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "isi trek tidak terbaca sebagai subtitle"})
		return
	}
	merged := subtitle.MergeCues(a, b)
	key := track.Key
	if key == "" {
		key = store.AnimeKey(track.AnilistID)
	}
	s.saveTrackKey(w, key, track.Episode, track.Label+" + "+other.Label, track.Lang, "merge", subtitle.ToVTT(merged, 0))
}

// apiSubtitleAutosync estimates the addressed track's timing offset
// against a reference track and, when a real offset (>= 0.25 s) is
// found, saves a corrected copy. The response reports the estimate.
func (s *Server) apiSubtitleAutosync(w http.ResponseWriter, r *http.Request) {
	track, err := s.db.SubtitleTrack(r.PathValue("trackID"))
	if err != nil || track == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "trek tidak ditemukan"})
		return
	}
	var body struct {
		ReferenceID string `json:"referenceId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil || body.ReferenceID == "" || body.ReferenceID == track.ID {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pilih trek referensi yang berbeda"})
		return
	}
	ref, err := s.db.SubtitleTrack(body.ReferenceID)
	if err != nil || ref == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "trek referensi tidak ditemukan"})
		return
	}
	if ref.Key != track.Key || ref.Episode != track.Episode {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "trek referensi harus dari episode yang sama"})
		return
	}
	refCues, err1 := subtitle.Parse(ref.Content)
	tgtCues, err2 := subtitle.Parse(track.Content)
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "isi trek tidak terbaca sebagai subtitle"})
		return
	}
	off := subtitle.EstimateOffset(refCues, tgtCues)
	if off > -0.25 && off < 0.25 {
		writeJSON(w, http.StatusOK, map[string]any{"offset": off, "applied": false})
		return
	}
	key := track.Key
	if key == "" {
		key = store.AnimeKey(track.AnilistID)
	}
	label := track.Label + fmt.Sprintf(" (sinkron %+.1f dtk)", off)
	t := store.SubtitleTrack{
		ID: newTrackID(), Key: key, AnilistID: track.AnilistID, Episode: track.Episode,
		Label: label, Lang: track.Lang, Source: "autosync", Content: subtitle.ToVTT(tgtCues, off),
	}
	if err := s.db.SaveSubtitleTrack(t); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"offset": off, "applied": true, "id": t.ID, "label": t.Label})
}

func (s *Server) apiSubtitleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteSubtitleTrack(r.PathValue("trackID")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
