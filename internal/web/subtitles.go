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
	existing, err := s.db.SubtitleTracks(a.ID, episode)
	if err != nil {
		return
	}
	have := map[string]bool{}
	for _, t := range existing {
		if t.Source == "hianime" {
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
		label := "HiAnime · " + tr.Label
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
		resp, err := http.DefaultClient.Do(req)
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
			Label: label, Lang: tr.Lang, Source: "hianime", Content: text,
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
	t := store.SubtitleTrack{
		ID: newTrackID(), AnilistID: anilistID, Episode: episode,
		Label: label, Lang: lang, Source: source, Content: content,
	}
	if err := s.db.SaveSubtitleTrack(t); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": t.ID, "label": t.Label, "lang": t.Lang, "source": t.Source})
}

// apiSubtitleList returns saved tracks for an episode; with
// ?q=online it also searches OpenSubtitles (no API key needed).
func (s *Server) apiSubtitleList(w http.ResponseWriter, r *http.Request) {
	anilistID, err1 := strconv.Atoi(r.PathValue("id"))
	episode, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
	saved, err := s.db.SubtitleTracks(anilistID, episode)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	resp := map[string]any{"saved": saved}
	if r.URL.Query().Get("q") == "online" {
		resp["results"] = s.searchOnlineSubtitles(r.Context(), anilistID, episode)
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

// apiSubtitleFetch downloads one online candidate and saves it.
func (s *Server) apiSubtitleFetch(w http.ResponseWriter, r *http.Request) {
	anilistID, err1 := strconv.Atoi(r.PathValue("id"))
	episode, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id/episode tidak valid"})
		return
	}
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
	if body.Source != "opensubtitles" || body.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sumber tidak didukung"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	text, err := subtitle.DownloadOpenSubtitles(ctx, body.ID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	label := body.Label
	if label == "" {
		label = "OpenSubtitles " + body.ID
	}
	s.saveTrack(w, anilistID, episode, label, body.Lang, "opensubtitles", text)
}

// apiSubtitleExternal fetches a subtitle from a user-supplied URL.
func (s *Server) apiSubtitleExternal(w http.ResponseWriter, r *http.Request) {
	anilistID, err1 := strconv.Atoi(r.PathValue("id"))
	episode, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
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
	resp, err := http.DefaultClient.Do(req)
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
	s.saveTrack(w, anilistID, episode, label, "", "external", text)
}

// apiSubtitleUpload saves a user-uploaded subtitle file (max 2 MiB).
func (s *Server) apiSubtitleUpload(w http.ResponseWriter, r *http.Request) {
	anilistID, err1 := strconv.Atoi(r.PathValue("id"))
	episode, err2 := strconv.Atoi(r.PathValue("ep"))
	if err1 != nil || err2 != nil {
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
	s.saveTrack(w, anilistID, episode, label, "", "upload", text)
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
	nt := store.SubtitleTrack{
		ID: newTrackID(), AnilistID: track.AnilistID, Episode: track.Episode,
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
func (s *Server) apiSubtitleDelete(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteSubtitleTrack(r.PathValue("trackID")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
