// Package anilist is a small client for the public AniList GraphQL API,
// used for anime metadata: discovery rails, search, season charts and detail.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const endpoint = "https://graphql.anilist.co"

// Anime is the subset of AniList Media fields NaniPlay renders.
type Anime struct {
	ID          int      `json:"id"`
	Title       Title    `json:"title"`
	Cover       string   // coverImage.extraLarge
	Banner      string   // bannerImage
	Score       int      `json:"averageScore"`
	SeasonYear  int      `json:"seasonYear"`
	Season      string   `json:"season"`
	Format      string   `json:"format"`
	Episodes    int      `json:"episodes"`
	Duration    int      `json:"duration"` // minutes per episode
	Status      string   `json:"status"`
	Genres      []string `json:"genres"`
	Description string   `json:"description"`
	Studio      string   // main studio name
	NextEpisode int      // next airing episode number, 0 if none
}

// Title holds the AniList title variants.
type Title struct {
	Romaji  string `json:"romaji"`
	English string `json:"english"`
	Native  string `json:"native"`
}

// Display returns the best human-facing title.
func (a Anime) Display() string {
	if a.Title.English != "" {
		return a.Title.English
	}
	return a.Title.Romaji
}

type mediaRaw struct {
	ID          int    `json:"id"`
	Title       Title  `json:"title"`
	CoverImage  struct {
		ExtraLarge string `json:"extraLarge"`
	} `json:"coverImage"`
	BannerImage string   `json:"bannerImage"`
	Score       int      `json:"averageScore"`
	SeasonYear  int      `json:"seasonYear"`
	Season      string   `json:"season"`
	Format      string   `json:"format"`
	Episodes    int      `json:"episodes"`
	Duration    int      `json:"duration"`
	Status      string   `json:"status"`
	Genres      []string `json:"genres"`
	Description string   `json:"description"`
	Studios     struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"studios"`
	NextAiring *struct {
		Episode int `json:"episode"`
	} `json:"nextAiringEpisode"`
}

func (m mediaRaw) toAnime() Anime {
	a := Anime{
		ID: m.ID, Title: m.Title, Cover: m.CoverImage.ExtraLarge, Banner: m.BannerImage,
		Score: m.Score, SeasonYear: m.SeasonYear, Season: m.Season, Format: m.Format,
		Episodes: m.Episodes, Duration: m.Duration, Status: m.Status, Genres: m.Genres,
		Description: stripHTML(m.Description),
	}
	if len(m.Studios.Nodes) > 0 {
		a.Studio = m.Studios.Nodes[0].Name
	}
	if m.NextAiring != nil {
		a.NextEpisode = m.NextAiring.Episode
	}
	return a
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// Client calls the AniList API with a small in-memory cache, because the
// public rate limit is tight.
type Client struct {
	http  *http.Client
	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	body []byte
	at   time.Time
}

// New returns a ready client.
func New() *Client {
	return &Client{
		http:  &http.Client{Timeout: 20 * time.Second},
		cache: make(map[string]cacheEntry),
	}
}

const mediaFields = `
  id
  title { romaji english native }
  coverImage { extraLarge }
  bannerImage
  averageScore
  seasonYear
  season
  format
  episodes
  duration
  status
  genres
  description(asHtml: false)
  studios(isMain: true) { nodes { name } }
  nextAiringEpisode { episode }
`

type gqlResponse struct {
	Data struct {
		Page struct {
			Media []mediaRaw `json:"media"`
		} `json:"Page"`
		Media *mediaRaw `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) query(ctx context.Context, key, gql string, vars map[string]any, ttl time.Duration) (*gqlResponse, error) {
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < ttl {
		c.mu.Unlock()
		var r gqlResponse
		if err := json.Unmarshal(e.body, &r); err == nil {
			return &r, nil
		}
	}
	c.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"query": gql, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NaniPlay/0.1 (+https://github.com/wahyuadamhusaeni/naniplay)")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("anilist: rate limited, coba lagi sebentar")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anilist: HTTP %d", resp.StatusCode)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	var r gqlResponse
	if err := json.Unmarshal(body.Bytes(), &r); err != nil {
		return nil, err
	}
	if len(r.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", r.Errors[0].Message)
	}
	c.mu.Lock()
	c.cache[key] = cacheEntry{body: body.Bytes(), at: time.Now()}
	c.mu.Unlock()
	return &r, nil
}

func pageQuery() string {
	return `query ($page: Int, $perPage: Int, $sort: [MediaSort], $season: MediaSeason, $seasonYear: Int, $search: String) {
		Page(page: $page, perPage: $perPage) {
			media(type: ANIME, isAdult: false, sort: $sort, season: $season, seasonYear: $seasonYear, search: $search) {
				` + mediaFields + `
			}
		}
	}`
}

func (c *Client) page(ctx context.Context, key string, vars map[string]any) ([]Anime, error) {
	r, err := c.query(ctx, key, pageQuery(), vars, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	out := make([]Anime, 0, len(r.Data.Page.Media))
	for _, m := range r.Data.Page.Media {
		out = append(out, m.toAnime())
	}
	return out, nil
}

// Trending returns the currently trending anime.
func (c *Client) Trending(ctx context.Context, n int) ([]Anime, error) {
	return c.page(ctx, fmt.Sprintf("trending-%d", n), map[string]any{
		"page": 1, "perPage": n, "sort": []string{"TRENDING_DESC"},
	})
}

// SeasonNow returns popular anime of the current season.
func (c *Client) SeasonNow(ctx context.Context, n int) ([]Anime, error) {
	season, year := currentSeason()
	return c.page(ctx, fmt.Sprintf("season-%s-%d-%d", season, year, n), map[string]any{
		"page": 1, "perPage": n, "sort": []string{"POPULARITY_DESC"},
		"season": season, "seasonYear": year,
	})
}

// TopAllTime returns the highest rated anime of all time.
func (c *Client) TopAllTime(ctx context.Context, n int) ([]Anime, error) {
	return c.page(ctx, fmt.Sprintf("top-%d", n), map[string]any{
		"page": 1, "perPage": n, "sort": []string{"SCORE_DESC"},
	})
}

// Search finds anime by title.
func (c *Client) Search(ctx context.Context, q string, n int) ([]Anime, error) {
	return c.page(ctx, fmt.Sprintf("search-%s-%d", strings.ToLower(q), n), map[string]any{
		"page": 1, "perPage": n, "search": q, "sort": []string{"SEARCH_MATCH"},
	})
}

// Detail returns one anime by AniList ID.
func (c *Client) Detail(ctx context.Context, id int) (*Anime, error) {
	gql := `query ($id: Int) { Media(id: $id, type: ANIME) { ` + mediaFields + ` } }`
	r, err := c.query(ctx, fmt.Sprintf("detail-%d", id), gql, map[string]any{"id": id}, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	if r.Data.Media == nil {
		return nil, fmt.Errorf("anilist: anime %d tidak ditemukan", id)
	}
	a := r.Data.Media.toAnime()
	return &a, nil
}

func currentSeason() (string, int) {
	now := time.Now()
	year := now.Year()
	switch now.Month() {
	case time.December, time.January, time.February:
		if now.Month() == time.December {
			return "WINTER", year + 1
		}
		return "WINTER", year
	case time.March, time.April, time.May:
		return "SPRING", year
	case time.June, time.July, time.August:
		return "SUMMER", year
	default:
		return "FALL", year
	}
}

// CurrentSeason exposes the running season for display purposes.
func CurrentSeason() (string, int) { return currentSeason() }
