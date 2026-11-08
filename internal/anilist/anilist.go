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
	ID           int      `json:"id"`
	IDMal        int      `json:"idMal"`
	Title        Title    `json:"title"`
	Cover        string   // coverImage.extraLarge
	Banner       string   // bannerImage
	Score        int      `json:"averageScore"`
	SeasonYear   int      `json:"seasonYear"`
	Season       string   `json:"season"`
	Format       string   `json:"format"`
	Episodes     int      `json:"episodes"`
	Duration     int      `json:"duration"` // minutes per episode
	Status       string   `json:"status"`
	Genres       []string `json:"genres"`
	Description  string   `json:"description"`
	Studio       string   // main studio name
	NextEpisode  int      // next airing episode number, 0 if none
	NextAiringAt int64    // unix seconds of the next airing, 0 if none
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
	ID         int   `json:"id"`
	IDMal      int   `json:"idMal"`
	Title      Title `json:"title"`
	CoverImage struct {
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
		Episode  int   `json:"episode"`
		AiringAt int64 `json:"airingAt"`
	} `json:"nextAiringEpisode"`
}

func (m mediaRaw) toAnime() Anime {
	a := Anime{
		ID: m.ID, IDMal: m.IDMal, Title: m.Title, Cover: m.CoverImage.ExtraLarge, Banner: m.BannerImage,
		Score: m.Score, SeasonYear: m.SeasonYear, Season: m.Season, Format: m.Format,
		Episodes: m.Episodes, Duration: m.Duration, Status: m.Status, Genres: m.Genres,
		Description: stripHTML(m.Description),
	}
	if len(m.Studios.Nodes) > 0 {
		a.Studio = m.Studios.Nodes[0].Name
	}
	if m.NextAiring != nil {
		a.NextEpisode = m.NextAiring.Episode
		a.NextAiringAt = m.NextAiring.AiringAt
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
	ttl  time.Duration
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
  idMal
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
  nextAiringEpisode { episode airingAt }
`

type gqlResponse struct {
	Data struct {
		Page struct {
			PageInfo struct {
				HasNextPage bool `json:"hasNextPage"`
			} `json:"pageInfo"`
			Media           []mediaRaw `json:"media"`
			AiringSchedules []struct {
				AiringAt int64    `json:"airingAt"`
				Episode  int      `json:"episode"`
				Media    mediaRaw `json:"media"`
			} `json:"airingSchedules"`
		} `json:"Page"`
		Media *mediaRaw `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// rawQuery performs a GraphQL call and returns the raw response body,
// served from the in-memory cache while fresh.
func (c *Client) rawQuery(ctx context.Context, key, gql string, vars map[string]any, ttl time.Duration) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.cache[key]; ok && time.Since(e.at) < ttl {
		body := e.body
		c.mu.Unlock()
		return body, nil
	}
	c.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"query": gql, "variables": vars})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "NaniPlay/0.2 (+https://github.com/wahyuadamhusaeni/naniplay)")
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
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	var probe struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(buf.Bytes(), &probe); err == nil && len(probe.Errors) > 0 {
		return nil, fmt.Errorf("anilist: %s", probe.Errors[0].Message)
	}
	c.mu.Lock()
	// Keep the cache bounded: drop entries past their own TTL on
	// every write (keys include arbitrary user search text, so
	// without eviction the map grows for the life of the process).
	now := time.Now()
	for k, e := range c.cache {
		if now.Sub(e.at) > e.ttl {
			delete(c.cache, k)
		}
	}
	if len(c.cache) > 2000 {
		c.cache = make(map[string]cacheEntry)
	}
	c.cache[key] = cacheEntry{body: buf.Bytes(), at: now, ttl: ttl}
	c.mu.Unlock()
	return buf.Bytes(), nil
}

