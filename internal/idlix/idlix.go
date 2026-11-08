// Package idlix is a client for the IDLIX site's own JSON API
// (movies & series catalog), implemented clean-room in pure Go.
//
// The site sits behind Cloudflare bot scoring that rejects ordinary
// HTTP clients with a "Just a moment" page. The gate is fingerprint
// based, so requests are made with a Chrome TLS/HTTP2 fingerprint
// (bogdanfinn/tls-client, pure Go) — verified live 2026-11-08:
// /api/movies and /api/genres answer 200 JSON while curl gets 403.
package idlix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// ErrChallenged means Cloudflare served an interstitial instead of the
// API response (fingerprint rejected or session flagged).
var ErrChallenged = errors.New("idlix: tantangan Cloudflare (Just a moment)")

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Fetcher abstracts the HTTP transport so tests can serve fixtures.
type Fetcher interface {
	Get(ctx context.Context, url string) (status int, body []byte, err error)
}

type tlsFetcher struct {
	client tls_client.HttpClient
	base   string
}

func newTLSFetcher(base string) (*tlsFetcher, error) {
	c, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_131),
		tls_client.WithNotFollowRedirects(),
	)
	if err != nil {
		return nil, err
	}
	return &tlsFetcher{client: c, base: base}, nil
}

func (f *tlsFetcher) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header = http.Header{
		"accept":            {"application/json, text/plain, */*"},
		"accept-language":   {"id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
		"referer":           {f.base + "/"},
		"user-agent":        {userAgent},
		http.HeaderOrderKey: {"accept", "accept-language", "referer", "user-agent"},
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, body, err
}

// Client is the IDLIX catalog client with a small in-memory cache.
type Client struct {
	base string
	f    Fetcher

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	body []byte
	at   time.Time
}

// New builds a client for the given IDLIX base URL
// (e.g. https://z2.idlixku.com).
func New(base string) (*Client, error) {
	base = strings.TrimRight(base, "/")
	f, err := newTLSFetcher(base)
	if err != nil {
		return nil, err
	}
	return &Client{base: base, f: f, cache: map[string]cacheEntry{}}, nil
}

// NewWithFetcher builds a client over a custom transport (tests).
func NewWithFetcher(base string, f Fetcher) *Client {
	return &Client{base: strings.TrimRight(base, "/"), f: f, cache: map[string]cacheEntry{}}
}

// Base returns the configured base URL.
func (c *Client) Base() string { return c.base }

// SetBase swaps the base URL and drops cached responses.
func (c *Client) SetBase(base string) {
	c.base = strings.TrimRight(base, "/")
	if tf, ok := c.f.(*tlsFetcher); ok {
		tf.base = c.base
	}
	c.mu.Lock()
	c.cache = map[string]cacheEntry{}
	c.mu.Unlock()
}

// getJSON fetches path (relative to base) and unmarshals into out.
// Responses are cached for ttl; a zero ttl disables caching.
func (c *Client) getJSON(ctx context.Context, path string, ttl time.Duration, out any) error {
	key := c.base + path
	if ttl > 0 {
		c.mu.Lock()
		if e, ok := c.cache[key]; ok && time.Since(e.at) < ttl {
			body := e.body
			c.mu.Unlock()
			return json.Unmarshal(body, out)
		}
		c.mu.Unlock()
	}
	status, body, err := c.f.Get(ctx, c.base+path)
	if err != nil {
		return fmt.Errorf("idlix: %w", err)
	}
	if status == 403 || (status == 200 && strings.Contains(string(body[:min(len(body), 512)]), "Just a moment")) {
		return ErrChallenged
	}
	if status != 200 {
		return fmt.Errorf("idlix: HTTP %d untuk %s", status, path)
	}
	if ttl > 0 {
		c.mu.Lock()
		c.cache[key] = cacheEntry{body: body, at: time.Now()}
		c.mu.Unlock()
	}
	return json.Unmarshal(body, out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---------- types ----------

// Rating is a vote average that the API sends as a string ("6.50")
// but occasionally as a number; both decode into a float.
type Rating float64

// UnmarshalJSON accepts "6.50", 6.5, or null.
func (r *Rating) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*r = 0
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		*r = 0
		return nil
	}
	*r = Rating(f)
	return nil
}

// Float returns the rating as a plain float64.
func (r Rating) Float() float64 { return float64(r) }

// Genre is a catalog genre (TMDB-backed).
type Genre struct {
	ID     string `json:"id"`
	TMDBID int    `json:"tmdbId"`
	Name   string `json:"name"`
	Slug   string `json:"slug"`
}

// Pagination describes a list page.
type Pagination struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// MovieItem is one movie in a list.
type MovieItem struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	PosterPath  string  `json:"posterPath"`
	Backdrop    string  `json:"backdropPath"`
	ReleaseDate string  `json:"releaseDate"`
	Rating      Rating  `json:"voteAverage"`
	Quality     string  `json:"quality"`
	Country     string  `json:"country"`
	Runtime     int     `json:"runtime"`
	Genres      []Genre `json:"genres"`
	HasVideo    bool    `json:"hasVideo"`
}

