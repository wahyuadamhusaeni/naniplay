// Package kitanonton is a clean-room client for KitaNonton-style
// WordPress (indoxxi/Layarkaca21 theme) catalogs. It lists the
// Indonesian-country archive, reads title detail pages, extracts the
// player server list from /play pages, and resolves the "dv" JuicyCodes
// embeds into direct HLS master URLs.
//
// The site changes domains often, so the base URL is a runtime setting,
// exactly like the other sources in NaniPlay.
package kitanonton

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	http2 "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// Fetcher fetches a page body. Abstracted for tests.
type Fetcher interface {
	Get(ctx context.Context, url string) (int, []byte, error)
}

// plainFetcher reads the WordPress site itself (no gate).
type plainFetcher struct{ client *http.Client }

func (f plainFetcher) Get(ctx context.Context, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "id-ID,id;q=0.9,en;q=0.8")
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, body, err
}

// tlsFetcher reads the JuicyCodes embed pages. They live on bare IP
// hosts whose certificates do not cover the IP, and the CDN edge
// expects a genuine browser TLS fingerprint, so this mirrors the
// IDLIX fetcher: bogdanfinn tls-client with a Chrome profile and
// InsecureSkipVerify for the IP-literal hosts.
type tlsFetcher struct{ client tlsclient.HttpClient }

func newTLSFetcher() (tlsFetcher, error) {
	c, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(),
		tlsclient.WithTimeoutSeconds(30),
		tlsclient.WithClientProfile(profiles.Chrome_131),
		tlsclient.WithCookieJar(tlsclient.NewCookieJar()),
		tlsclient.WithInsecureSkipVerify(),
	)
	if err != nil {
		return tlsFetcher{}, err
	}
	return tlsFetcher{client: c}, nil
}

