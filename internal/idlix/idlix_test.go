package idlix

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureFetcher serves recorded API responses from testdata, keyed by
// a substring of the request path.
type fixtureFetcher struct {
	routes map[string]string // path substring -> fixture file
}

func (f fixtureFetcher) Get(_ context.Context, rawURL string) (int, []byte, error) {
	for sub, file := range f.routes {
		if strings.Contains(rawURL, sub) {
			b, err := os.ReadFile(filepath.Join("testdata", file))
			return 200, b, err
		}
	}
	return 404, []byte(`{"message":"not found"}`), nil
}

func testClient() *Client {
	return NewWithFetcher("https://idlix.example", fixtureFetcher{routes: map[string]string{
		"/api/movies?": "movies.json",
		"/api/movies/": "movie.json",
		"/api/series?": "series.json",
		"/api/series/": "series_detail.json",
		"/api/genres":  "genres.json",
		"/api/search":  "search.json",
	}})
}

func TestMoviesList(t *testing.T) {
	items, pg, err := testClient().Movies(context.Background(), 1, 3, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("dapat %d film, mau 2", len(items))
	}
	if items[0].Title == "" || items[0].Slug == "" {
		t.Errorf("judul/slug kosong: %+v", items[0])
	}
	if items[0].Poster() == "" || !strings.Contains(items[0].Poster(), "image.tmdb.org/t/p/w342") {
		t.Errorf("URL poster salah: %q", items[0].Poster())
	}
	if pg.Total <= 0 || pg.TotalPages <= 0 {
		t.Errorf("pagination tidak masuk akal: %+v", pg)
	}
	if items[0].Rating.Float() <= 0 {
		t.Errorf("rating string tidak terparse: %v", items[0].Rating.Float())
	}
}

func TestMovieDetail(t *testing.T) {
	m, err := testClient().Movie(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if m.Title == "" || len(m.Genres) == 0 {
		t.Errorf("detail tidak lengkap: %+v", m)
	}
}

func TestSeriesDetailDefaultSeason(t *testing.T) {
	s, err := testClient().SeriesDetail(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultSeason == nil || len(s.DefaultSeason.Episodes) == 0 {
		t.Fatalf("defaultSeason/episode hilang: %+v", s.DefaultSeason)
	}
	if len(s.Seasons) == 0 {
		t.Error("daftar season kosong")
	}
	ep := s.DefaultSeason.Episodes[0]
	if ep.EpisodeNumber != 1 || ep.Still() == "" {
		t.Errorf("episode pertama janggal: %+v", ep)
	}
}

func TestGenresAndSearch(t *testing.T) {
	g, err := testClient().Genres(context.Background())
	if err != nil || len(g) == 0 || g[0].Slug == "" {
		t.Fatalf("genre: %v / %+v", err, g)
	}
	res, total, err := testClient().Search(context.Background(), "avengers")
	if err != nil || len(res) == 0 || total < len(res) {
		t.Fatalf("search: %v / %d hasil dari total %d", err, len(res), total)
	}
	kinds := map[string]bool{}
	for _, r := range res {
		kinds[r.ContentType] = true
	}
	if !kinds["movie"] && !kinds["tv_series"] {
		t.Errorf("contentType tidak dikenal: %v", kinds)
	}
}

func TestRatingFlexible(t *testing.T) {
	var r Rating
	for in, want := range map[string]float64{`"6.50"`: 6.5, `7.25`: 7.25, `null`: 0, `""`: 0} {
		if err := r.UnmarshalJSON([]byte(in)); err != nil {
			t.Fatalf("unmarshal %s: %v", in, err)
		}
		if r.Float() != want {
			t.Errorf("%s -> %v, mau %v", in, r.Float(), want)
		}
	}
}

// flowFetcher serves the playback conversation from canned responses.
type flowFetcher struct {
	gets  map[string]string
	posts map[string]string
}

func (f *flowFetcher) Get(ctx context.Context, rawURL string) (int, []byte, error) {
	for path, body := range f.gets {
		if strings.Contains(rawURL, path) {
			return 200, []byte(body), nil
		}
	}
	return 404, []byte(`{"message":"not found"}`), nil
}

func (f *flowFetcher) PostJSON(ctx context.Context, fullURL string, body []byte, headers map[string]string) (int, []byte, error) {
	for path, respBody := range f.posts {
		if strings.Contains(fullURL, path) {
			return 200, []byte(respBody), nil
		}
	}
	return 500, []byte(`{}`), nil
}

func TestResolvePlaybackFlow(t *testing.T) {
	// gate token segment: {"owner":"d:device42"} dalam base64url
	seg := "eyJvd25lciI6ImQ6ZGV2aWNlNDIifQ"
	ff := &flowFetcher{
		gets: map[string]string{
			"/api/watch/play-info/movie/abc": `{"gateToken":"` + seg + `.x.y","serverNow":1000,"unlockAt":1000}`,
		},
		posts: map[string]string{
			"/api/watch/session/claim":      `{"kind":"pentos","claim":"clm-1","redeemUrl":"https://host.example/api/play","title":"Film Uji","durationSec":5400}`,
			"https://host.example/api/play": `{"url":"https://cdn.example/master.m3u8","subtitles":[{"lang":"en","label":"English","path":"https://cdn.example/en.vtt"}]}`,
		},
	}
	c := NewWithFetcher("https://idlix.example", ff)
	pb, err := c.ResolvePlayback(context.Background(), "movie", "abc")
	if err != nil {
		t.Fatalf("ResolvePlayback: %v", err)
	}
	if pb.MasterURL != "https://cdn.example/master.m3u8" || pb.Title != "Film Uji" || len(pb.Subtitles) != 1 {
		t.Fatalf("playback salah: %+v", pb)
	}
}

func TestGateDID(t *testing.T) {
	if got := gateDID("eyJvd25lciI6ImQ6ZGV2aWNlNDIifQ.x.y"); got != "did=device42" {
		t.Fatalf("gateDID = %q", got)
	}
	if got := gateDID("bukan-token"); got != "" {
		t.Fatalf("gateDID token rusak = %q, mau kosong", got)
	}
}