// Year extracts the release year ("" when unknown).
func (m MovieItem) Year() string { return yearOf(m.ReleaseDate) }

// Poster returns the TMDB poster URL (w342), "" when absent.
func (m MovieItem) Poster() string { return tmdbImg(m.PosterPath, "w342") }

// BackdropURL returns the TMDB backdrop URL (w1280), "" when absent.
func (m MovieItem) BackdropURL() string { return tmdbImg(m.Backdrop, "w1280") }

// SeriesItem is one series in a list.
type SeriesItem struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Slug            string  `json:"slug"`
	PosterPath      string  `json:"posterPath"`
	Backdrop        string  `json:"backdropPath"`
	FirstAirDate    string  `json:"firstAirDate"`
	Rating          Rating  `json:"voteAverage"`
	Country         string  `json:"country"`
	NumberOfSeasons int     `json:"numberOfSeasons"`
	Genres          []Genre `json:"genres"`
	HasVideo        bool    `json:"hasVideo"`
}

// Year extracts the first-air year ("" when unknown).
func (s SeriesItem) Year() string { return yearOf(s.FirstAirDate) }

// Poster returns the TMDB poster URL (w342), "" when absent.
func (s SeriesItem) Poster() string { return tmdbImg(s.PosterPath, "w342") }

// BackdropURL returns the TMDB backdrop URL (w1280), "" when absent.
func (s SeriesItem) BackdropURL() string { return tmdbImg(s.Backdrop, "w1280") }

// CastMember is one credited person.
type CastMember struct {
	Name        string `json:"name"`
	Character   string `json:"character"`
	ProfilePath string `json:"profilePath"`
}

// Profile returns the TMDB profile image URL (w185), "" when absent.
func (c CastMember) Profile() string { return tmdbImg(c.ProfilePath, "w185") }

// Movie is a movie detail.
type Movie struct {
	MovieItem
	TMDBID   int          `json:"tmdbId"`
	ImdbID   string       `json:"imdbId"`
	Overview string       `json:"overview"`
	Tagline  string       `json:"tagline"`
	Director string       `json:"director"`
	Status   string       `json:"status"`
	Trailer  string       `json:"trailerUrl"`
	Cast     []CastMember `json:"cast"`
}

// Episode is one series episode.
type Episode struct {
	ID            string `json:"id"`
	EpisodeNumber int    `json:"episodeNumber"`
	Name          string `json:"name"`
	Overview      string `json:"overview"`
	StillPath     string `json:"stillPath"`
	AirDate       string `json:"airDate"`
	Runtime       int    `json:"runtime"`
	HasVideo      bool   `json:"hasVideo"`
}

// Still returns the TMDB still URL (w500), "" when absent.
func (e Episode) Still() string { return tmdbImg(e.StillPath, "w500") }

// SeasonDetail is a season with its episodes.
type SeasonDetail struct {
	ID           string    `json:"id"`
	SeasonNumber int       `json:"seasonNumber"`
	Name         string    `json:"name"`
	Overview     string    `json:"overview"`
	PosterPath   string    `json:"posterPath"`
	EpisodeCount int       `json:"episodeCount"`
	Episodes     []Episode `json:"episodes"`
}

// Season is season metadata (no episodes).
type Season struct {
	ID           string `json:"id"`
	SeasonNumber int    `json:"seasonNumber"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episodeCount"`
	AirDate      string `json:"airDate"`
}

// Series is a series detail.
type Series struct {
	SeriesItem
	TMDBID           int           `json:"tmdbId"`
	ImdbID           string        `json:"imdbId"`
	Overview         string        `json:"overview"`
	Tagline          string        `json:"tagline"`
	Status           string        `json:"status"`
	Trailer          string        `json:"trailerUrl"`
	NumberOfEpisodes int           `json:"numberOfEpisodes"`
	Seasons          []Season      `json:"seasons"`
	DefaultSeason    *SeasonDetail `json:"defaultSeason"`
	Cast             []CastMember  `json:"cast"`
}