// FetchStream GETs a CDN resource (playlist or segment) with the
// full browser header set the JuicyCodes CDN expects, including an
// Origin matching the embed page. The caller owns the returned body.
func (f tlsFetcher) FetchStream(ctx context.Context, rawURL, referer, rangeHeader string) (int, string, io.ReadCloser, error) {
	req, err := http2.NewRequestWithContext(ctx, http2.MethodGet, rawURL, nil)
	if err != nil {
		return 0, "", nil, err
	}
	origin := ""
	if u, err := url.Parse(referer); err == nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	h := http2.Header{
		"user-agent":         {userAgent},
		"accept":             {"*/*"},
		"referer":            {referer},
		http2.HeaderOrderKey: {"user-agent", "accept", "referer"},
	}
	if origin != "" {
		h = http2.Header{
			"user-agent":         {userAgent},
			"accept":             {"*/*"},
			"accept-language":    {"id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
			"accept-encoding":    {"gzip, deflate, br, zstd"},
			"origin":             {origin},
			"referer":            {referer},
			"sec-ch-ua":          {`"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`},
			"sec-ch-ua-mobile":   {"?0"},
			"sec-ch-ua-platform": {`"Windows"`},
			"sec-fetch-dest":     {"empty"},
			"sec-fetch-mode":     {"cors"},
			"sec-fetch-site":     {"cross-site"},
			"priority":           {"u=1, i"},
			http2.HeaderOrderKey: {"user-agent", "accept", "accept-language", "accept-encoding", "origin", "referer", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site", "priority"},
		}
	}
	if rangeHeader != "" {
		h.Set("range", rangeHeader)
	}
	req.Header = h
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, "", nil, err
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), resp.Body, nil
}

// GetWithReferer is Get with a caller-chosen referer (the document
// gate on the embed hosts keys on the source site's domain).
func (f tlsFetcher) GetWithReferer(ctx context.Context, url, referer string) (int, []byte, error) {
	req, err := http2.NewRequestWithContext(ctx, http2.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header = http2.Header{
		"user-agent":         {userAgent},
		"accept":             {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"accept-language":    {"id-ID,id;q=0.9,en;q=0.8"},
		"referer":            {referer},
		http2.HeaderOrderKey: {"user-agent", "accept", "accept-language", "referer"},
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, body, err
}

func (f tlsFetcher) Get(ctx context.Context, url string) (int, []byte, error) {
	req, err := http2.NewRequestWithContext(ctx, http2.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header = http2.Header{
		"user-agent":         {userAgent},
		"accept":             {"text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
		"accept-language":    {"id-ID,id;q=0.9,en;q=0.8"},
		"referer":            {"https://kitanonton2.garden/"},
		http2.HeaderOrderKey: {"user-agent", "accept", "accept-language", "referer"},
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return resp.StatusCode, body, err
}

// Item is one catalog card (movie or series).
type Item struct {
	Slug        string
	Title       string
	Year        int
	Rating      float64
	Quality     string
	DurationMin int
	Poster      string
	Series      bool
	Episodes    int // parsed from the card badge when present
}

// Detail is a title page: metadata plus synopsis.
type Detail struct {
	Item
	Overview    string
	Genres      []string
	Director    string
	Cast        string
	ReleaseDate string
	TrailerID   string // YouTube video id, when present
}

// Server is one playable server entry from a /play page.
type Server struct {
	Type    string // "dv" (JuicyCodes) or "ex" (AbyssPlayer)
	URL     string // decoded embed URL
	Episode int    // 1 for movies
	Label   string // quality label ("720p") or "Ep N"
}

// Stream is a resolved JuicyCodes embed: the direct HLS master.
type Stream struct {
	MasterURL string
	Poster    string
	Labels    map[string]string
}

type cacheEntry struct {
	body []byte
	at   time.Time
}

// Client talks to one KitaNonton deployment.
type Client struct {
	base     string
	site     Fetcher
	embed    Fetcher
	embedTLS tlsFetcher

	mu       sync.Mutex
	cache    map[string]cacheEntry
	egressIP string
	egressAt time.Time
}

// New builds a Client for a base URL like https://kitanonton2.garden.
func New(base string) (*Client, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return nil, errors.New("kitanonton: base harus diawali http:// atau https://")
	}
	emb, err := newTLSFetcher()
	if err != nil {
		return nil, err
	}
	return &Client{
		base:     base,
		site:     plainFetcher{client: &http.Client{Timeout: 30 * time.Second}},
		embed:    emb,
		embedTLS: emb,
		cache:    map[string]cacheEntry{},
	}, nil
}

// Base returns the current base URL.
func (c *Client) Base() string { return c.base }

// SetBase swaps the base URL (domain rotation) and clears the cache.
func (c *Client) SetBase(base string) {
	c.base = strings.TrimRight(strings.TrimSpace(base), "/")
	c.mu.Lock()
	c.cache = map[string]cacheEntry{}
	c.mu.Unlock()
}

// getSite fetches a catalog/detail/play page with a short cache.
func (c *Client) getSite(ctx context.Context, url string) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.cache[url]; ok && time.Since(e.at) < 10*time.Minute {
		c.mu.Unlock()
		return e.body, nil
	}
	c.mu.Unlock()
	status, body, err := c.site.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("kitanonton: %s membalas %d", url, status)
	}
	c.mu.Lock()
	c.cache[url] = cacheEntry{body: body, at: time.Now()}
	c.mu.Unlock()
	return body, nil
}

// Catalog reads one page of the Indonesian-country archive.
// Page 1 is /country/indonesia/, later pages append page/N/.
func (c *Client) Catalog(ctx context.Context, page int) ([]Item, int, error) {
	if page < 1 {
		page = 1
	}
	url := c.base + "/country/indonesia/"
	if page > 1 {
		url = fmt.Sprintf("%scountry/indonesia/page/%d/", c.base+"/", page)
	}
	body, err := c.getSite(ctx, url)
	if err != nil {
		return nil, 0, err
	}
	html := string(body)
	return parseCatalog(html, c.base), parseTotalPages(html), nil
}

// Detail reads a movie or series detail page.
func (c *Client) Detail(ctx context.Context, slug string, series bool) (*Detail, error) {
	url := c.detailURL(slug, series)
	body, err := c.getSite(ctx, url)
	if err != nil {
		return nil, err
	}
	d := parseDetail(string(body))
	if d == nil {
		return nil, errors.New("kitanonton: halaman detail tidak terbaca")
	}
	d.Slug = slug
	d.Series = series
	return d, nil
}

func (c *Client) detailURL(slug string, series bool) string {
	if series {
		return c.base + "/series/" + slug + "/"
	}
	return c.base + "/" + slug + "/"
}

// PlayServers reads the /play page and returns its server list. For
// series, servers for every episode are on the single play page,
// each labelled "Ep N".
func (c *Client) PlayServers(ctx context.Context, slug string, series bool) ([]Server, error) {
	url := c.detailURL(slug, series) + "play/"
	body, err := c.getSite(ctx, url)
	if err != nil {
		return nil, err
	}
	servers := parsePlayServers(string(body))
	if len(servers) == 0 {
		return nil, errors.New("kitanonton: tidak ada server di halaman putar")
	}
	return servers, nil
}

// ServersForEpisode filters a play page's servers to one episode.
func ServersForEpisode(servers []Server, episode int) []Server {
	out := make([]Server, 0, len(servers))
	for _, s := range servers {
		if s.Episode == episode {
			out = append(out, s)
		}
	}
	return out
}

// EpisodeCount reports the highest episode number in a server list.
func EpisodeCount(servers []Server) int {
	max := 0
	for _, s := range servers {
		if s.Episode > max {
			max = s.Episode
		}
	}
	return max
}

// Resolve fetches one JuicyCodes embed page and decodes the player
// configuration into the direct HLS master URL. The returned URL may
// carry a token baked for THIS server's IP/UA; callers serving a
// browser should re-sign it with SignForViewer.
func (c *Client) Resolve(ctx context.Context, embedURL string) (*Stream, error) {
	status, body, err := c.embed.Get(ctx, embedURL)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("kitanonton: embed membalas %d", status)
	}
	return parseEmbed(string(body))
}

// EgressIP reports this server's public IP as the CDN sees it,
// discovered lazily through the embed fetcher and cached for an
// hour. The identity token must name THIS IP when the server itself
// fetches the stream (proxy mode).
func (c *Client) EgressIP(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.egressIP != "" && time.Since(c.egressAt) < time.Hour {
		ip := c.egressIP
		c.mu.Unlock()
		return ip, nil
	}
	c.mu.Unlock()
	_, body, err := c.embed.Get(ctx, "https://api.ipify.org")
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("kitanonton: egress tidak dikenal (%q)", ip)
	}
	c.mu.Lock()
	c.egressIP, c.egressAt = ip, time.Now()
	c.mu.Unlock()
	return ip, nil
}

// SignForServer signs a master/playlist URL with THIS server's
// identity token (its egress IP + its fetcher's User-Agent), for use
// when the server fetches the stream itself and proxies it to the
// viewer. If the egress IP cannot be determined the URL is returned
// unchanged.
func (c *Client) SignForServer(ctx context.Context, rawURL string) string {
	ip, err := c.EgressIP(ctx)
	if err != nil || ip == "" {
		return rawURL
	}
	return SignForViewer(rawURL, ip, userAgent)
}

// FetchStream proxies one CDN resource through the embed fetcher.
func (c *Client) FetchStream(ctx context.Context, rawURL, referer, rangeHeader string) (int, string, io.ReadCloser, error) {
	return c.embedTLS.FetchStream(ctx, rawURL, referer, rangeHeader)
}

// FetchEmbedDoc downloads one embed document with this client's
// site as referer, for the web layer's proxied-embed mode.
func (c *Client) FetchEmbedDoc(ctx context.Context, embedURL string) (string, error) {
	status, body, err := c.embedTLS.GetWithReferer(ctx, embedURL, c.base+"/")
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("kitanonton: embed membalas %d", status)
	}
	return string(body), nil
}

// MintToken builds the JuicyCodes identity token: the CDN's
// "signature" is just base64(IP + "~~" + User-Agent) of the viewer —
// no secret is involved (verified live 2026-11-08).
func MintToken(ip, ua string) string {
	return base64.StdEncoding.EncodeToString([]byte(ip + "~~" + ua))
}

// SignForViewer returns the master URL carrying the viewer's own
// identity token, replacing any token baked for another IP/UA, so the
// viewer's browser can fetch the CDN directly.
func SignForViewer(master, ip, ua string) string {
	tok := MintToken(ip, ua)
	if i := strings.Index(master, "token="); i >= 0 {
		rest := master[i+len("token="):]
		if j := strings.Index(rest, "&"); j >= 0 {
			return master[:i] + "token=" + tok + rest[j:]
		}
		return master[:i] + "token=" + tok
	}
	if strings.Contains(master, "?") {
		return master + "&token=" + tok
	}
	return master + "?token=" + tok
}
