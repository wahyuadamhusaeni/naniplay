package subtitle

import (
	"math"
	"sort"
	"strings"
)

// MergeCues combines two subtitle tracks into one bilingual track:
// every primary cue gains the text of the secondary cues whose
// midpoint falls inside its window (as extra lines); secondary cues
// that match no primary window stand on their own. The result is
// sorted by start time. This is how viewers pair, for example, an
// Indonesian track with the English original.
func MergeCues(primary, secondary []Cue) []Cue {
	used := make([]bool, len(secondary))
	out := make([]Cue, 0, len(primary)+len(secondary))
	for _, p := range primary {
		merged := p
		var extra []string
		for i, sc := range secondary {
			mid := (sc.Start + sc.End) / 2
			if mid >= p.Start-0.5 && mid <= p.End+0.5 {
				if strings.TrimSpace(sc.Text) != "" {
					extra = append(extra, strings.Join(strings.Fields(strings.ReplaceAll(sc.Text, "\n", " ")), " "))
					used[i] = true
				}
			}
		}
		if len(extra) > 0 {
			merged.Text = strings.TrimSpace(p.Text) + "\n" + strings.Join(extra, " ")
		}
		out = append(out, merged)
	}
	for i, sc := range secondary {
		if !used[i] {
			out = append(out, sc)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// EstimateOffset finds the constant shift (seconds) that best aligns
// the target track's cue starts onto the reference track's, by
// histogramming all pairwise start differences in [-30, +30] s at
// 0.25 s resolution and averaging the winning bin. It returns 0 when
// there is too little evidence (fewer than 3 agreeing pairs) — the
// caller treats that as "already in sync / cannot tell".
func EstimateOffset(reference, target []Cue) float64 {
	if len(reference) == 0 || len(target) == 0 {
		return 0
	}
	type bin struct {
		count int
		sum   float64
	}
	bins := map[int]*bin{}
	for _, t := range target {
		for _, r := range reference {
			d := r.Start - t.Start
			if d < -30 || d > 30 {
				continue
			}
			k := int(math.Round(d / 0.25))
			b, ok := bins[k]
			if !ok {
				b = &bin{}
				bins[k] = b
			}
			b.count++
			b.sum += d
		}
	}
	bestCount := 0
	bestMean := 0.0
	for _, b := range bins {
		mean := b.sum / float64(b.count)
		if b.count > bestCount || (b.count == bestCount && math.Abs(mean) < math.Abs(bestMean)) {
			bestCount, bestMean = b.count, mean
		}
	}
	if bestCount < 3 {
		return 0
	}
	return bestMean
}
