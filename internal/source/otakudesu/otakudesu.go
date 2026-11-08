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
	"sync"
	"time"

	"github.com/wahyuadamhusaeni/naniplay/internal/source"
)

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Client is the Otakudesu provider. The base URL can be swapped at
// runtime because the source changes domains from time to time.
type Client struct {
	mu   sync.RWMutex
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

// Base returns the current source base URL.
func (c *Client) Base() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.base
}

// SetBase swaps the source base URL (used by the settings page).
func (c *Client) SetBase(base string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.base = strings.TrimRight(strings.TrimSpace(base), "/")
}

func (c *Client) fetch(ctx context.Context, rawurl, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawurl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	if referer == "" {
		referer = c.Base() + "/"
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
	u := fmt.Sprintf("%s/?s=%s&post_type=anime", c.Base(), url.QueryEscape(strings.TrimSpace(clean)))
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
		out = append(out, source.Series{ID: slug, Title: title, Poster: poster, URL: href, Source: "Otakudesu"})
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
		pageURL = fmt.Sprintf("%s/anime/%s/", c.Base(), strings.Trim(seriesID, "/"))
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
	page, err := c.fetch(ctx, embedURL, c.Base()+"/")
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

// Match finds the Otakudesu series that best matches one of the given
// AniList titles. Candidates are scored by token overlap and season
// agreement; when the expected episode count is known and the top two
// candidates score closely, the one whose episode count is closer wins.
// It returns (nil, nil, nil) when nothing is a plausible match — never a
// blind first-result fallback.
func (c *Client) Match(ctx context.Context, titles []string, wantEpisodes int) (*source.Series, []source.Episode, error) {
	type candidate struct {
		series source.Series
		score  float64
	}
	seen := map[string]bool{}
	var cands []candidate
	var lastErr error
	for _, t := range titles {
		if strings.TrimSpace(t) == "" {
			continue
		}
		results, err := c.Search(ctx, t, 10)
		if err != nil {
			lastErr = err
			continue
		}
		for _, r := range results {
			if seen[r.ID] {
				continue
			}
			seen[r.ID] = true
			if s := ScoreTitle(t, r.Title); s > 0 {
				cands = append(cands, candidate{series: r, score: s})
			}
		}
	}
	if len(cands) == 0 {
		return nil, nil, lastErr
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	best := cands[0]
	if best.score < 0.55 {
		return nil, nil, lastErr
	}
	// Close call? Let the episode count arbitrate between the top two.
	if wantEpisodes > 0 && len(cands) > 1 && best.score-cands[1].score < 0.12 {
		epsBest, errB := c.Episodes(ctx, best.series.ID)
		epsSecond, errS := c.Episodes(ctx, cands[1].series.ID)
		if errB == nil && errS == nil {
			dB := abs(len(epsBest) - wantEpisodes)
			dS := abs(len(epsSecond) - wantEpisodes)
			if dS < dB {
				return &cands[1].series, epsSecond, nil
			}
			return &best.series, epsBest, nil
		}
	}
	eps, err := c.Episodes(ctx, best.series.ID)
	if err != nil {
		return nil, nil, err
	}
	return &best.series, eps, nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

var (
	nonWord   = regexp.MustCompile(`[^a-z0-9 ]+`)
	spaceRun  = regexp.MustCompile(`\s+`)
	seasonTok = regexp.MustCompile(`^(season|s|\d+(nd|rd|th|st))$`)
	seasonNum = regexp.MustCompile(`(?i)(?:season|s)\s*(\d+)|(\d+)\s*(?:nd|rd|th|st)\s*season`)
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

// extractSeason returns the season number a title refers to (0 = none).
func extractSeason(s string) int {
	m := seasonNum.FindStringSubmatch(strings.ToLower(s))
	if m == nil {
		return 0
	}
	for _, g := range m[1:] {
		if g != "" {
			if n, err := strconv.Atoi(g); err == nil {
				return n
			}
		}
	}
	return 0
}

// ScoreTitle rates how plausibly candidate names the same series as query:
// token containment blended with Jaccard similarity, heavily penalized
// when both titles name different seasons.
func ScoreTitle(query, candidate string) float64 {
	tq, tc := normalizeTitle(query), normalizeTitle(candidate)
	if len(tq) == 0 || len(tc) == 0 {
		return 0
	}
	setQ := map[string]bool{}
	for _, t := range tq {
		setQ[t] = true
	}
	setC := map[string]bool{}
	for _, t := range tc {
		setC[t] = true
	}
	inter := 0
	for t := range setQ {
		if setC[t] {
			inter++
		}
	}
	if inter == 0 {
		return 0
	}
	containment := float64(inter) / float64(min(len(setQ), len(setC)))
	jaccard := float64(inter) / float64(len(setQ)+len(setC)-inter)
	score := 0.7*containment + 0.3*jaccard
	sq, sc := extractSeason(query), extractSeason(candidate)
	if sq > 0 && sc > 0 && sq != sc {
		score *= 0.3
	}
	return score
}

// TitlesMatch reports whether two titles plausibly name the same series.
func TitlesMatch(a, b string) bool { return ScoreTitle(a, b) >= 0.8 }
