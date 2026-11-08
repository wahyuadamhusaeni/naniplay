package subtitle

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Candidate is one downloadable subtitle offered by a source.
type Candidate struct {
	ID        string `json:"id"`
	Source    string `json:"source"` // opensubtitles | animetosho
	Label     string `json:"label"`
	Lang      string `json:"lang"` // id | en | ...
	Format    string `json:"format"`
	URL       string `json:"url,omitempty"`
	Downloads int    `json:"downloads,omitempty"`
	Score     int    `json:"score"`
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

var httpClient = &http.Client{Timeout: 14 * time.Second}

func httpGet(ctx context.Context, rawURL, ua string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d dari %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

var langFromISO = map[string]string{
	"ind": "id", "id": "id", "eng": "en", "en": "en",
	"may": "ms", "msa": "ms", "ms": "ms", "jpn": "ja", "ja": "ja",
	"spa": "es", "fre": "fr", "ger": "de", "por": "pt", "pob": "pt",
	"kor": "ko", "chi": "zh", "zho": "zh", "zht": "zh",
}

var isoFromLang = map[string]string{"id": "ind", "en": "eng"}

// --- IMDb resolution ---------------------------------------------------

var (
	imdbCache   = map[string]imdbEntry{}
	imdbCacheMu sync.Mutex
)

type imdbEntry struct {
	id string
	at time.Time
}

// ResolveIMDb maps an anime title to its IMDb series id ("tt…") using
// the public Cinemeta catalog, falling back to IMDb's suggestion API.
func ResolveIMDb(ctx context.Context, title string) string {
	base, _ := BaseTitle(title)
	if base == "" {
		return ""
	}
	key := strings.ToLower(base)
	imdbCacheMu.Lock()
	if e, ok := imdbCache[key]; ok && time.Since(e.at) < 24*time.Hour {
		imdbCacheMu.Unlock()
		return e.id
	}
	imdbCacheMu.Unlock()

	found := ""
	// Cinemeta public catalog search.
	if raw, err := httpGet(ctx, "https://v3-cinemeta.strem.io/catalog/series/top/search="+url.QueryEscape(base)+".json", browserUA); err == nil {
		var data struct {
			Metas []struct {
				ID     string `json:"id"`
				ImdbID string `json:"imdb_id"`
				Name   string `json:"name"`
			} `json:"metas"`
		}
		if json.Unmarshal(raw, &data) == nil {
			for _, m := range data.Metas {
				id := m.ImdbID
				if id == "" {
					id = m.ID
				}
				if strings.HasPrefix(id, "tt") && TitleSimilarity(base, m.Name) >= 0.5 {
					found = id
					break
				}
			}
		}
	}
	// IMDb suggestion API fallback.
	if found == "" {
		first := "a"
		for _, r := range strings.ToLower(base) {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				first = string(r)
				break
			}
		}
		u := "https://v3.sg.media-imdb.com/suggestion/" + first + "/" + url.QueryEscape(base) + ".json"
		if raw, err := httpGet(ctx, u, browserUA); err == nil {
			var data struct {
				D []struct {
					ID string `json:"id"`
					L  string `json:"l"`
					Q  string `json:"q"`
				} `json:"d"`
			}
			if json.Unmarshal(raw, &data) == nil {
				for _, it := range data.D {
					if !strings.HasPrefix(it.ID, "tt") || strings.Contains(strings.ToLower(it.Q), "video game") {
						continue
					}
					if TitleSimilarity(base, it.L) >= 0.5 {
						found = it.ID
						break
					}
				}
			}
		}
	}
	imdbCacheMu.Lock()
	imdbCache[key] = imdbEntry{id: found, at: time.Now()}
	imdbCacheMu.Unlock()
	return found
}

// --- OpenSubtitles search (keyless public endpoints) -------------------

var (
	osCache   = map[string]osCacheEntry{}
	osCacheMu sync.Mutex
)

type osCacheEntry struct {
	results []Candidate
	at      time.Time
}

var fileIDInURL = regexp.MustCompile(`/file/(\d+)`)

// SearchOpenSubtitles finds Indonesian (then English) subtitles for a
// title/season/episode via OpenSubtitles' public Stremio v3 API and
// its legacy REST API — no API key required. Candidates are strictly
// validated against the title, season and episode, then scored.
func SearchOpenSubtitles(ctx context.Context, title string, season, episode int) []Candidate {
	base, inferred := BaseTitle(title)
	if season < 1 {
		season = inferred
	}
	if episode < 1 {
		episode = 1
	}
	cacheKey := fmt.Sprintf("%s|s%d|e%d", strings.ToLower(base), season, episode)
	osCacheMu.Lock()
	if e, ok := osCache[cacheKey]; ok && time.Since(e.at) < 30*time.Minute {
		osCacheMu.Unlock()
		return e.results
	}
	osCacheMu.Unlock()

	imdb := ResolveIMDb(ctx, title)
	byID := map[string]*Candidate{}

	add := func(fid, filename, metaTitle, rawLang string, downloads int) {
		if fid == "" {
			return
		}
		lang := langFromISO[strings.ToLower(rawLang)]
		if lang == "" {
			lang = strings.ToLower(rawLang)
		}
		if lang != "id" && lang != "en" {
			return
		}
		// Indonesian requests must never receive Malay subtitles.
		if lang == "ms" {
			return
		}
		if base != "" && !AccurateMatch(base, strings.TrimSpace(filename+" "+metaTitle), season, episode) {
			return
		}
		score := scoreCandidate(filename, season, episode, lang, downloads)
		if score < 0 {
			return
		}
		label := filename
		if label == "" {
			label = "OpenSubtitles #" + fid
		}
		c := &Candidate{
			ID: fid, Source: "opensubtitles", Label: label, Lang: lang,
			Format: "srt", Downloads: downloads, Score: score,
			URL: "https://dl.opensubtitles.org/en/download/subencoding-utf8/file/" + fid,
		}
		if old, ok := byID[fid]; !ok || score > old.Score {
			byID[fid] = c
		}
	}

	// Tier 1: Stremio-hosted OpenSubtitles v3 public API.
	if strings.HasPrefix(imdb, "tt") {
		seasons := []int{season}
		if season > 1 {
			seasons = append(seasons, 1)
		}
		for _, s := range seasons {
			u := fmt.Sprintf("https://opensubtitles-v3.strem.io/subtitles/series/%s:%d:%d.json", imdb, s, episode)
			raw, err := httpGet(ctx, u, browserUA)
			if err != nil {
				continue
			}
			var data struct {
				Subtitles []struct {
					ID   string          `json:"id"`
					URL  string          `json:"url"`
					Lang string          `json:"lang"`
					Name string          `json:"subtitleFileName"`
					Rel  string          `json:"movieReleaseName"`
					G    json.RawMessage `json:"g"`
				} `json:"subtitles"`
			}
			if json.Unmarshal(raw, &data) != nil {
				continue
			}
			for _, sub := range data.Subtitles {
				fid := sub.ID
				if m := fileIDInURL.FindStringSubmatch(sub.URL); m != nil {
					fid = m[1]
				}
				dl := 0
				var n int
				if json.Unmarshal(sub.G, &n) == nil {
					dl = n
				}
				name := sub.Name
				if name == "" {
					name = sub.Rel
				}
				add(fid, name, sub.Rel, sub.Lang, dl)
			}
		}
	}

	// Tier 2: legacy public REST API.
	numIMDb := strings.TrimPrefix(imdb, "tt")
	for _, iso := range []string{"ind", "eng"} {
		if iso == "eng" {
			hasID := false
			for _, c := range byID {
				if c.Lang == "id" {
					hasID = true
					break
				}
			}
			if hasID {
				break
			}
		}
		var urls []string
		if numIMDb != "" {
			urls = append(urls, fmt.Sprintf("https://rest.opensubtitles.org/search/episode-%d/imdbid-%s/season-%d/sublanguageid-%s", episode, numIMDb, season, iso))
		}
		for _, v := range TitleVariations(base)[:1] {
			urls = append(urls, fmt.Sprintf("https://rest.opensubtitles.org/search/episode-%d/query-%s/season-%d/sublanguageid-%s", episode, url.QueryEscape(strings.ToLower(v)), season, iso))
		}
		for _, u := range urls {
			raw, err := httpGet(ctx, u, "TemporaryUserAgent")
			if err != nil || !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
				continue
			}
			var items []struct {
				Fid       string `json:"IDSubtitleFile"`
				FileName  string `json:"SubFileName"`
				RelName   string `json:"MovieReleaseName"`
				MovieName string `json:"MovieName"`
				Lang      string `json:"SubLanguageID"`
				Downloads string `json:"SubDownloadsCnt"`
			}
			if json.Unmarshal(raw, &items) != nil {
				continue
			}
			for _, it := range items {
				dl, _ := strconv.Atoi(it.Downloads)
				add(it.Fid, it.FileName, it.RelName+" "+it.MovieName, it.Lang, dl)
			}
		}
	}

	results := []Candidate{}
	for _, c := range byID {
		results = append(results, *c)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if (results[i].Lang == "id") != (results[j].Lang == "id") {
			return results[i].Lang == "id"
		}
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Downloads > results[j].Downloads
	})
	if len(results) > 12 {
		results = results[:12]
	}
	if len(results) > 0 {
		osCacheMu.Lock()
		osCache[cacheKey] = osCacheEntry{results: results, at: time.Now()}
		osCacheMu.Unlock()
	}
	return results
}

