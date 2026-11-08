// Package anichan implements the AniChan streaming source
// (anichan.to). Unlike the other sources it is keyed directly by
// AniList ID, so "matching" is a lookup, not a title search. Its
// watch API needs a session token plus a rotating key triple that
// ships inside the site's own JS bundle: the triple is scraped from
// the bundle (with a bundled static fallback), the session token
// keys an HMAC-SHA256 derivation, and the server list payload is
// AES-256-GCM encrypted with the derived key.
package anichan

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
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

	"github.com/wahyuadamhusaeni/naniplay/internal/source"
)

const defaultBase = "https://anichan.to"
const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// Static key triple from AniChan's JS bundle — a fallback only; the
// live triple is scraped from the bundle because it rotates.
const (
	staticK = "f7312859"
	staticO = "dF+YyX8ksuoyznJHUVeHE1yRdVJVwZM4/FEnWRi2Wbg="
	staticB = "Mh83TJtN3J8ZDnKT35ASzU31AL5zurEGSAsX9scP4Lk="
)

type keyTriple struct{ k, o, b string }

// Client is an AniChan provider.
type Client struct {
	base string
	http *http.Client

	mu       sync.Mutex
	keys     keyTriple
	keysAt   time.Time
	tokenN   string
	cookie   string
	tokenExp time.Time
}

// New builds an AniChan client; empty base uses the default domain.
func New(base string) *Client {
	if base == "" {
		base = defaultBase
	}
	return &Client{
		base: strings.TrimSuffix(base, "/"),
		http: &http.Client{Timeout: 25 * time.Second},
		keys: keyTriple{staticK, staticO, staticB},
	}
}

// Name implements source.Provider.
func (c *Client) Name() string { return "anichan" }

// Search is unsupported: AniChan is keyed by AniList ID (see Lookup).
func (c *Client) Search(ctx context.Context, query string, limit int) ([]source.Series, error) {
	return nil, fmt.Errorf("AniChan tidak menyediakan pencarian judul (berbasis ID AniList)")
}

func (c *Client) do(ctx context.Context, method, rawURL string, headers map[string]string, body string) (int, http.Header, []byte, error) {
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", browserUA)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return 0, nil, nil, err
	}
	return resp.StatusCode, resp.Header, data, nil
}

func (c *Client) get(ctx context.Context, rawURL string, headers map[string]string) (int, []byte, error) {
	code, _, body, err := c.do(ctx, http.MethodGet, rawURL, headers, "")
	return code, body, err
}

// --- Keys --------------------------------------------------------------

var (
	chunkRe  = regexp.MustCompile(`/_next/static/chunks/[^"']+\.js`)
	tripleRe = regexp.MustCompile(`[a-zA-Z0-9_$]+="([0-9a-f]{8})",[a-zA-Z0-9_$]+="([A-Za-z0-9+/=]{44})",[a-zA-Z0-9_$]+="([A-Za-z0-9+/=]{44})"`)
)

// refreshKeys scrapes the current key triple from the site's JS
// bundle (the chunk that talks to /api/watch/servers). Falls back to
// the last known triple when scraping fails.
func (c *Client) refreshKeys(ctx context.Context) keyTriple {
	c.mu.Lock()
	if time.Since(c.keysAt) < 6*time.Hour {
		keys := c.keys
		c.mu.Unlock()
		return keys
	}
	c.mu.Unlock()

	_, home, err := c.get(ctx, c.base+"/", nil)
	if err == nil {
		chunks := chunkRe.FindAllString(string(home), -1)
		seen := map[string]bool{}
		for _, ch := range chunks {
			if seen[ch] {
				continue
			}
			seen[ch] = true
			_, js, err := c.get(ctx, c.base+ch, nil)
			if err != nil || !strings.Contains(string(js), "watch/servers") {
				continue
			}
			if m := tripleRe.FindSubmatch(js); m != nil {
				triple := keyTriple{string(m[1]), string(m[2]), string(m[3])}
				c.mu.Lock()
				c.keys, c.keysAt = triple, time.Now()
				c.mu.Unlock()
				return triple
			}
		}
	}
	c.mu.Lock()
	c.keysAt = time.Now() // do not hammer the site when scraping fails
	keys := c.keys
	c.mu.Unlock()
	return keys
}

