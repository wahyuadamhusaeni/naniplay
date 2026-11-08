package kitanonton

import (
	"encoding/base64"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// The indoxxi theme's markup mixes single and double quotes and is
// regular enough for targeted regexes (the same approach the
// Otakudesu and HiAnime scrapers in this project use).

var (
	reCardHref    = regexp.MustCompile(`href=["']([^"']+)["']`)
	reCardH2      = regexp.MustCompile(`<h2>([^<]+)</h2>`)
	reCardImg     = regexp.MustCompile(`<img[^>]*src=["']([^"']+)["']`)
	reCardQuality = regexp.MustCompile(`mli-quality[^>]*>([^<]+)<`)
	reCardRating  = regexp.MustCompile(`mli-rating[^>]*>(?:<[^>]+>)*\s*([\d.]+)`)
	reCardDurasi  = regexp.MustCompile(`mli-durasi[^>]*>\s*(\d+)`)
	reCardEps     = regexp.MustCompile(`mli-eps[^>]*>([^<]+)<`)
	reTotalPages  = regexp.MustCompile(`country/indonesia/page/(\d+)`)
	reYearSuffix  = regexp.MustCompile(`^(.*?)\s*\((\d{4})\)\s*$`)
	reFirstInt    = regexp.MustCompile(`(\d+)`)
)

// parseCatalog extracts the ml-item cards of an archive page.
func parseCatalog(pageHTML, base string) []Item {
	idx := regexp.MustCompile(`ml-item`).FindAllStringIndex(pageHTML, -1)
	out := []Item{}
	seen := map[string]bool{}
	for _, pos := range idx {
		end := pos[0] + 1800
		if end > len(pageHTML) {
			end = len(pageHTML)
		}
		seg := pageHTML[pos[0]:end]
		m := reCardHref.FindStringSubmatch(seg)
		if m == nil {
			continue
		}
		href := html.UnescapeString(m[1])
		if !strings.HasPrefix(href, base) {
			continue
		}
		path := strings.Trim(strings.TrimPrefix(href, base), "/")
		it := Item{}
		if rest, ok := strings.CutPrefix(path, "series/"); ok {
			it.Series = true
			it.Slug = strings.Trim(rest, "/")
		} else {
			it.Slug = path
		}
		if it.Slug == "" || seen[it.Slug] {
			continue
		}
		if hm := reCardH2.FindStringSubmatch(seg); hm != nil {
			title := html.UnescapeString(strings.TrimSpace(hm[1]))
			if ym := reYearSuffix.FindStringSubmatch(title); ym != nil {
				it.Title = strings.TrimSpace(ym[1])
				it.Year, _ = strconv.Atoi(ym[2])
			} else {
				it.Title = title
			}
		}
		if it.Title == "" {
			continue
		}
		seen[it.Slug] = true
		if im := reCardImg.FindStringSubmatch(seg); im != nil {
			it.Poster = html.UnescapeString(im[1])
		}
		if q := reCardQuality.FindStringSubmatch(seg); q != nil {
			it.Quality = strings.TrimSpace(q[1])
		}
		if r := reCardRating.FindStringSubmatch(seg); r != nil {
			it.Rating, _ = strconv.ParseFloat(r[1], 64)
		}
		if d := reCardDurasi.FindStringSubmatch(seg); d != nil {
			it.DurationMin, _ = strconv.Atoi(d[1])
		}
		if e := reCardEps.FindStringSubmatch(seg); e != nil {
			if n := reFirstInt.FindStringSubmatch(e[1]); n != nil {
				it.Episodes, _ = strconv.Atoi(n[1])
			}
		}
		out = append(out, it)
	}
	return out
}

// parseTotalPages reads the archive pagination (largest page link).
func parseTotalPages(pageHTML string) int {
	max := 1
	for _, m := range reTotalPages.FindAllStringSubmatch(pageHTML, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && n > max {
			max = n
		}
	}
	return max
}

var (
	reH1        = regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`)
	reCover     = regexp.MustCompile(`mvi-cover"[^>]*style="background-image:url\(([^)]+)\)`)
	reRatingVal = regexp.MustCompile(`itemprop="ratingValue"[^>]*content="([\d.]+)"`)
	reTrailer   = regexp.MustCompile(`youtube\.com/embed/([\w-]{6,})`)
	reTags      = regexp.MustCompile(`<[^>]+>`)
	reDuration  = regexp.MustCompile(`Duration:\s*(\d+)\s*Min`)
	reRelease   = regexp.MustCompile(`Release Date:\s*([0-9]{4}-[0-9]{2}-[0-9]{2})`)
	reQualityTx = regexp.MustCompile(`Quality:\s*([A-Za-z0-9]+)`)
	reGenreLink = regexp.MustCompile(`href="[^"]*/genre/[^"/]+/"[^>]*>(?:<[^>]+>)*([^<]+)<`)
	reSynopsis  = regexp.MustCompile(`(?s)class="[^"]*sinopsis-indo[^"]*"[^>]*>(.*?)</div>`)
	reDesc      = regexp.MustCompile(`(?s)<div class="desc"[^>]*>(.*?)</div>`)
	reDirector  = regexp.MustCompile(`(?s)Directors?:\s*(.*?)<br`)
	reCast      = regexp.MustCompile(`(?s)Actors?:\s*(.*?)<br`)
)

func stripTags(s string) string {
	return html.UnescapeString(strings.TrimSpace(reTags.ReplaceAllString(s, " ")))
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// parseDetail extracts the metadata block of a detail page. The
// labeled fields (Duration / Release Date / Quality / Directors /
// Actors) are read from the tag-stripped text of the info region,
// where they appear as "Label: value" runs.
func parseDetail(pageHTML string) *Detail {
	d := &Detail{}
	if m := reH1.FindStringSubmatch(pageHTML); m != nil {
		title := collapseSpace(stripTags(m[1]))
		if ym := reYearSuffix.FindStringSubmatch(title); ym != nil {
			d.Title = strings.TrimSpace(ym[1])
			d.Year, _ = strconv.Atoi(ym[2])
		} else {
			d.Title = title
		}
	}
	if d.Title == "" {
		return nil
	}
	if m := reCover.FindStringSubmatch(pageHTML); m != nil {
		d.Poster = html.UnescapeString(m[1])
	}
	if m := reRatingVal.FindStringSubmatch(pageHTML); m != nil {
		d.Rating, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := reTrailer.FindStringSubmatch(pageHTML); m != nil {
		d.TrailerID = m[1]
	}
	// Info region: from the mvi-content block onwards, bounded.
	// Anchor on the class attribute — the bare string also appears
	// in the theme's CSS in <head>.
	region := pageHTML
	if i := strings.Index(pageHTML, `class="mvi-content"`); i >= 0 {
		end := i + 9000
		if end > len(pageHTML) {
			end = len(pageHTML)
		}
		region = pageHTML[i:end]
	}
	text := collapseSpace(stripTags(region))
	if m := reDuration.FindStringSubmatch(text); m != nil {
		d.DurationMin, _ = strconv.Atoi(m[1])
	}
	if m := reRelease.FindStringSubmatch(text); m != nil {
		d.ReleaseDate = m[1]
	}
	if m := reQualityTx.FindStringSubmatch(text); m != nil {
		d.Quality = m[1]
	}
	if m := reDirector.FindStringSubmatch(region); m != nil {
		d.Director = collapseSpace(stripTags(m[1]))
	}
	if m := reCast.FindStringSubmatch(region); m != nil {
		d.Cast = collapseSpace(stripTags(m[1]))
	}
	seenGenre := map[string]bool{}
	for _, m := range reGenreLink.FindAllStringSubmatch(region, -1) {
		g := collapseSpace(m[1])
		if g != "" && !seenGenre[g] {
			seenGenre[g] = true
			d.Genres = append(d.Genres, g)
		}
	}
	if m := reSynopsis.FindStringSubmatch(pageHTML); m != nil {
		d.Overview = collapseSpace(stripTags(m[1]))
	} else if m := reDesc.FindStringSubmatch(pageHTML); m != nil {
		d.Overview = collapseSpace(stripTags(m[1]))
	}
	// The .desc block opens with boilerplate ("Nonton Film X (2026)
	// | KITA NONTON Nonton Film X (2026) – <synopsis>"); cut it.
	if strings.HasPrefix(d.Overview, "Nonton ") {
		if i := strings.Index(d.Overview, "– "); i >= 0 {
			d.Overview = d.Overview[i+len("– "):]
		}
	}
	return d
}

var (
	reServerDiv = regexp.MustCompile(`data-type="([^"]+)"[^>]*?data-iframe="([^"]+)"`)
	reSmall     = regexp.MustCompile(`<small>([^<]*)</small>`)
	reEpLabel   = regexp.MustCompile(`(?i)ep\s*(\d+)`)
)

// parsePlayServers extracts the server list of a /play page. Each
// server div carries data-type + a plain-base64 embed URL in
// data-iframe; the visible <small> label is the quality for movies
// and "Ep N" for series episodes.
func parsePlayServers(pageHTML string) []Server {
	out := []Server{}
	for _, m := range reServerDiv.FindAllStringSubmatchIndex(pageHTML, -1) {
		typ := pageHTML[m[2]:m[3]]
		raw := pageHTML[m[4]:m[5]]
		decoded, err := base64.StdEncoding.DecodeString(padBase64(raw))
		if err != nil || !strings.HasPrefix(string(decoded), "http") {
			continue
		}
		s := Server{Type: typ, URL: string(decoded), Episode: 1}
		end := m[1] + 700
		if end > len(pageHTML) {
			end = len(pageHTML)
		}
		if sm := reSmall.FindStringSubmatch(pageHTML[m[1]:end]); sm != nil {
			s.Label = strings.TrimSpace(sm[1])
			if em := reEpLabel.FindStringSubmatch(s.Label); em != nil {
				s.Episode, _ = strconv.Atoi(em[1])
			}
		}
		out = append(out, s)
	}
	return out
}

func padBase64(s string) string {
	if r := len(s) % 4; r != 0 {
		s += strings.Repeat("=", 4-r)
	}
	return s
}
