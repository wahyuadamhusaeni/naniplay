// Package hianime implements the HiAnime streaming source
// (hianime.at). Its player embeds are served by Megaplay: the embed
// page carries a data-id, an AJAX call returns an AES-256-CBC
// encrypted payload with the HLS master URL, and the same response
// bundles multi-language soft-subtitle tracks (WebVTT) which the web
// layer imports so they can be translated like any other track.
package hianime

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
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
	"github.com/wahyuadamhusaeni/naniplay/internal/subtitle"
)

const defaultBase = "https://hianime.at"
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Megaplay source-payload key material (embedded in their player JS).
var (
	megaKey = append([]byte("i?LMTAx0Q6,:}50U"), make([]byte, 16)...)
	megaIV  = []byte("W0;27ToaUpl_P%'c")
)

// Client is a HiAnime provider.
type Client struct {
	base string
	http *http.Client

	mu  sync.Mutex
	eps map[string][]source.Episode
}

// New builds a HiAnime client; empty base uses the default domain.
func New(base string) *Client {
	if base == "" {
		base = defaultBase
	}
	return &Client{
		base: strings.TrimSuffix(base, "/"),
		http: &http.Client{Timeout: 25 * time.Second},
		eps:  map[string][]source.Episode{},
	}
}

// Name implements source.Provider.
func (c *Client) Name() string { return "hianime" }

func (c *Client) fetch(ctx context.Context, rawURL, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HiAnime HTTP %d", resp.StatusCode)
	}
	text := string(body)
	if strings.Contains(text, "<title>Just a moment") {
		return "", fmt.Errorf("HiAnime terhalang Cloudflare")
	}
	return text, nil
}

// --- Search ------------------------------------------------------------

type card struct {
	slug string
	en   string
	jp   string
}

var anchorRe = regexp.MustCompile(`<a\s[^>]*>`)