// --- Session -----------------------------------------------------------

// session returns a valid (token, cookie header) pair, refreshing
// the session when expired (or when forced after a 401).
func (c *Client) session(ctx context.Context, force bool) (string, string, error) {
	c.mu.Lock()
	if !force && c.tokenN != "" && time.Until(c.tokenExp) > time.Minute {
		token, cookie := c.tokenN, c.cookie
		c.mu.Unlock()
		return token, cookie, nil
	}
	c.mu.Unlock()

	code, header, body, err := c.do(ctx, http.MethodPost, c.base+"/api/watch/session", map[string]string{
		"Content-Type": "application/json",
		"Referer":      c.base + "/",
		"Origin":       c.base,
	}, `{"token":""}`)
	if err != nil {
		return "", "", err
	}
	if code != http.StatusOK {
		return "", "", fmt.Errorf("sesi AniChan gagal (HTTP %d)", code)
	}
	var data struct {
		N   string  `json:"n"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(body, &data); err != nil || data.N == "" {
		return "", "", fmt.Errorf("respons sesi AniChan tidak terbaca")
	}
	cookies := []string{}
	for _, sc := range header.Values("Set-Cookie") {
		if i := strings.Index(sc, ";"); i > 0 {
			cookies = append(cookies, strings.TrimSpace(sc[:i]))
		}
	}
	exp := time.Now().Add(2 * time.Hour)
	if data.Exp > 0 {
		exp = time.Unix(int64(data.Exp/1000), 0) // exp is in milliseconds
		if exp.Before(time.Now()) {
			exp = time.Unix(int64(data.Exp), 0)
		}
	}
	c.mu.Lock()
	c.tokenN, c.cookie, c.tokenExp = data.N, strings.Join(cookies, "; "), exp
	c.mu.Unlock()
	return data.N, c.cookie, nil
}

// --- Payload decryption --------------------------------------------------

// decryptPayload decrypts an encrypted servers payload:
// hmacKey = O XOR b; aesKey = HMAC-SHA256(hmacKey, sessionToken);
// plaintext = AES-256-GCM(aesKey, iv).decrypt(ciphertext).
func decryptPayload(raw []byte, tokenN string, keys keyTriple) (json.RawMessage, error) {
	var probe struct {
		V int    `json:"v"`
		I string `json:"i"`
		D string `json:"d"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if probe.V != 1 || probe.I == "" || probe.D == "" {
		return raw, nil // not encrypted
	}
	rawO, err := base64.StdEncoding.DecodeString(keys.o)
	if err != nil {
		return nil, err
	}
	rawB, err := base64.StdEncoding.DecodeString(keys.b)
	if err != nil {
		return nil, err
	}
	hmacKey := make([]byte, len(rawO))
	for i := range rawO {
		if i < len(rawB) {
			hmacKey[i] = rawO[i] ^ rawB[i]
		}
	}
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(tokenN))
	aesKey := mac.Sum(nil)

	iv, err := base64.StdEncoding.DecodeString(probe.I)
	if err != nil {
		return nil, err
	}
	ct, err := base64.StdEncoding.DecodeString(probe.D)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, iv, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("dekripsi payload AniChan gagal: %w", err)
	}
	return plain, nil
}

// --- Lookup & episodes ---------------------------------------------------

type episodesResponse struct {
	Episodes int `json:"episodes"`
}

func (c *Client) episodeCount(ctx context.Context, anilistID int) (int, error) {
	for _, u := range []string{
		fmt.Sprintf("%s/api/watch/episodes?anilistId=%d", c.base, anilistID),
		fmt.Sprintf("%s/api/catalog/anime/%d", c.base, anilistID),
	} {
		code, body, err := c.get(ctx, u, map[string]string{"Referer": c.base + "/"})
		if err != nil || code != http.StatusOK {
			continue
		}
		var data episodesResponse
		if err := json.Unmarshal(body, &data); err == nil && data.Episodes > 0 {
			return data.Episodes, nil
		}
	}
	return 0, fmt.Errorf("AniChan tidak memiliki anime ini")
}

