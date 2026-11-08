package subtitle

import (
	"math"
	"strings"
	"testing"
)

func TestMergeCues(t *testing.T) {
	id := []Cue{
		{Start: 0, End: 2, Text: "Halo."},
		{Start: 3, End: 5, Text: "Apa kabar?"},
	}
	en := []Cue{
		{Start: 0.2, End: 1.8, Text: "Hello."},
		{Start: 3.1, End: 4.9, Text: "How are you?"},
		{Start: 20, End: 22, Text: "Unmatched line."},
	}
	out := MergeCues(id, en)
	if len(out) != 3 {
		t.Fatalf("hasil %d cue, mau 3: %+v", len(out), out)
	}
	if !strings.Contains(out[0].Text, "Halo.") || !strings.Contains(out[0].Text, "Hello.") {
		t.Fatalf("cue 1 tidak tergabung: %q", out[0].Text)
	}
	if !strings.Contains(out[1].Text, "How are you?") {
		t.Fatalf("cue 2 tidak tergabung: %q", out[1].Text)
	}
	if out[2].Text != "Unmatched line." {
		t.Fatalf("cue yatim salah: %+v", out[2])
	}
}

func TestEstimateOffset(t *testing.T) {
	var ref, tgt []Cue
	for i := 0; i < 12; i++ {
		start := float64(i * 7)
		ref = append(ref, Cue{Start: start, End: start + 2, Text: "x"})
		tgt = append(tgt, Cue{Start: start - 3.7, End: start - 1.7, Text: "y"})
	}
	got := EstimateOffset(ref, tgt)
	if math.Abs(got-3.7) > 0.3 {
		t.Fatalf("EstimateOffset = %.2f, mau ≈3.7", got)
	}
	if got := EstimateOffset(ref, ref); math.Abs(got) > 0.3 {
		t.Fatalf("trek identik offset %.2f, mau ≈0", got)
	}
	if got := EstimateOffset(ref, nil); got != 0 {
		t.Fatalf("target kosong offset %.2f, mau 0", got)
	}
}
