package subtitle

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Machine translation of subtitle cues via Google's public GTX web
// endpoint — the same zero-key trick anime players use to turn an
// English subtitle into an Indonesian one on the fly. Quality is
// literal; it exists so an episode is never unwatcheable just because
// no Indonesian subtitle was published for it.

var gtxBase = "https://translate.googleapis.com/translate_a/single"

// Contraction expansion: raw machine translation handles full forms
// far better than spoken contractions, so normalize first.
var contractionRules = []struct {
	re   *regexp.Regexp
	full string
}{}
var contractionMap = map[string]string{
	"i'm": "I am", "don't": "do not", "can't": "cannot", "won't": "will not",
	"didn't": "did not", "doesn't": "does not", "isn't": "is not",
	"aren't": "are not", "wasn't": "was not", "weren't": "were not",
	"haven't": "have not", "hasn't": "has not", "couldn't": "could not",
	"shouldn't": "should not", "wouldn't": "would not", "it's": "it is",
	"that's": "that is", "there's": "there is", "what's": "what is",
	"let's": "let us", "you're": "you are", "we're": "we are",
	"they're": "they are", "i've": "I have", "we've": "we have",
	"i'll": "I will", "you'll": "you will", "he's": "he is", "she's": "she is",
}

func init() {
	for short, full := range contractionMap {
		re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(short) + `\b`)
		contractionRules = append(contractionRules, struct {
			re   *regexp.Regexp
			full string
		}{re, full})
	}
}

func expandContractions(s string) string {
	for _, r := range contractionRules {
		s = r.re.ReplaceAllString(s, r.full)
	}
	return s
}

// gtxPost sends one form-encoded POST to the GTX endpoint. POST is
// used rather than GET: the endpoint rate-limits (429) long GET query
// strings from datacenter addresses, while form posts go through.
// Errors deliberately omit the request body — it is subtitle text.
func gtxPost(ctx context.Context, params url.Values) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, gtxBase, strings.NewReader(params.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", browserUA)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("HTTP 429 (dibatasi Google)")
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("HTTP %d dari endpoint terjemahan", resp.StatusCode)
		}
		return body, nil
	}
	return nil, lastErr
}

// gtxTranslateText translates one joined block of text. The endpoint
// answers with segment pairs; concatenating the translated segments
// reproduces the block (newlines are preserved as segment breaks).
func gtxTranslateText(ctx context.Context, text, sourceLang, targetLang string) (string, error) {
	params := url.Values{
		"client": {"gtx"},
		"sl":     {sourceLang},
		"tl":     {targetLang},
		"dt":     {"t"},
		"q":      {text},
	}
	raw, err := gtxPost(ctx, params)
	if err != nil {
		return "", err
	}
	var resp []any
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("respons GTX tidak dikenal: %w", err)
	}
	if len(resp) == 0 {
		return "", fmt.Errorf("respons GTX kosong")
	}
	segments, ok := resp[0].([]any)
	if !ok {
		return "", fmt.Errorf("respons GTX tidak dikenal")
	}
	var b strings.Builder
	for _, seg := range segments {
		pair, ok := seg.([]any)
		if !ok || len(pair) == 0 {
			continue
		}
		if s, ok := pair[0].(string); ok {
			b.WriteString(s)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("hasil terjemahan kosong")
	}
	return out, nil
}

// translateChunk translates a batch of cue texts, preserving count
// and order. Batches ride on newline-joined requests; if the joined
// result cannot be split back reliably, cues fall back to individual
// requests. Transport-level failures abort the whole translation at
// once — retrying cue by cue against a blocked endpoint would just
// grind for minutes before failing anyway.
func translateChunk(ctx context.Context, texts []string, sourceLang, targetLang string) ([]string, error) {
	joined := strings.Join(texts, "\n")
	translated, err := gtxTranslateText(ctx, joined, sourceLang, targetLang)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(translated, "\n")
	if len(lines) == len(texts) {
		out := make([]string, len(lines))
		for i, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				ln = texts[i]
			}
			out[i] = ln
		}
		return out, nil
	}
	out := make([]string, len(texts))
	for i, t := range texts {
		one, err := gtxTranslateText(ctx, t, sourceLang, targetLang)
		if err != nil {
			return nil, err
		}
		out[i] = one
	}
	return out, nil
}

// TranslateCues machine-translates every cue's text to targetLang
// (source "auto" detects). Timings are untouched. Multi-line cues are
// flattened before translation, so results are single-line per cue.
func TranslateCues(ctx context.Context, cues []Cue, sourceLang, targetLang string) ([]Cue, error) {
	if sourceLang == "" {
		sourceLang = "auto"
	}
	if targetLang == "" {
		targetLang = "id"
	}
	texts := make([]string, len(cues))
	for i, c := range cues {
		t := strings.Join(strings.Fields(strings.ReplaceAll(c.Text, "\n", " ")), " ")
		texts[i] = expandContractions(t)
	}
	const chunkBudget = 1200
	translated := make([]string, len(texts))
	for start := 0; start < len(texts); {
		end := start
		size := 0
		for end < len(texts) && size+len(texts[end])+1 <= chunkBudget {
			size += len(texts[end]) + 1
			end++
		}
		if end == start {
			end = start + 1
		}
		part, err := translateChunk(ctx, texts[start:end], sourceLang, targetLang)
		if err != nil {
			return nil, fmt.Errorf("endpoint terjemahan tidak merespons dengan benar (mungkin dibatasi jaringan): %w", err)
		}
		copy(translated[start:end], part)
		start = end
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(120 * time.Millisecond):
		}
	}
	out := make([]Cue, len(cues))
	for i, c := range cues {
		c.Text = translated[i]
		out[i] = c
	}
	return out, nil
}
