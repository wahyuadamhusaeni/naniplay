package subtitle

import (
	"strings"
	"testing"
)

const sampleSRT = `1
00:00:03,221 --> 00:00:06,500
Halo, <i>dunia</i>!

2
00:00:07,000 --> 00:00:09,250
Baris satu
baris dua.
`

func TestParseSRT(t *testing.T) {
	cues, err := Parse(sampleSRT)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("harus 2 cue, dapat %d", len(cues))
	}
	if cues[0].Start != 3.221 || cues[0].End != 6.5 {
		t.Errorf("waktu cue 1 salah: %v-%v", cues[0].Start, cues[0].End)
	}
	if cues[0].Text != "Halo, dunia!" {
		t.Errorf("tag HTML harus dibuang: %q", cues[0].Text)
	}
	if cues[1].Text != "Baris satu\nbaris dua." {
		t.Errorf("teks multi-baris salah: %q", cues[1].Text)
	}
}

const sampleVTT = `WEBVTT

00:01.000 --> 00:04.500 align:center
Halo dari VTT

cue-2
01:02:03.250 --> 01:02:05.000
Kedua
`

func TestParseVTT(t *testing.T) {
	cues, err := Parse(sampleVTT)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("harus 2 cue, dapat %d", len(cues))
	}
	if cues[1].Start != 3723.25 {
		t.Errorf("waktu cue 2 salah: %v", cues[1].Start)
	}
}

const sampleASS = `[Script Info]
Title: Uji

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:03.22,0:00:06.50,Default,,0,0,0,,{\pos(320,240)}Halo\Ndunia, apa kabar?
Dialogue: 0,0:00:07.00,0:00:09.25,Default,,0,0,0,,Kedua
`

func TestParseASS(t *testing.T) {
	cues, err := Parse(sampleASS)
	if err != nil {
		t.Fatal(err)
	}
	if len(cues) != 2 {
		t.Fatalf("harus 2 cue, dapat %d", len(cues))
	}
	if cues[0].Start != 3.22 {
		t.Errorf("centisecond ASS salah: %v", cues[0].Start)
	}
	if cues[0].Text != "Halo\ndunia, apa kabar?" {
		t.Errorf("override tag / \\N salah dibersihkan: %q", cues[0].Text)
	}
}

func TestToVTTOffset(t *testing.T) {
	cues := []Cue{
		{Start: 1, End: 2, Text: "hilang"},
		{Start: 3, End: 5, Text: "geser <b>aman</b>"},
	}
	out := ToVTT(cues, -2.5)
	if strings.Contains(out, "hilang") {
		t.Error("cue yang tergeser penuh ke negatif harus dibuang")
	}
	if !strings.Contains(out, "00:00:00.500 --> 00:00:02.500") {
		t.Errorf("offset salah:\n%s", out)
	}
	if !strings.Contains(out, "&lt;b&gt;") {
		t.Error("teks harus di-escape untuk VTT")
	}
}

func TestStripAds(t *testing.T) {
	cues := []Cue{
		{Start: 0, End: 5, Text: "Advertise your product or brand here\nopensubtitles.org"},
		{Start: 6, End: 8, Text: "Dialog asli."},
	}
	out := StripAds(cues)
	if len(out) != 1 || out[0].Text != "Dialog asli." {
		t.Errorf("iklan OpenSubtitles harus dibuang: %+v", out)
	}
}

func TestAccurateMatch(t *testing.T) {
	if AccurateMatch("Naruto", "Boruto - Naruto Next Generations S01E01", 1, 1) {
		t.Error("Boruto tidak boleh cocok untuk Naruto")
	}
	if !AccurateMatch("Sousou no Frieren", "[SubsPlease] Sousou no Frieren - 01 (1080p)", 1, 1) {
		t.Error("rilis SubsPlease Frieren ep 1 harus cocok")
	}
	if AccurateMatch("Mushoku Tensei", "Mushoku Tensei S02E03", 1, 3) {
		t.Error("season 2 tidak boleh cocok untuk season 1")
	}
	if AccurateMatch("Naruto", "Naruto S01E05", 1, 2) {
		t.Error("episode 5 tidak boleh cocok untuk episode 2")
	}
}

func TestFileMarkers(t *testing.T) {
	if FileSeason("Show.S02E03.1080p") != 2 || FileEpisode("Show.S02E03.1080p") != 3 {
		t.Error("ekstraksi SxxEyy gagal")
	}
	if FileEpisode("[Group] Show - 07 [1080p]") != 7 {
		t.Error("ekstraksi episode gaya fansub gagal")
	}
	if Base, season := BaseTitle("Mushoku Tensei Season 2"); Base != "Mushoku Tensei" || season != 2 {
		t.Errorf("BaseTitle salah: %q s%d", Base, season)
	}
}