func (c *Client) query(ctx context.Context, key, gql string, vars map[string]any, ttl time.Duration) (*gqlResponse, error) {
	body, err := c.rawQuery(ctx, key, gql, vars, ttl)
	if err != nil {
		return nil, err
	}
	var r gqlResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
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

// ScheduleEntry is one upcoming episode airing.
type ScheduleEntry struct {
	AiringAt time.Time
	Episode  int
	Anime    Anime
}

// AiringSchedule returns episode airings between from and to (inclusive),
// ordered by time. Results are cached for three hours.
func (c *Client) AiringSchedule(ctx context.Context, from, to time.Time) ([]ScheduleEntry, error) {
	gql := `query ($page: Int, $from: Int, $to: Int) {
		Page(page: $page, perPage: 50) {
			airingSchedules(airingAt_greater: $from, airingAt_lesser: $to, sort: TIME) {
				airingAt
				episode
				media { ` + mediaFields + ` }
			}
		}
	}`
	out := []ScheduleEntry{}
	for page := 1; page <= 16; page++ {
		key := fmt.Sprintf("sched-%d-%d-%d", from.Unix(), to.Unix(), page)
		r, err := c.query(ctx, key, gql, map[string]any{
			"page": page, "from": int(from.Unix()), "to": int(to.Unix()),
		}, 3*time.Hour)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		for _, s := range r.Data.Page.AiringSchedules {
			out = append(out, ScheduleEntry{
				AiringAt: time.Unix(s.AiringAt, 0),
				Episode:  s.Episode,
				Anime:    s.Media.toAnime(),
			})
		}
		if len(r.Data.Page.AiringSchedules) < 50 {
			break
		}
	}
	return out, nil
}

// LatestEpisodes returns recently aired episodes, newest first
// (one page of the 50 latest airings in the past 72 hours; the
// caller dedupes per anime). Cached for ten minutes.
func (c *Client) LatestEpisodes(ctx context.Context) ([]ScheduleEntry, error) {
	now := time.Now()
	gql := `query ($from: Int, $to: Int) {
		Page(page: 1, perPage: 50) {
			airingSchedules(airingAt_greater: $from, airingAt_lesser: $to, sort: TIME_DESC) {
				airingAt
				episode
				media { ` + mediaFields + ` }
			}
		}
	}`
	r, err := c.query(ctx, "latest-eps", gql, map[string]any{
		"from": int(now.Add(-72 * time.Hour).Unix()), "to": int(now.Unix()),
	}, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	out := []ScheduleEntry{}
	for _, e := range r.Data.Page.AiringSchedules {
		out = append(out, ScheduleEntry{
			AiringAt: time.Unix(e.AiringAt, 0),
			Episode:  e.Episode,
			Anime:    e.Media.toAnime(),
		})
	}
	return out, nil
}

// Relation is an anime related to another (sequel, prequel, side story...).
type Relation struct {
	Type  string // AniList relationType, e.g. SEQUEL, PREQUEL
	Anime Anime
}

// Relations returns the relations of one anime, sequels and prequels first.
func (c *Client) Relations(ctx context.Context, id int) ([]Relation, error) {
	gql := `query ($id: Int) {
		Media(id: $id, type: ANIME) {
			relations {
				edges {
					relationType
					node { ` + mediaFields + ` }
				}
			}
		}
	}`
	var raw struct {
		Data struct {
			Media *struct {
				Relations struct {
					Edges []struct {
						RelationType string   `json:"relationType"`
						Node         mediaRaw `json:"node"`
					} `json:"edges"`
				} `json:"relations"`
			} `json:"Media"`
		} `json:"data"`
	}
	body, err := c.rawQuery(ctx, fmt.Sprintf("rel-%d", id), gql, map[string]any{"id": id}, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.Data.Media == nil {
		return nil, nil
	}
	priority := map[string]int{"SEQUEL": 0, "PREQUEL": 1, "PARENT": 2, "SIDE_STORY": 3, "ALTERNATIVE": 4}
	out := []Relation{}
	for _, e := range raw.Data.Media.Relations.Edges {
		a := e.Node.toAnime()
		if a.Format != "TV" && a.Format != "MOVIE" && a.Format != "ONA" && a.Format != "TV_SHORT" {
			continue // skip manga/novel/music relations
		}
		out = append(out, Relation{Type: e.RelationType, Anime: a})
	}
	sortRelations(out, priority)
	return out, nil
}

func sortRelations(r []Relation, priority map[string]int) {
	for i := 1; i < len(r); i++ {
		for j := i; j > 0; j-- {
			pi, pj := priority[r[j].Type], priority[r[j-1].Type]
			if pi >= pj {
				break
			}
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
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

// Genres is the canonical AniList genre list.
var Genres = []string{
	"Action", "Adventure", "Comedy", "Drama", "Ecchi", "Fantasy",
	"Hentai", "Horror", "Mahou Shoujo", "Mecha", "Music", "Mystery",
	"Psychological", "Romance", "Sci-Fi", "Slice of Life", "Sports",
	"Supernatural", "Thriller",
}

// ByGenre lists anime of one genre, most popular first, with a
// has-next-page flag for paging.
func (c *Client) ByGenre(ctx context.Context, genre string, page int) ([]Anime, bool, error) {
	if page < 1 {
		page = 1
	}
	gql := `query ($page: Int, $perPage: Int, $genre: String) {
		Page(page: $page, perPage: $perPage) {
			pageInfo { hasNextPage }
			media(type: ANIME, isAdult: false, sort: POPULARITY_DESC, genre_in: [$genre]) {
				` + mediaFields + `
			}
		}
	}`
	r, err := c.query(ctx, fmt.Sprintf("genre-%s-%d", genre, page), gql, map[string]any{
		"page": page, "perPage": 24, "genre": genre,
	}, 10*time.Minute)
	if err != nil {
		return nil, false, err
	}
	out := make([]Anime, 0, len(r.Data.Page.Media))
	for _, m := range r.Data.Page.Media {
		out = append(out, m.toAnime())
	}
	return out, r.Data.Page.PageInfo.HasNextPage, nil
}