// Lookup resolves a series purely by AniList ID — AniChan's catalog
// is keyed by it, so no title guessing is involved.
func (c *Client) Lookup(ctx context.Context, anilistID int, title string, wantEpisodes int) (*source.Series, []source.Episode, error) {
	total, err := c.episodeCount(ctx, anilistID)
	if err != nil {
		return nil, nil, err
	}
	eps := c.buildEpisodes(anilistID, total)
	series := &source.Series{
		ID:     strconv.Itoa(anilistID),
		Title:  title,
		URL:    fmt.Sprintf("%s/anime/%d", c.base, anilistID),
		Source: "AniChan",
	}
	return series, eps, nil
}

func (c *Client) buildEpisodes(anilistID, total int) []source.Episode {
	eps := make([]source.Episode, 0, total)
	for n := 1; n <= total; n++ {
		eps = append(eps, source.Episode{
			Number: n,
			URL:    fmt.Sprintf("%s/anime/%d?ep=%d", c.base, anilistID, n),
		})
	}
	return eps
}

// Episodes implements source.Provider (seriesID = AniList ID).
func (c *Client) Episodes(ctx context.Context, seriesID string) ([]source.Episode, error) {
	aid, err := strconv.Atoi(seriesID)
	if err != nil {
		return nil, fmt.Errorf("id seri AniChan tidak valid: %s", seriesID)
	}
	total, err := c.episodeCount(ctx, aid)
	if err != nil {
		return nil, err
	}
	return c.buildEpisodes(aid, total), nil
}

// --- Resolve -------------------------------------------------------------

type acServer struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Stream    string `json:"stream"`
	Subtitles []struct {
		Lang    string `json:"lang"`
		URL     string `json:"url"`
		Default bool   `json:"default"`
	} `json:"subtitles"`
}

func serverPriority(name string) int {
	low := strings.ToLower(name)
	switch {
	case strings.Contains(low, "anikoto"):
		return 0
	case strings.Contains(low, "megaplay_hd1"):
		return 1
	case strings.Contains(low, "megaplay_hd2"):
		return 2
	case strings.Contains(low, "aniwatch"), strings.Contains(low, "zoko"):
		return 3
	default:
		return 4
	}
}

// unwrapStream extracts the direct upstream URL from AniChan's
// wrapper URLs (?url=...&ref=...); native relative streams are made
// absolute against the AniChan origin with its own referer.
func (c *Client) unwrapStream(stream string) (direct, referer string) {
	if strings.HasPrefix(stream, "/") {
		return c.base + stream, c.base + "/"
	}
	u, err := url.Parse(stream)
	if err != nil {
		return stream, c.base + "/"
	}
	if raw := u.Query().Get("url"); strings.HasPrefix(raw, "http") {
		ref := u.Query().Get("ref")
		if ref == "" {
			ref = "https://megaplay.buzz/"
		}
		return raw, ref
	}
	return stream, c.base + "/"
}

