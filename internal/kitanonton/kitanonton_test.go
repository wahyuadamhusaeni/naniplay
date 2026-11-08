package kitanonton

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseCatalog(t *testing.T) {
	items := parseCatalog(fixture(t, "catalog.html"), "https://kitanonton2.garden")
	if len(items) == 0 {
		t.Fatal("tidak ada kartu terbaca")
	}
	var movie, series *Item
	for i := range items {
		if items[i].Series && series == nil {
			series = &items[i]
		}
		if !items[i].Series && movie == nil {
			movie = &items[i]
		}
	}
	if movie == nil {
		t.Fatal("tidak ada film terbaca")
	}
	if movie.Title == "" || movie.Slug == "" {
		t.Fatalf("film tidak lengkap: %+v", *movie)
	}
	if movie.Year < 2000 || movie.Year > 2100 {
		t.Fatalf("tahun film janggal: %+v", *movie)
	}
	if series == nil {
		t.Fatal("tidak ada series terbaca")
	}
	if series.Slug == "" || strings.Contains(series.Slug, "/") {
		t.Fatalf("slug series janggal: %+v", *series)
	}
	t.Logf("film: %+v", *movie)
	t.Logf("series: %+v", *series)
}

func TestParseDetail(t *testing.T) {
	d := parseDetail(fixture(t, "detail.html"))
	if d == nil {
		t.Fatal("detail tidak terbaca")
	}
	if d.Title == "" {
		t.Fatal("judul kosong")
	}
	if d.DurationMin <= 0 {
		t.Fatalf("durasi tidak terbaca: %+v", d)
	}
	if d.Rating <= 0 {
		t.Fatalf("rating tidak terbaca: %+v", d)
	}
	t.Logf("detail: judul=%q tahun=%d durasi=%d rating=%.1f poster=%q trailer=%q genres=%v",
		d.Title, d.Year, d.DurationMin, d.Rating, d.Poster, d.TrailerID, d.Genres)
}

func TestParsePlayServersMovie(t *testing.T) {
	servers := parsePlayServers(fixture(t, "play.html"))
	if len(servers) == 0 {
		t.Fatal("tidak ada server terbaca")
	}
	var dv int
	for _, s := range servers {
		if !strings.HasPrefix(s.URL, "http") {
			t.Fatalf("url server janggal: %+v", s)
		}
		if s.Type == "dv" {
			dv++
		}
		if s.Episode != 1 {
			t.Fatalf("episode film harus 1: %+v", s)
		}
	}
	if dv == 0 {
		t.Fatal("tidak ada server dv")
	}
	t.Logf("server film: %+v", servers)
}

func TestParsePlayServersSeries(t *testing.T) {
	servers := parsePlayServers(fixture(t, "playseries.html"))
	if len(servers) < 2 {
		t.Fatalf("server series terlalu sedikit: %d", len(servers))
	}
	eps := map[int]bool{}
	for _, s := range servers {
		eps[s.Episode] = true
	}
	if len(eps) < 2 {
		t.Fatalf("episode series tidak terkelompok: %v", eps)
	}
	if got := EpisodeCount(servers); got < 2 {
		t.Fatalf("EpisodeCount=%d", got)
	}
	t.Logf("server series: %d entri, episode %v", len(servers), eps)
}

func TestParseEmbed(t *testing.T) {
	st, err := parseEmbed(fixture(t, "embed.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(st.MasterURL, ".m3u8") {
		t.Fatalf("master bukan m3u8: %q", st.MasterURL)
	}
	if !strings.Contains(st.MasterURL, "groovy.monster") {
		t.Fatalf("master bukan CDN juicy: %q", st.MasterURL)
	}
	if len(st.Labels) == 0 {
		t.Fatal("label kualitas kosong")
	}
	t.Logf("master: %q labels: %v", st.MasterURL, st.Labels)
}

func TestMintAndSign(t *testing.T) {
	tok := MintToken("203.0.113.7", "TestAgent/1.0")
	if tok == "" {
		t.Fatal("token kosong")
	}
	// Tanpa token lama: ditempel.
	got := SignForViewer("https://cdn.example/stream/abc.m3u8", "203.0.113.7", "TestAgent/1.0")
	if !strings.HasSuffix(got, "?token="+tok) {
		t.Fatalf("tempel token salah: %q", got)
	}
	// Dengan token lama: diganti, parameter lain utuh.
	got = SignForViewer("https://cdn.example/stream/abc.m3u8?token=LAMA&x=1", "203.0.113.7", "TestAgent/1.0")
	if strings.Contains(got, "LAMA") || !strings.Contains(got, "token="+tok) || !strings.HasSuffix(got, "&x=1") {
		t.Fatalf("ganti token salah: %q", got)
	}
}
