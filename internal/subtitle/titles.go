package subtitle

import (
	"regexp"
	"strconv"
	"strings"
)

// Helpers for matching anime titles to subtitle release filenames:
// season/episode markers in release names are the strongest signal
// that a candidate subtitle belongs to the episode being watched.

var (
	seasonWordRe = regexp.MustCompile(`(?i)\b(?:season\s*(\d+)|(\d+)(?:nd|rd|th|st)\s*season|part\s*(\d+)|cour\s*(\d+)|s(\d+))\b`)
	romanRe      = regexp.MustCompile(`(?i)\b(ii|iii|iv|v|vi)\b`)
	seasonStrip  = regexp.MustCompile(`(?i)\b(?:season\s*\d+|\d+(?:nd|rd|th|st)\s*season|part\s*\d+|cour\s*\d+|s\d+)\b`)
	parenRe      = regexp.MustCompile(`\([^)]*\)`)
	bracketRe    = regexp.MustCompile(`\[[^\]]*\]`)
	spacingRe    = regexp.MustCompile(`\s+`)
	fileSeasonSE = regexp.MustCompile(`(?i)\bs0*(\d+)e\d+\b`)
	fileEpSE     = regexp.MustCompile(`(?i)\bs\d+e0*(\d+)\b`)
	fileEpWord   = regexp.MustCompile(`(?i)\bep(?:isode)?[\s._-]*0*(\d+)\b`)
	fileEpDelim  = regexp.MustCompile(`[\s._\[(-]0*(\d+)(?:v\d+)?[\s._\]\)-]`)
	nonAlnumRe   = regexp.MustCompile(`[^a-z0-9 ]+`)
)

var romanSeasons = map[string]int{"ii": 2, "iii": 3, "iv": 4, "v": 5, "vi": 6}

func firstGroup(m []string) int {
	for _, g := range m[1:] {
		if g != "" {
			if n, err := strconv.Atoi(g); err == nil {
				return n
			}
		}
	}
	return 0
}

// BaseTitle strips season/part markers and parentheticals from an
// anime title and returns the clean base title plus the season number
// it carried (1 when none was found).
func BaseTitle(title string) (string, int) {
	season := 1
	if m := seasonWordRe.FindStringSubmatch(title); m != nil {
		if n := firstGroup(m); n > 0 {
			season = n
		}
	} else if m := romanRe.FindStringSubmatch(strings.ToLower(title)); m != nil {
		season = romanSeasons[m[1]]
	}
	base := seasonStrip.ReplaceAllString(title, "")
	base = parenRe.ReplaceAllString(base, " ")
	base = spacingRe.ReplaceAllString(base, " ")
	return strings.Trim(base, " -:"), season
}

// TitleVariations returns the distinct search/match forms of a title:
// full, base (season-stripped), and the part before a subtitle colon.
func TitleVariations(title string) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Trim(spacingRe.ReplaceAllString(s, " "), " -:;,._")
		key := strings.ToLower(s)
		if len(s) > 1 && !seen[key] {
			seen[key] = true
			out = append(out, s)
		}
	}
	base, _ := BaseTitle(title)
	add(title)
	add(base)
	for _, sep := range []string{":", " - ", " – "} {
		if strings.Contains(base, sep) {
			add(strings.Split(base, sep)[0])
		}
	}
	if len(out) == 0 {
		return []string{"Anime"}
	}
	return out
}

// FileSeason extracts an explicit season number from a release
// filename; 0 means "no season marker present".
func FileSeason(name string) int {
	low := strings.ToLower(name)
	if m := fileSeasonSE.FindStringSubmatch(low); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if m := seasonWordRe.FindStringSubmatch(low); m != nil {
		if n := firstGroup(m); n > 0 {
			return n
		}
	}
	if m := romanRe.FindStringSubmatch(low); m != nil {
		return romanSeasons[m[1]]
	}
	return 0
}

// FileEpisode extracts an explicit episode number from a release
// filename; -1 means "no episode marker present".
func FileEpisode(name string) int {
	low := strings.ToLower(name)
	if m := fileEpSE.FindStringSubmatch(low); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if m := fileEpWord.FindStringSubmatch(low); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if m := fileEpDelim.FindStringSubmatch(low); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n
		}
	}
	return -1
}

var stopTokens = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "in": true, "on": true,
	"tv": true, "series": true, "episode": true, "ep": true, "season": true,
	"sub": true, "indo": true, "indonesia": true, "subtitle": true,
	"srt": true, "ass": true, "vtt": true, "1080p": true, "720p": true,
	"webrip": true, "webdl": true, "bdrip": true, "animation": true,
}

func tokens(s string) map[string]bool {
	s = nonAlnumRe.ReplaceAllString(strings.ToLower(s), " ")
	out := map[string]bool{}
	for _, t := range strings.Fields(s) {
		if len(t) > 1 && !stopTokens[t] {
			out[t] = true
		}
	}
	return out
}

// TitleSimilarity scores how well a candidate title covers the query
// title's tokens. Short queries demand near-full coverage so a shared
// franchise word alone never counts as a match.
func TitleSimilarity(query, candidate string) float64 {
	qa, qb := tokens(query), tokens(candidate)
	if len(qa) == 0 || len(qb) == 0 {
		return 0
	}
	inter := 0
	for t := range qa {
		if qb[t] {
			inter++
		}
	}
	if inter == 0 {
		return 0
	}
	coverage := float64(inter) / float64(len(qa))
	union := len(qa) + len(qb) - inter
	jaccard := float64(inter) / float64(union)
	if len(qa) <= 2 {
		if coverage >= 0.99 && (jaccard >= 0.28 || len(qb) <= len(qa)+2) {
			return coverage
		}
		return 0
	}
	if coverage >= 0.70 && jaccard >= 0.20 {
		return coverage
	}
	return 0
}

// Offshoot franchise pairs that must never cross-match.
var offshoots = [][2]string{
	{"naruto", "boruto"},
	{"inuyasha", "yashahime"},
	{"bleach", "burn the witch"},
}

// AccurateMatch reports whether a subtitle release name plausibly
// belongs to animeTitle at the given season/episode: franchise
// offshoots are rejected, title similarity must clear 0.60, and any
// season/episode marker present in the name must agree.
func AccurateMatch(animeTitle, candidate string, season, episode int) bool {
	if animeTitle == "" || candidate == "" {
		return false
	}
	lowA, lowC := strings.ToLower(animeTitle), strings.ToLower(candidate)
	for _, pair := range offshoots {
		parent, spin := pair[0], pair[1]
		if strings.Contains(lowA, parent) && !strings.Contains(lowA, spin) && strings.Contains(lowC, spin) {
			return false
		}
		if strings.Contains(lowA, spin) && !strings.Contains(lowA, parent) && strings.Contains(lowC, parent) && !strings.Contains(lowC, spin) {
			return false
		}
	}
	best := 0.0
	for _, v := range TitleVariations(animeTitle) {
		if s := TitleSimilarity(v, candidate); s > best {
			best = s
		}
	}
	if best < 0.60 {
		return false
	}
	if season < 1 {
		season = 1
	}
	if cs := FileSeason(candidate); cs > 0 && cs != season {
		return false
	}
	if episode > 0 {
		if ce := FileEpisode(candidate); ce > 0 && ce != episode {
			return false
		}
	}
	return true
}