var trustedTags = []string{"nf", "netflix", "cr", "crunchyroll", "webrip", "web-dl", "bstation", "muse", "judas", "subsplease", "erai", "asw"}

// scoreCandidate ranks a release filename for the wanted episode.
// A detected season/episode mismatch, or Malay content offered as
// Indonesian, scores -999 (reject).
func scoreCandidate(filename string, season, episode int, lang string, downloads int) int {
	low := strings.ToLower(filename)
	if cs := FileSeason(low); cs > 0 && cs != season {
		return -999
	}
	if ce := FileEpisode(low); ce > 0 && ce != episode {
		return -999
	}
	if lang == "id" && (strings.Contains(low, "malay") || strings.Contains(low, "[may") || strings.Contains(low, "(may")) {
		return -999
	}
	score := 50
	switch lang {
	case "id":
		score += 100
	case "en":
		score += 30
	}
	if FileSeason(low) == season {
		score += 30
	}
	if FileEpisode(low) == episode {
		score += 50
	}
	for _, tag := range trustedTags {
		if strings.Contains(low, tag) {
			score += 15
			break
		}
	}
	if d := downloads / 100; d > 30 {
		score += 30
	} else {
		score += d
	}
	return score
}

// DownloadOpenSubtitles fetches a subtitle file's text by its
// OpenSubtitles file id, trying the canonical download host and the
// Stremio mirror; responses may be gzipped.
func DownloadOpenSubtitles(ctx context.Context, fileID string) (string, error) {
	if fileID == "" {
		return "", fmt.Errorf("file id kosong")
	}
	urls := []string{
		"https://dl.opensubtitles.org/en/download/subencoding-utf8/file/" + fileID,
		"https://subs5.strem.io/en/download/subencoding-stremio-utf8/src-api/file/" + fileID,
	}
	var lastErr error
	for _, u := range urls {
		ua := browserUA
		if strings.Contains(u, "opensubtitles.org") {
			ua = "TemporaryUserAgent"
		}
		raw, err := httpGet(ctx, u, ua)
		if err != nil {
			lastErr = err
			continue
		}
		if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
			if zr, err := gzip.NewReader(bytes.NewReader(raw)); err == nil {
				if plain, err := io.ReadAll(zr); err == nil {
					raw = plain
				}
			}
		}
		text := bidiRe.ReplaceAllString(strings.ToValidUTF8(string(raw), ""), "")
		if LooksLikeSubtitle(text) {
			return text, nil
		}
		lastErr = fmt.Errorf("respons dari %s bukan subtitle", u)
	}
	return "", fmt.Errorf("gagal mengunduh subtitle OpenSubtitles: %w", lastErr)
}