func (c *Client) fetchServers(ctx context.Context, aid, ep int) ([]acServer, error) {
	tokenN, cookie, err := c.session(ctx, false)
	if err != nil {
		return nil, err
	}
	keys := c.refreshKeys(ctx)
	fetch := func(token, cookie string) ([]acServer, int, error) {
		for _, tier := range []string{"fast", "rest"} {
			u := fmt.Sprintf("%s/api/watch/servers?anilistId=%d&ep=%d&category=sub&tier=%s", c.base, aid, ep, tier)
			code, body, err := c.get(ctx, u, map[string]string{
				"X-Wk":    keys.k,
				"Referer": fmt.Sprintf("%s/anime/%d", c.base, aid),
				"Origin":  c.base,
				"Cookie":  cookie,
			})
			if err != nil {
				return nil, 0, err
			}
			if code == http.StatusUnauthorized {
				return nil, code, nil
			}
			if code != http.StatusOK {
				continue
			}
			plain, err := decryptPayload(body, token, keys)
			if err != nil {
				return nil, 0, err
			}
			var data struct {
				Servers []acServer `json:"servers"`
			}
			if err := json.Unmarshal(plain, &data); err != nil {
				return nil, 0, fmt.Errorf("daftar server AniChan tidak terbaca")
			}
			if len(data.Servers) > 0 {
				return data.Servers, code, nil
			}
		}
		return nil, http.StatusOK, nil
	}
	servers, code, err := fetch(tokenN, cookie)
	if err == nil && code == http.StatusUnauthorized {
		tokenN, cookie, err = c.session(ctx, true)
		if err == nil {
			servers, _, err = fetch(tokenN, cookie)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("AniChan tidak punya server untuk episode ini")
	}
	return servers, nil
}

// Resolve implements source.Provider: episodeURL is {base}/anime/{aid}?ep={n}.
func (c *Client) Resolve(ctx context.Context, episodeURL string) (*source.Stream, error) {
	u, err := url.Parse(episodeURL)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	aid, errAID := strconv.Atoi(parts[len(parts)-1])
	ep, errEP := strconv.Atoi(u.Query().Get("ep"))
	if errAID != nil || errEP != nil || ep < 1 {
		return nil, fmt.Errorf("URL episode AniChan tidak valid")
	}
	servers, err := c.fetchServers(ctx, aid, ep)
	if err != nil {
		return nil, err
	}
	hls := []acServer{}
	for _, s := range servers {
		if strings.EqualFold(s.Type, "hls") && s.Stream != "" {
			hls = append(hls, s)
		}
	}
	if len(hls) == 0 {
		return nil, fmt.Errorf("tidak ada server HLS langsung di AniChan untuk episode ini")
	}
	sort.SliceStable(hls, func(i, j int) bool { return serverPriority(hls[i].Name) < serverPriority(hls[j].Name) })

	var lastErr error
	for _, srv := range hls {
		direct, referer := c.unwrapStream(srv.Stream)
		code, body, err := c.get(ctx, direct, map[string]string{"Referer": referer})
		if err != nil {
			lastErr = err
			continue
		}
		if code != http.StatusOK || !strings.Contains(string(body), "#EXTM3U") {
			lastErr = fmt.Errorf("server %s: stream tidak valid (HTTP %d)", srv.Name, code)
			continue
		}
		tracks := []source.SubTrack{}
		for _, st := range srv.Subtitles {
			trackURL := st.URL
			if strings.HasPrefix(trackURL, "/") {
				trackURL = c.base + trackURL
			}
			if !strings.HasPrefix(trackURL, "http") {
				continue
			}
			label := st.Lang
			if label == "" {
				label = "Subtitle"
			}
			tracks = append(tracks, source.SubTrack{Label: label, Lang: langCode(label), URL: trackURL})
		}
		return &source.Stream{
			MasterURL: direct,
			Referer:   referer,
			Server:    "AniChan · " + srv.Name,
			SubLang:   "Softsub bawaan (bisa diterjemahkan)",
			Subtitles: tracks,
			SubSource: "AniChan",
		}, nil
	}
	return nil, fmt.Errorf("semua server AniChan gagal: %w", lastErr)
}

// langCode maps a subtitle language name to an ISO code.
func langCode(label string) string {
	low := strings.ToLower(label)
	switch {
	case strings.Contains(low, "english"):
		return "en"
	case strings.Contains(low, "indonesia"):
		return "id"
	case strings.Contains(low, "arab"):
		return "ar"
	case strings.Contains(low, "spanish"):
		return "es"
	case strings.Contains(low, "french"):
		return "fr"
	case strings.Contains(low, "german"):
		return "de"
	case len(low) >= 2:
		return low[:2]
	default:
		return ""
	}
}