func attr(tag, name string) string {
	m := regexp.MustCompile(name + `="([^"]*)"`).FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// searchCards parses the search-results page: each result anchor has
// class "dynamic-name", an English title attribute, and the Japanese
// (romaji) name in data-jname — ideal for AniList title matching.
func (c *Client) searchCards(ctx context.Context, query string) ([]card, error) {
	page, err := c.fetch(ctx, c.base+"/search?keyword="+url.QueryEscape(query), "")
	if err != nil {
		return nil, err
	}
	cards := []card{}
	seen := map[string]bool{}
	for _, tag := range anchorRe.FindAllString(page, -1) {
		if !strings.Contains(tag, "dynamic-name") {
			continue
		}
		href := attr(tag, "href")
		slug := href
		if i := strings.LastIndex(slug, "/"); i >= 0 {
			slug = slug[i+1:]
		}
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		cards = append(cards, card{slug: slug, en: attr(tag, "title"), jp: attr(tag, "data-jname")})
	}
	return cards, nil
}

// Search implements source.Provider.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]source.Series, error) {
	cards, err := c.searchCards(ctx, query)
	if err != nil {
		return nil, err
	}
	out := []source.Series{}
	for _, cd := range cards {
		title := cd.jp
		if title == "" {
			title = cd.en
		}
		out = append(out, source.Series{ID: cd.slug, Title: title, URL: c.base + "/" + cd.slug, Source: "HiAnime"})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Match finds the best HiAnime series for AniList titles, using the
// same discipline as the Otakudesu matcher: strict title verification
// (franchise/season guards), never a blind first-result guess, and
// episode-count arbitration between close candidates.
func (c *Client) Match(ctx context.Context, titles []string, wantEpisodes int) (*source.Series, []source.Episode, error) {
	type cand struct {
		card  card
		score float64
	}
	best := map[string]*cand{}
	for _, q := range titles {
		if strings.TrimSpace(q) == "" {
			continue
		}
		cards, err := c.searchCards(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		_, season := subtitle.BaseTitle(q)
		for _, cd := range cards {
			score := 0.0
			for _, name := range []string{cd.jp, cd.en} {
				if name == "" || !subtitle.AccurateMatch(q, name, season, 0) {
					continue
				}
				if s := subtitle.TitleSimilarity(q, name); s > score {
					score = s
				}
			}
			if score <= 0 {
				continue
			}
			if old, ok := best[cd.slug]; !ok || score > old.score {
				best[cd.slug] = &cand{card: cd, score: score}
			}
		}
		if len(best) > 0 {
			break
		}
	}
	if len(best) == 0 {
		return nil, nil, nil
	}
	ranked := []*cand{}
	for _, cd := range best {
		ranked = append(ranked, cd)
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	winner := ranked[0]
	// Episode-count arbitration between close candidates.
	if wantEpisodes > 0 && len(ranked) > 1 && winner.score-ranked[1].score <= 0.12 {
		epsW, errW := c.Episodes(ctx, winner.card.slug)
		epsR, errR := c.Episodes(ctx, ranked[1].card.slug)
		if errW == nil && errR == nil {
			diff := func(n int) int {
				d := n - wantEpisodes
				if d < 0 {
					return -d
				}
				return d
			}
			if diff(len(epsR)) < diff(len(epsW)) {
				winner = ranked[1]
			}
		}
	}
	eps, err := c.Episodes(ctx, winner.card.slug)
	if err != nil {
		return nil, nil, err
	}
	if len(eps) == 0 {
		return nil, nil, nil
	}
	title := winner.card.jp
	if title == "" {
		title = winner.card.en
	}
	series := &source.Series{ID: winner.card.slug, Title: title, URL: c.base + "/" + winner.card.slug, Source: "HiAnime"}
	return series, eps, nil
}

// --- Episodes ----------------------------------------------------------

var epPairRe = regexp.MustCompile(`data-number="([0-9]+)"[^>]*?data-id="([0-9]+)"`)

// Episodes implements source.Provider.
func (c *Client) Episodes(ctx context.Context, seriesID string) ([]source.Episode, error) {
	c.mu.Lock()
	if eps, ok := c.eps[seriesID]; ok {
		c.mu.Unlock()
		return eps, nil
	}
	c.mu.Unlock()

	numeric := seriesID
	if i := strings.LastIndex(seriesID, "-"); i >= 0 {
		numeric = seriesID[i+1:]
	}
	if _, err := strconv.Atoi(numeric); err != nil {
		return nil, fmt.Errorf("slug HiAnime tidak valid: %s", seriesID)
	}
	body, err := c.fetch(ctx, c.base+"/api/theme/episode/list/"+numeric, "")
	if err != nil {
		return nil, err
	}
	if i := strings.Index(body, "{"); i > 0 {
		body = body[i:]
	}
	var data struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil, fmt.Errorf("daftar episode HiAnime tidak terbaca")
	}
	byNum := map[int]string{}
	for _, m := range epPairRe.FindAllStringSubmatch(data.HTML, -1) {
		n, err := strconv.Atoi(m[1])
		if err == nil {
			byNum[n] = m[2]
		}
	}
	nums := []int{}
	for n := range byNum {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	eps := []source.Episode{}
	for _, n := range nums {
		eps = append(eps, source.Episode{
			Number: n,
			URL:    fmt.Sprintf("%s/watch/%s?ep=%s", c.base, seriesID, byNum[n]),
		})
	}
	c.mu.Lock()
	c.eps[seriesID] = eps
	c.mu.Unlock()
	return eps, nil
}

// --- Resolve -----------------------------------------------------------

type serverEntry struct {
	name  string
	embed string
}

func parseServers(htmlText string) []serverEntry {
	parts := strings.Split(strings.ReplaceAll(htmlText, `\"`, `"`), "server-item")
	servers := []serverEntry{}
	for _, p := range parts {
		if !strings.Contains(p, `data-type="sub"`) {
			continue
		}
		mh := regexp.MustCompile(`data-hash="([^"]*)"`).FindStringSubmatch(p)
		if mh == nil {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(mh[1]))
		if err != nil {
			if raw, err = base64.URLEncoding.DecodeString(strings.TrimSpace(mh[1])); err != nil {
				continue
			}
		}
		embed := string(raw)
		if !strings.HasPrefix(embed, "http") {
			continue
		}
		name := "unknown"
		if mn := regexp.MustCompile(`data-server-name="([^"]*)"`).FindStringSubmatch(p); mn != nil {
			name = mn[1]
		}
		servers = append(servers, serverEntry{name: name, embed: embed})
	}
	sort.SliceStable(servers, func(i, j int) bool {
		return serverScore(servers[i]) < serverScore(servers[j])
	})
	return servers
}

func serverScore(s serverEntry) int {
	if strings.Contains(s.embed, "megaplay") {
		return 0
	}
	return 9
}

// Resolve implements source.Provider: episodeURL carries the episode
// id in its ?ep= parameter, as produced by Episodes.
func (c *Client) Resolve(ctx context.Context, episodeURL string) (*source.Stream, error) {
	u, err := url.Parse(episodeURL)
	if err != nil {
		return nil, err
	}
	epID := u.Query().Get("ep")
	if epID == "" {
		return nil, fmt.Errorf("URL episode HiAnime tidak membawa id episode")
	}
	serversHTML, err := c.fetch(ctx, c.base+"/api/theme/episode/servers?episodeId="+epID, "")
	if err != nil {
		return nil, err
	}
	servers := parseServers(serversHTML)
	if len(servers) == 0 {
		return nil, fmt.Errorf("tidak ada server sub di HiAnime untuk episode ini")
	}
	var lastErr error
	for _, srv := range servers {
		if !strings.Contains(srv.embed, "megaplay") {
			continue
		}
		stream, err := c.resolveMegaplay(ctx, srv)
		if err == nil {
			return stream, nil
		}
		lastErr = err
	}
	// Legacy embeds (ZokoAnime / Vidtube) carry their whole player
	// config in an XOR-obfuscated blob — try them after Megaplay.
	for _, srv := range servers {
		if strings.Contains(srv.embed, "megaplay") {
			continue
		}
		stream, err := c.resolveLegacy(ctx, srv)
		if err == nil {
			return stream, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("semua server HiAnime gagal: %w", lastErr)
	}
	return nil, fmt.Errorf("tidak ada server yang bisa dipakai untuk episode ini")
}

var legacyBlobRe = regexp.MustCompile(`window\.__P\s*=\s*"([^"]*)"`)
var legacyXORKey = []byte("otaku-embed-v1")

type legacyConfig struct {
	Src     string `json:"src"`
	Sources []struct {
		Src string `json:"src"`
	} `json:"sources"`
	Subtitles []struct {
		Src   string `json:"src"`
		File  string `json:"file"`
		URL   string `json:"url"`
		Label string `json:"label"`
		Lang  string `json:"lang"`
	} `json:"subtitles"`
}

// resolveLegacy resolves ZokoAnime-style embeds: the page's
// window.__P blob is base64 + a repeating XOR key away from the
// player config JSON (master playlist + subtitle tracks).
func (c *Client) resolveLegacy(ctx context.Context, srv serverEntry) (*source.Stream, error) {
	page, err := c.fetch(ctx, srv.embed, c.base+"/")
	if err != nil {
		return nil, err
	}
	if strings.Contains(page, "<title>Player</title>") && strings.Contains(page, "Not ") && strings.Contains(page, "found.") {
		return nil, fmt.Errorf("server melaporkan anime tidak ditemukan (404)")
	}
	m := legacyBlobRe.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("blob konfigurasi (window.__P) tidak ditemukan")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(m[1]))
	if err != nil {
		return nil, fmt.Errorf("blob konfigurasi bukan base64")
	}
	plain := make([]byte, len(raw))
	for i, b := range raw {
		plain[i] = b ^ legacyXORKey[i%len(legacyXORKey)]
	}
	var cfg legacyConfig
	if err := json.Unmarshal(plain, &cfg); err != nil {
		return nil, fmt.Errorf("konfigurasi legacy tidak terbaca")
	}
	master := cfg.Src
	if master == "" && len(cfg.Sources) > 0 {
		master = cfg.Sources[0].Src
	}
	if !strings.Contains(master, ".m3u8") {
		return nil, fmt.Errorf("konfigurasi legacy tidak membawa master m3u8")
	}
	origin := srv.embed
	if u, err := url.Parse(srv.embed); err == nil {
		origin = u.Scheme + "://" + u.Host
	}
	tracks := []source.SubTrack{}
	for _, t := range cfg.Subtitles {
		trackURL := t.Src
		if trackURL == "" {
			trackURL = t.File
		}
		if trackURL == "" {
			trackURL = t.URL
		}
		if strings.HasPrefix(trackURL, "/") {
			trackURL = origin + trackURL
		}
		if !strings.HasPrefix(trackURL, "http") {
			continue
		}
		label := t.Label
		if label == "" {
			label = t.Lang
		}
		if label == "" {
			label = "Subtitle"
		}
		tracks = append(tracks, source.SubTrack{Label: label, Lang: langFromLabel(label), URL: trackURL})
	}
	return &source.Stream{
		MasterURL: master,
		Referer:   origin + "/",
		Server:    "HiAnime · " + srv.name,
		SubLang:   "Softsub bawaan (bisa diterjemahkan)",
		Subtitles: tracks,
		SubSource: "HiAnime",
	}, nil
}

var dataIDRe = regexp.MustCompile(`data-id="(\d+)"`)

type megaplaySources struct {
	Enc     string          `json:"enc"`
	Sources json.RawMessage `json:"sources"`
	Tracks  []struct {
		File    string `json:"file"`
		Src     string `json:"src"`
		URL     string `json:"url"`
		Label   string `json:"label"`
		Kind    string `json:"kind"`
		Default bool   `json:"default"`
	} `json:"tracks"`
}

func (c *Client) resolveMegaplay(ctx context.Context, srv serverEntry) (*source.Stream, error) {
	page, err := c.fetch(ctx, srv.embed, c.base+"/")
	if err != nil {
		return nil, err
	}
	m := dataIDRe.FindStringSubmatch(page)
	if m == nil {
		return nil, fmt.Errorf("data-id Megaplay tidak ditemukan")
	}
	origin := srv.embed
	if u, err := url.Parse(srv.embed); err == nil {
		origin = u.Scheme + "://" + u.Host
	}
	apiURL := origin + "/stream/getSourcesNew?id=" + m[1]
	txt, err := c.fetch(ctx, apiURL, srv.embed)
	if err != nil {
		// Older players expose the non-"New" endpoint.
		txt, err = c.fetch(ctx, origin+"/stream/getSources?id="+m[1], srv.embed)
		if err != nil {
			return nil, err
		}
	}
	var cfg megaplaySources
	if err := json.Unmarshal([]byte(txt), &cfg); err != nil {
		return nil, fmt.Errorf("respons Megaplay tidak terbaca")
	}
	master := ""
	if len(cfg.Sources) > 0 && string(cfg.Sources) != "null" {
		var list []struct {
			File string `json:"file"`
		}
		if err := json.Unmarshal(cfg.Sources, &list); err == nil && len(list) > 0 {
			master = list[0].File
		} else {
			var single struct {
				File string `json:"file"`
			}
			if err := json.Unmarshal(cfg.Sources, &single); err == nil {
				master = single.File
			}
		}
	}
	if master == "" && cfg.Enc != "" {
		plain, err := megaDecrypt(cfg.Enc)
		if err != nil {
			return nil, err
		}
		var dec struct {
			File string `json:"file"`
		}
		if err := json.Unmarshal([]byte(plain), &dec); err != nil {
			return nil, fmt.Errorf("payload Megaplay tidak terbaca")
		}
		master = dec.File
	}
	if master == "" {
		return nil, fmt.Errorf("Megaplay tidak memberikan master playlist")
	}
	if strings.HasPrefix(master, "/") {
		master = origin + master
	}
	tracks := []source.SubTrack{}
	for _, t := range cfg.Tracks {
		kind := strings.ToLower(t.Kind)
		if kind != "" && kind != "subtitles" && kind != "subtitle" && kind != "captions" && kind != "caption" {
			continue
		}
		trackURL := t.File
		if trackURL == "" {
			trackURL = t.Src
		}
		if trackURL == "" {
			trackURL = t.URL
		}
		if !strings.HasPrefix(trackURL, "http") {
			continue
		}
		label := t.Label
		if label == "" {
			label = "Subtitle"
		}
		tracks = append(tracks, source.SubTrack{Label: label, Lang: langFromLabel(label), URL: trackURL})
	}
	return &source.Stream{
		MasterURL: master,
		Referer:   origin + "/",
		Server:    "HiAnime · " + srv.name,
		SubLang:   "Softsub bawaan (bisa diterjemahkan)",
		Subtitles: tracks,
		SubSource: "HiAnime",
	}, nil
}

// megaDecrypt decrypts Megaplay's base64url AES-256-CBC payload.
func megaDecrypt(enc string) (string, error) {
	raw, err := base64.URLEncoding.DecodeString(strings.TrimSpace(enc))
	if err != nil {
		// tolerate missing padding
		raw, err = base64.RawURLEncoding.DecodeString(strings.TrimSpace(enc))
		if err != nil {
			return "", fmt.Errorf("payload Megaplay bukan base64url")
		}
	}
	block, err := aes.NewCipher(megaKey)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return "", fmt.Errorf("payload Megaplay korup")
	}
	out := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, megaIV).CryptBlocks(out, raw)
	if pad := int(out[len(out)-1]); pad >= 1 && pad <= aes.BlockSize {
		ok := true
		for _, b := range out[len(out)-pad:] {
			if int(b) != pad {
				ok = false
				break
			}
		}
		if ok {
			out = out[:len(out)-pad]
		}
	}
	return string(out), nil
}

var langLabels = []struct {
	key  string
	code string
}{
	{"indonesia", "id"}, {"bahasa", "id"},
	{"english", "en"}, {"arabic", "ar"}, {"spanish", "es"},
	{"french", "fr"}, {"german", "de"}, {"italian", "it"},
	{"portuguese", "pt"}, {"russian", "ru"}, {"japanese", "ja"},
	{"korean", "ko"}, {"thai", "th"}, {"vietnamese", "vi"},
	{"malay", "ms"}, {"turkish", "tr"}, {"hindi", "hi"},
	{"chinese", "zh"}, {"polish", "pl"}, {"dutch", "nl"},
	{"swedish", "sv"}, {"ukrainian", "uk"},
}

// langFromLabel infers an ISO code from a subtitle track label.
func langFromLabel(label string) string {
	low := strings.ToLower(label)
	for _, l := range langLabels {
		if strings.Contains(low, l.key) {
			return l.code
		}
	}
	return ""
}
