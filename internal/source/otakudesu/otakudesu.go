// Package otakudesu implements the Otakudesu streaming source:
// Indonesian-subtitled anime scraped from otakudesu, whose episode pages
// embed a "putarin" player whose configuration is AES-256-GCM encrypted.
package otakudesu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/source"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Client is the Otakudesu provider.
type Client struct {
	base string
	http *http.Client
}

// New creates a provider rooted at base (no trailing slash).
func New(base string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		http: &http.Client{Timeout: 25 * time.Second},
	}
}

// Name implements source.Provider.
func (c *Client) Name() string { return "otakudesu" }

func (c *Client) fetch(ctx context.Context, rawurl, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	if referer == "" {
		referer = c.base + "/"
	}
	req.Header.Set("Referer", referer)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("otakudesu: HTTP %d untuk %s", resp.StatusCode, rawurl)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

var (
	cardRe = regexp.MustCompile(`(?s)<a class="ot-k"\s+href="([^"]+)">.*?<img[^>]*src="([^"]+)".*?<h2 class="ot-k-j">([^<]+)</h2>`)
	epRe   = regexp.MustCompile(`<a class="ot-ep"\s+href="([^"]+)">\s*<span>([^<]+)</span>`)
	epNum  = regexp.MustCompile(`(?i)(?:episode|eps|ep)\s*#?\s*(\d+)`)
	slugRe = regexp.MustCompile(`/anime/([^/]+)/?`)
)

// Search implements source.Provider.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]source.Series, error) {
	clean := strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, query)
	u := fmt.Sprintf("%s/?s=%s&post_type=anime", c.base, url.QueryEscape(strings.TrimSpace(clean)))
	page, err := c.fetch(ctx, u, "")
	if err != nil {
		return nil, err
	}
	out := []source.Series{}
	for _, m := range cardRe.FindAllStringSubmatch(page, -1) {
		href, poster, title := m[1], m[2], html.UnescapeString(strings.TrimSpace(m[3]))
		slug := href
		if sm := slugRe.FindStringSubmatch(href); sm != nil {
			slug = sm[1]
		}
		out = append(out, source.Series{ID: slug, Title: title, Poster: poster, URL: href})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Episodes implements source.Provider.
func (c *Client) Episodes(ctx context.Context, seriesID string) ([]source.Episode, error) {
	pageURL := seriesID
	if !strings.HasPrefix(pageURL, "http") {
		pageURL = fmt.Sprintf("%s/anime/%s/", c.base, strings.Trim(seriesID, "/"))
	}
	page, err := c.fetch(ctx, pageURL, "")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []source.Episode{}
	for _, m := range epRe.FindAllStringSubmatch(page, -1) {
		href, title := m[1], strings.TrimSpace(m[2])
		if seen[href] {
			continue
		}
		seen[href] = true
		num := len(out) + 1
		if nm := epNum.FindStringSubmatch(title); nm != nil {
			if n, err := strconv.Atoi(nm[1]); err == nil {
				num = n
			}
		}
		out = append(out, source.Episode{Number: num, Title: title, URL: href})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

var (
	iframeRe  = regexp.MustCompile(`<iframe[^>]*src="([^"]*putarin\.[a-z]+/e/[^"]+)"`)
	iframeAny = regexp.MustCompile(`(?s)<div class="ot-player"[^>]*>.*?<iframe[^>]*src="([^"]+)"`)
	pxRe      = regexp.MustCompile(`window\.__PX\s*=\s*(\{[^<]+});`)
)

// Resolve implements source.Provider.
func (c *Client) Resolve(ctx context.Context, episodeURL string) (*source.Stream, error) {
	page, err := c.fetch(ctx, episodeURL, "")
	if err != nil {
		return nil, err
	}
	embed := ""
	if m := iframeRe.FindStringSubmatch(page); m != nil {
		embed = m[1]
	} else if m := iframeAny.FindStringSubmatch(page); m != nil {
		embed = m[1]
	}
	if embed == "" {
		return nil, fmt.Errorf("otakudesu: iframe pemutar tidak ditemukan")
	}
	if strings.HasPrefix(embed, "//") {
		embed = "https:" + embed
	}
	cfg, origin, err := c.decryptPlayerConfig(ctx, embed)
	if err != nil {
		return nil, err
	}
	rel, _ := cfg["file"].(string)
	if rel == "" {
		if srcs, ok := cfg["sources"].([]any); ok && len(srcs) > 0 {
			if first, ok := srcs[0].(map[string]any); ok {
				rel, _ = first["file"].(string)
			}
		}
	}
	if rel == "" {
		return nil, fmt.Errorf("otakudesu: file stream tidak ditemukan di konfigurasi pemutar")
	}
	base, err := url.Parse(origin + "/")
	if err != nil {
		return nil, err
	}
	relURL, err := url.Parse(rel)
	if err != nil {
		return nil, err
	}
	return &source.Stream{
		MasterURL: base.ResolveReference(relURL).String(),
		Referer:   origin + "/",
		Server:    "Otakudesu (Sub Indo)",
		SubLang:   "Indonesian (Hardsub)",
	}, nil
}

// decryptPlayerConfig fetches the putarin embed page, pulls the AES key for
// its nonce, and decrypts the player configuration JSON.
func (c *Client) decryptPlayerConfig(ctx context.Context, embedURL string) (map[string]any, string, error) {
	page, err := c.fetch(ctx, embedURL, c.base+"/")
	if err != nil {
		return nil, "", err
	}
	m := pxRe.FindStringSubmatch(page)
	if m == nil {
		return nil, "", fmt.Errorf("otakudesu: token pemutar (__PX) tidak ditemukan")
	}
	var px struct {
		Nonce string `json:"n"`
		Data  string `json:"d"`
	}
	if err := json.Unmarshal([]byte(m[1]), &px); err != nil {
		return nil, "", err
	}
	embedParsed, err := url.Parse(embedURL)
	if err != nil {
		return nil, "", err
	}
	origin := embedParsed.Scheme + "://" + embedParsed.Host
	keyHex, err := c.fetch(ctx, fmt.Sprintf("%s/api/pk?n=%s", origin, url.QueryEscape(px.Nonce)), embedURL)
	if err != nil {
		return nil, "", err
	}
	key, err := hex.DecodeString(strings.TrimSpace(keyHex))
	if err != nil || len(key) != 32 {
		return nil, "", fmt.Errorf("otakudesu: kunci dekripsi stream tidak valid")
	}
	raw, err := base64.StdEncoding.DecodeString(px.Data)
	if err != nil || len(raw) <= 12 {
		return nil, "", fmt.Errorf("otakudesu: data pemutar rusak")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", err
	}
	plain, err := gcm.Open(nil, raw[:12], raw[12:], nil)
	if err != nil {
		return nil, "", fmt.Errorf("otakudesu: dekripsi konfigurasi pemutar gagal: %w", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, "", err
	}
	return cfg, origin, nil
}

// Match finds the Otakudesu series whose title best matches an AniList title.
// It returns nil when nothing is a plausible match.
func (c *Client) Match(ctx context.Context, titles ...string) (*source.Series, []source.Episode, error) {
	var lastErr error
	for _, t := range titles {
		if strings.TrimSpace(t) == "" {
			continue
		}
		results, err := c.Search(ctx, t, 8)
		if err != nil {
			lastErr = err
			continue
		}
		for i := range results {
			if TitlesMatch(t, results[i].Title) {
				eps, err := c.Episodes(ctx, results[i].ID)
				if err != nil {
					return nil, nil, err
				}
				return &results[i], eps, nil
			}
		}
	}
	return nil, nil, lastErr
}

var (
	nonWord   = regexp.MustCompile(`[^a-z0-9 ]+`)
	spaceRun  = regexp.MustCompile(`\s+`)
	seasonTok = regexp.MustCompile(`^(season|s|\d+(nd|rd|th|st))$`)
)

func normalizeTitle(s string) []string {
	s = strings.ToLower(s)
	s = nonWord.ReplaceAllString(s, " ")
	s = spaceRun.ReplaceAllString(s, " ")
	parts := strings.Fields(s)
	out := parts[:0]
	for _, p := range parts {
		if seasonTok.MatchString(p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// TitlesMatch reports whether two titles plausibly name the same series:
// every distinctive token of the shorter title must appear in the longer.
func TitlesMatch(a, b string) bool {
	ta, tb := normalizeTitle(a), normalizeTitle(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	set := map[string]bool{}
	for _, t := range tb {
		set[t] = true
	}
	short, long := ta, set
	if len(tb) < len(ta) {
		short = tb
		long = map[string]bool{}
		for _, t := range ta {
			long[t] = true
		}
	}
	for _, t := range short {
		if !long[t] {
			return false
		}
	}
	return true
}
