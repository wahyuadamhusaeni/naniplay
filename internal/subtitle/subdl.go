package subtitle

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SubDL (subdl.com) is a subtitle database for movies and TV series,
// used here for the cinema (IDLIX) side of NaniPlay — the same
// service the idlix-androidtv apps integrate. It needs a free user
// API key (stored in Settings, never rendered back).
//
// Search:
//
//	GET https://api.subdl.com/api/v1/subtitles?api_key=K
//	    &tmdb_id=|imdb_id=|film_name=&type=movie|tv
//	    &season_number=&episode_number=&languages=ID,EN
//	-> {status, subtitles: [{release_name, name, lang, language,
//	   author, url, season, episode}]}
//
// Download URLs are relative to https://dl.subdl.com and serve a
// ZIP archive containing the subtitle file.

const subdlAPIBase = "https://api.subdl.com/api/v1/subtitles"
const subdlDLBase = "https://dl.subdl.com"

type subdlSubtitle struct {
	ReleaseName string `json:"release_name"`
	Name        string `json:"name"`
	Lang        string `json:"lang"`
	Language    string `json:"language"`
	Author      string `json:"author"`
	URL         string `json:"url"`
}

type subdlResponse struct {
	Status    bool            `json:"status"`
	Subtitles []subdlSubtitle `json:"subtitles"`
}

// SubDLQuery identifies a movie or episode on SubDL. IMDb is
// preferred (IDLIX details carry it); Name is the fallback.
type SubDLQuery struct {
	IMDb    string
	Name    string
	Type    string // "movie" | "tv"
	Season  int
	Episode int
}

// SearchSubDL returns subtitle candidates for a movie/episode in
// Indonesian and English, best first.
func SearchSubDL(ctx context.Context, apiKey string, q SubDLQuery) ([]Candidate, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("kunci API SubDL belum diatur")
	}
	params := url.Values{}
	params.Set("api_key", apiKey)
	params.Set("languages", "ID,EN")
	typ := q.Type
	if typ == "" {
		typ = "movie"
	}
	params.Set("type", typ)
	switch {
	case strings.TrimSpace(q.IMDb) != "":
		params.Set("imdb_id", strings.TrimSpace(q.IMDb))
	case strings.TrimSpace(q.Name) != "":
		params.Set("film_name", strings.TrimSpace(q.Name))
	default:
		return nil, fmt.Errorf("tidak ada IMDb ID maupun judul untuk dicari")
	}
	if typ == "tv" {
		if q.Season > 0 {
			params.Set("season_number", fmt.Sprint(q.Season))
		}
		if q.Episode > 0 {
			params.Set("episode_number", fmt.Sprint(q.Episode))
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subdlAPIBase+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NaniPlay")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SubDL menjawab HTTP %s", resp.Status)
	}
	var sr subdlResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&sr); err != nil {
		return nil, err
	}
	out := []Candidate{}
	for _, sp := range sr.Subtitles {
		if sp.URL == "" {
			continue
		}
		dl := sp.URL
		if !strings.HasPrefix(dl, "http") {
			dl = subdlDLBase + dl
		}
		lang := strings.ToLower(strings.TrimSpace(sp.Language))
		if lang == "" {
			lang = strings.ToLower(strings.TrimSpace(sp.Lang))
		}
		bucket := "en"
		if lang == "id" || strings.Contains(lang, "indones") {
			bucket = "id"
		}
		label := sp.ReleaseName
		if label == "" {
			label = sp.Name
		}
		if sp.Author != "" {
			label += " (" + sp.Author + ")"
		}
		score := 30
		if bucket == "id" {
			score = 100
		}
		out = append(out, Candidate{
			ID:     dl,
			Source: "subdl",
			Label:  label,
			Lang:   bucket,
			Score:  score,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

// DownloadSubDL fetches one SubDL download URL and returns the
// subtitle text. SubDL serves ZIP archives; the first subtitle
// file inside wins. Only dl.subdl.com URLs are accepted — the URL
// arrives from the client (it came from our own search results),
// so the host allowlist keeps this from becoming a general fetcher.
func DownloadSubDL(ctx context.Context, rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host != "dl.subdl.com" {
		return "", fmt.Errorf("URL unduhan SubDL tidak valid")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "NaniPlay")
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("SubDL menjawab HTTP %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if len(data) >= 2 && data[0] == 'P' && data[1] == 'K' {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", fmt.Errorf("arsip SubDL tidak terbaca: %w", err)
		}
		for _, f := range zr.File {
			low := strings.ToLower(f.Name)
			if strings.HasSuffix(low, ".srt") || strings.HasSuffix(low, ".ass") ||
				strings.HasSuffix(low, ".ssa") || strings.HasSuffix(low, ".vtt") {
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				defer rc.Close()
				text, err := io.ReadAll(io.LimitReader(rc, 4<<20))
				if err != nil {
					return "", err
				}
				return string(text), nil
			}
		}
		return "", fmt.Errorf("arsip SubDL tidak berisi berkas subtitle")
	}
	return string(data), nil
}
