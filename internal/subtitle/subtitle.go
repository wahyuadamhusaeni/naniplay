// Package subtitle parses, cleans, converts and shifts subtitle files.
//
// All supported inputs (SubRip .srt, WebVTT .vtt, SubStation Alpha
// .ass/.ssa) are normalized into a list of Cue values and rendered back
// out as WebVTT — the one format every browser player understands.
package subtitle

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Cue is one subtitle entry; times are seconds.
type Cue struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

var (
	bidiRe  = regexp.MustCompile("[\u200b-\u200f\u202a-\u202e\ufeff]")
	tagRe   = regexp.MustCompile(`<[^>]+>`)
	assTag  = regexp.MustCompile(`\{[^}]*\}`)
	timeSep = " --> "
)

// Parse detects the format of raw subtitle content and returns its cues.
func Parse(content string) ([]Cue, error) {
	content = bidiRe.ReplaceAllString(strings.ReplaceAll(content, "\r\n", "\n"), "")
	if strings.Contains(content, "[Events]") && strings.Contains(content, "Dialogue:") {
		return parseASS(content)
	}
	return parseTimedText(content)
}

// parseTimedText handles the block structure shared by SRT and WebVTT.
func parseTimedText(content string) ([]Cue, error) {
	var cues []Cue
	lines := strings.Split(content, "\n")
	i := 0
	for i < len(lines) {
		line := strings.TrimSpace(lines[i])
		if !strings.Contains(line, timeSep) {
			i++
			continue
		}
		parts := strings.SplitN(line, timeSep, 2)
		start, err1 := parseTimestamp(parts[0])
		endToken := strings.Fields(parts[1])
		if len(endToken) == 0 {
			i++
			continue
		}
		end, err2 := parseTimestamp(endToken[0])
		i++
		var textLines []string
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			textLines = append(textLines, strings.TrimSpace(lines[i]))
			i++
		}
		if err1 != nil || err2 != nil || len(textLines) == 0 {
			continue
		}
		text := cleanText(strings.Join(textLines, "\n"))
		if text == "" {
			continue
		}
		cues = append(cues, Cue{Start: start, End: end, Text: text})
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("tidak ada cue subtitle yang terbaca")
	}
	sort.SliceStable(cues, func(a, b int) bool { return cues[a].Start < cues[b].Start })
	return cues, nil
}

// parseASS parses the [Events] section of a SubStation Alpha file.
func parseASS(content string) ([]Cue, error) {
	var format []string
	startIdx, endIdx, textIdx := 1, 2, 9
	var cues []Cue
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Format:") {
			format = strings.Split(strings.TrimSpace(line[len("Format:"):]), ",")
			for j := range format {
				format[j] = strings.TrimSpace(format[j])
			}
			for j, f := range format {
				switch f {
				case "Start":
					startIdx = j
				case "End":
					endIdx = j
				case "Text":
					textIdx = j
				}
			}
			continue
		}
		if !strings.HasPrefix(line, "Dialogue:") {
			continue
		}
		body := strings.TrimSpace(line[len("Dialogue:"):])
		n := len(format)
		if n == 0 {
			n = 10
		}
		parts := splitNCommas(body, n)
		if len(parts) <= textIdx {
			continue
		}
		start, err1 := parseTimestamp(strings.TrimSpace(parts[startIdx]))
		end, err2 := parseTimestamp(strings.TrimSpace(parts[endIdx]))
		if err1 != nil || err2 != nil {
			continue
		}
		text := assTag.ReplaceAllString(parts[textIdx], "")
		text = strings.ReplaceAll(text, `\N`, "\n")
		text = strings.ReplaceAll(text, `\n`, "\n")
		text = strings.ReplaceAll(text, `\h`, " ")
		text = cleanText(text)
		if text == "" {
			continue
		}
		cues = append(cues, Cue{Start: start, End: end, Text: text})
	}
	if len(cues) == 0 {
		return nil, fmt.Errorf("tidak ada dialogue ASS yang terbaca")
	}
	sort.SliceStable(cues, func(a, b int) bool { return cues[a].Start < cues[b].Start })
	return cues, nil
}

// splitNCommas splits s into at most n parts on commas; the last part
// keeps any remaining commas (ASS Text may contain commas).
func splitNCommas(s string, n int) []string {
	parts := []string{}
	rest := s
	for len(parts) < n-1 {
		idx := strings.Index(rest, ",")
		if idx < 0 {
			break
		}
		parts = append(parts, rest[:idx])
		rest = rest[idx+1:]
	}
	return append(parts, rest)
}

