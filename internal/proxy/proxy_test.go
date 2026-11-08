package proxy

import (
	"net/url"
	"strings"
	"testing"
)

func TestRewritePlaylistSegments(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/hls/master.m3u8")
	body := "#EXTM3U\n#EXT-X-VERSION:3\nseg1.ts\n/sub/720.m3u8\nhttps://other.example.com/abs.ts\n"
	out := rewritePlaylist(body, base, "https://ref.example.com/")
	if !strings.Contains(out, "#EXTM3U") {
		t.Error("header playlist hilang")
	}
	if strings.Contains(out, "\nseg1.ts") || strings.Contains(out, "other.example.com/abs.ts\n") {
		t.Error("URL segmen tidak ditulis ulang ke proxy")
	}
	if !strings.Contains(out, "/api/proxy?") {
		t.Error("hasil harus merutekan segmen lewat /api/proxy")
	}
	// The segment must resolve against the playlist directory.
	if !strings.Contains(out, "cdn.example.com%2Fhls%2Fseg1.ts") {
		t.Errorf("segmen relatif tidak ter-resolve terhadap direktori playlist:\n%s", out)
	}
}

func TestRewritePlaylistKeyURI(t *testing.T) {
	base, _ := url.Parse("https://cdn.example.com/hls/master.m3u8")
	body := "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\nseg.ts\n"
	out := rewritePlaylist(body, base, "")
	if !strings.Contains(out, `URI="/api/proxy?`) {
		t.Errorf("URI kunci tidak ditulis ulang:\n%s", out)
	}
}
