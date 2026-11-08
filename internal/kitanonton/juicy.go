package kitanonton

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// The JuicyCodes embed hides its JW Player configuration in a
// _juicycodes("..."+"...") blob. The encoding (reverse-engineered
// and verified byte-for-byte against the real player on 2026-11-08)
// is pure string math:
//
//  1. the last 3 chars form the salt: concat(charCode(c) - 100);
//  2. the rest is base64 (URL-safe alphabet) decoded to a string over
//     a 10-symbol alphabet (a double ROT13 in the JS is the identity
//     and is skipped);
//  3. each char maps to its index in the symbol table, forming a
//     digit string;
//  4. each 4-digit chunk yields one output char:
//     char((chunk mod 1000) - salt).
//
// The result is JS of the fixed shape
// "var config = {...JSON...}; jwplayer.key = '...'; ...".

var juicySymbols = []byte{'`', '%', '-', '+', '*', '$', '!', '_', '^', '='}

var reJuicyCall = regexp.MustCompile(`_juicycodes\(((?:"[^"]*"\s*\+?\s*)+)\)`)
var reQuoted = regexp.MustCompile(`"([^"]*)"`)
var reJuicyConfig = regexp.MustCompile(`(?s)var config = (\{.*?\});`)

func decodeJuicy(blob string) (string, error) {
	if len(blob) < 8 {
		return "", errors.New("kitanonton: blob juicy terlalu pendek")
	}
	saltText := ""
	for _, c := range blob[len(blob)-3:] {
		saltText += strconv.Itoa(int(c) - 100)
	}
	salt, err := strconv.Atoi(saltText)
	if err != nil {
		return "", errors.New("kitanonton: garam juicy tidak valid")
	}
	body := blob[:len(blob)-3]
	if r := len(body) % 4; r != 0 {
		body += strings.Repeat("=", 4-r)
	}
	raw, err := base64.URLEncoding.DecodeString(body)
	if err != nil {
		return "", err
	}
	digits := make([]byte, 0, len(raw))
	for _, b := range raw {
		idx := -1
		for i, s := range juicySymbols {
			if b == s {
				idx = i
				break
			}
		}
		if idx < 0 {
			return "", errors.New("kitanonton: simbol juicy tidak dikenal")
		}
		digits = append(digits, byte('0'+idx))
	}
	var out strings.Builder
	for i := 0; i+3 < len(digits)+1 && i+4 <= len(digits); i += 4 {
		n, err := strconv.Atoi(string(digits[i : i+4]))
		if err != nil {
			return "", err
		}
		out.WriteRune(rune(n%1000 - salt))
	}
	return out.String(), nil
}

// juicyConfig mirrors the fields of the decoded JW config we use.
type juicyConfig struct {
	Image   string `json:"image"`
	Sources struct {
		File   string            `json:"file"`
		Labels map[string]string `json:"labels"`
	} `json:"sources"`
}

// parseEmbed extracts and decodes the player config of an embed page.
func parseEmbed(pageHTML string) (*Stream, error) {
	m := reJuicyCall.FindStringSubmatch(pageHTML)
	if m == nil {
		return nil, errors.New("kitanonton: blob juicy tidak ditemukan di embed")
	}
	var blob strings.Builder
	for _, q := range reQuoted.FindAllStringSubmatch(m[1], -1) {
		blob.WriteString(q[1])
	}
	js, err := decodeJuicy(blob.String())
	if err != nil {
		return nil, err
	}
	cm := reJuicyConfig.FindStringSubmatch(js)
	if cm == nil {
		return nil, errors.New("kitanonton: konfigurasi juicy tidak ditemukan")
	}
	cfg, err := parseJuicyConfig(cm[1])
	if err != nil {
		return nil, err
	}
	if cfg.Sources.File == "" {
		return nil, errors.New("kitanonton: konfigurasi juicy tanpa sumber")
	}
	return &Stream{MasterURL: cfg.Sources.File, Poster: cfg.Image, Labels: cfg.Sources.Labels}, nil
}

// parseJuicyConfig accepts the sources field either as an object
// (observed in production) or as a one-element array.
func parseJuicyConfig(text string) (*juicyConfig, error) {
	var cfg juicyConfig
	if err := json.Unmarshal([]byte(text), &cfg); err == nil && cfg.Sources.File != "" {
		return &cfg, nil
	}
	var alt struct {
		Image   string `json:"image"`
		Sources []struct {
			File   string            `json:"file"`
			Labels map[string]string `json:"labels"`
		} `json:"sources"`
	}
	if err := json.Unmarshal([]byte(text), &alt); err != nil {
		return nil, err
	}
	if len(alt.Sources) == 0 {
		return nil, errors.New("kitanonton: sources kosong")
	}
	cfg.Image = alt.Image
	cfg.Sources.File = alt.Sources[0].File
	cfg.Sources.Labels = alt.Sources[0].Labels
	return &cfg, nil
}