// parseTimestamp parses "HH:MM:SS.mmm", "MM:SS.mmm", SRT's comma decimal
// separator, and ASS centiseconds ("H:MM:SS.cc").
func parseTimestamp(s string) (float64, error) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	parts := strings.Split(s, ":")
	var h, m float64
	var secStr string
	switch len(parts) {
	case 3:
		var err error
		if h, err = strconv.ParseFloat(parts[0], 64); err != nil {
			return 0, err
		}
		if m, err = strconv.ParseFloat(parts[1], 64); err != nil {
			return 0, err
		}
		secStr = parts[2]
	case 2:
		var err error
		if m, err = strconv.ParseFloat(parts[0], 64); err != nil {
			return 0, err
		}
		secStr = parts[1]
	default:
		return 0, fmt.Errorf("format waktu tidak dikenal: %q", s)
	}
	sec, err := strconv.ParseFloat(secStr, 64)
	if err != nil {
		return 0, err
	}
	return h*3600 + m*60 + sec, nil
}

func cleanText(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	lines := []string{}
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	return strings.Join(lines, "\n")
}

// ToVTT renders cues as a WebVTT document, shifting all timings by
// offset seconds (the manual desync correction). Cues shifted fully
// before zero are dropped; starts are clamped at zero.
func ToVTT(cues []Cue, offset float64) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	for _, c := range cues {
		start := c.Start + offset
		end := c.End + offset
		if end <= 0 {
			continue
		}
		if start < 0 {
			start = 0
		}
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n", formatTimestamp(start), formatTimestamp(end), esc.Replace(c.Text))
	}
	return b.String()
}

func formatTimestamp(sec float64) string {
	total := int64(sec*1000 + 0.5)
	ms := total % 1000
	s := total / 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", s/3600, (s/60)%60, s%60, ms)
}

// LooksLikeSubtitle reports whether text plausibly is a subtitle file
// (used to reject HTML error pages from download endpoints).
func LooksLikeSubtitle(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	low := strings.ToLower(t)
	if strings.Contains(low, "<html") || strings.Contains(low, "<!doctype") || strings.Contains(low, "<body") {
		return false
	}
	return strings.Contains(t, "-->") || strings.Contains(low, "dialogue:") || strings.HasPrefix(low, "webvtt")
}

var adPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)opensubtitles\.(org|com)`),
	regexp.MustCompile(`(?i)osdb\.link`),
	regexp.MustCompile(`(?i)advertise\s+your\s+product`),
	regexp.MustCompile(`(?i)iklankan\s+produk`),
	regexp.MustCompile(`(?i)become\s+(a\s+)?vip\s+member`),
	regexp.MustCompile(`(?i)support\s+us\s+and\s+become\s+vip`),
	regexp.MustCompile(`(?i)subtitles?\s+downloaded\s+from`),
	regexp.MustCompile(`(?i)americascardroom`),
}

// StripAds removes watermark/advertising cues (and single ad lines)
// that OpenSubtitles downloads commonly embed.
func StripAds(cues []Cue) []Cue {
	isAd := func(s string) bool {
		for _, p := range adPatterns {
			if p.MatchString(s) {
				return true
			}
		}
		return false
	}
	out := []Cue{}
	for _, c := range cues {
		if isAd(c.Text) {
			// Whole-cue ad: drop it entirely.
			lines := strings.Split(c.Text, "\n")
			allAd := true
			for _, ln := range lines {
				if strings.TrimSpace(ln) != "" && !isAd(ln) {
					allAd = false
					break
				}
			}
			if allAd {
				continue
			}
		}
		kept := []string{}
		for _, ln := range strings.Split(c.Text, "\n") {
			if !isAd(ln) {
				kept = append(kept, ln)
			}
		}
		text := strings.TrimSpace(strings.Join(kept, "\n"))
		if text == "" {
			continue
		}
		c.Text = text
		out = append(out, c)
	}
	return out
}

// DetectFormat returns "ass", "vtt", or "srt" for raw content.
func DetectFormat(content string) string {
	if strings.Contains(content, "[Events]") && strings.Contains(content, "Dialogue:") {
		return "ass"
	}
	if strings.HasPrefix(strings.TrimSpace(content), "WEBVTT") {
		return "vtt"
	}
	return "srt"
}