// SearchResult is one mixed search hit (movie or tv_series).
type SearchResult struct {
	ID          string   `json:"id"`
	ContentType string   `json:"contentType"` // "movie" | "tv_series"
	Title       string   `json:"title"`
	Overview    string   `json:"overview"`
	Genres      []string `json:"genres"`
	ReleaseDate string   `json:"releaseDate"`
	FirstAir    string   `json:"firstAirDate"`
	Rating      Rating   `json:"voteAverage"`
	PosterPath  string   `json:"posterPath"`
	Backdrop    string   `json:"backdropPath"`
	Quality     string   `json:"quality"`
	Slug        string   `json:"slug"`
}

// IsSeries reports whether the hit is a series.
func (r SearchResult) IsSeries() bool { return r.ContentType == "tv_series" }

// Year extracts the release/first-air year.
func (r SearchResult) Year() string {
	if r.FirstAir != "" {
		return yearOf(r.FirstAir)
	}
	return yearOf(r.ReleaseDate)
}

// Poster returns the TMDB poster URL (w342), "" when absent.
func (r SearchResult) Poster() string { return tmdbImg(r.PosterPath, "w342") }

func tmdbImg(path, size string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "http") {
		return path
	}
	return "https://image.tmdb.org/t/p/" + size + "/" + strings.TrimPrefix(path, "/")
}

func yearOf(date string) string {
	if len(date) >= 4 {
		return date[:4]
	}
	return ""
}

// ---------- endpoints ----------

type listResp[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

func listPath(kind string, page, limit int, sort, genre string) string {
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	if sort != "" {
		q.Set("sort", sort)
	}
	if genre != "" {
		q.Set("genre", genre)
	}
	return "/api/" + kind + "?" + q.Encode()
}

// Movies lists movies (kind filters: sort e.g. "popularity", genre slug).
func (c *Client) Movies(ctx context.Context, page, limit int, sort, genre string) ([]MovieItem, Pagination, error) {
	var out listResp[MovieItem]
	err := c.getJSON(ctx, listPath("movies", page, limit, sort, genre), 10*time.Minute, &out)
	return out.Data, out.Pagination, err
}

// SeriesList lists series.
func (c *Client) SeriesList(ctx context.Context, page, limit int, sort, genre string) ([]SeriesItem, Pagination, error) {
	var out listResp[SeriesItem]
	err := c.getJSON(ctx, listPath("series", page, limit, sort, genre), 10*time.Minute, &out)
	return out.Data, out.Pagination, err
}

// Movie fetches one movie detail by slug.
func (c *Client) Movie(ctx context.Context, slug string) (*Movie, error) {
	var out Movie
	err := c.getJSON(ctx, "/api/movies/"+url.PathEscape(slug), 20*time.Minute, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SeriesDetail fetches one series detail by slug (includes the default
// season's episodes).
func (c *Client) SeriesDetail(ctx context.Context, slug string) (*Series, error) {
	var out Series
	err := c.getJSON(ctx, "/api/series/"+url.PathEscape(slug), 20*time.Minute, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

type episodeResp struct {
	Season *SeasonDetail `json:"season"`
}

// SeasonEpisodes returns one season (with episodes) of a series. The
// API exposes episodes through the episode endpoint, whose response
// carries the whole season; episode 1 is used as the carrier.
func (c *Client) SeasonEpisodes(ctx context.Context, slug string, season int) (*SeasonDetail, error) {
	path := fmt.Sprintf("/api/series/%s/season/%d/episode/1", url.PathEscape(slug), season)
	var out episodeResp
	if err := c.getJSON(ctx, path, 20*time.Minute, &out); err != nil {
		return nil, err
	}
	return out.Season, nil
}

// Genres lists all catalog genres.
func (c *Client) Genres(ctx context.Context) ([]Genre, error) {
	var out struct {
		Data []Genre `json:"data"`
	}
	err := c.getJSON(ctx, "/api/genres", 6*time.Hour, &out)
	return out.Data, err
}

// Search searches movies and series together.
func (c *Client) Search(ctx context.Context, q string) ([]SearchResult, int, error) {
	var out struct {
		Results []SearchResult `json:"results"`
		Total   int            `json:"total"`
	}
	err := c.getJSON(ctx, "/api/search?q="+url.QueryEscape(q), 0, &out)
	return out.Results, out.Total, err
}
