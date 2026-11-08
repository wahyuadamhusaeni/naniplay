package hianime

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMegaDecryptRoundtrip(t *testing.T) {
	plain := []byte(`{"file":"https://cdn.example/master.m3u8"}`)
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	for i := 0; i < pad; i++ {
		plain = append(plain, byte(pad))
	}
	block, _ := aes.NewCipher(megaKey)
	enc := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, megaIV).CryptBlocks(enc, plain)
	got, err := megaDecrypt(base64.URLEncoding.EncodeToString(enc))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "master.m3u8") {
		t.Errorf("hasil dekripsi salah: %q", got)
	}
}

func TestParseServersOrdersMegaplayFirst(t *testing.T) {
	zoko := base64.StdEncoding.EncodeToString([]byte("https://zokoanime.video/stream/mal/1/1/sub"))
	mega := base64.StdEncoding.EncodeToString([]byte("https://megaplay.buzz/stream/s-2/123/sub"))
	page := fmt.Sprintf(`
	<div class="server-item" data-type="sub" data-hash="%s" data-server-name="ZokoAnime"></div>
	<div class="server-item" data-type="dub" data-hash="%s" data-server-name="DubServer"></div>
	<div class="server-item" data-type="sub" data-hash="%s" data-server-name="HD-1"></div>`, zoko, mega, mega)
	servers := parseServers(page)
	if len(servers) != 2 {
		t.Fatalf("server sub harusnya 2 (dub disaring), dapat %d", len(servers))
	}
	if !strings.Contains(servers[0].embed, "megaplay") {
		t.Errorf("Megaplay harus diutamakan, dapat %s", servers[0].embed)
	}
}

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<div class="flw-item"><h3 class="film-name">
			<a href="https://hianime.at/frieren-beyond-journeys-end-481" title="Frieren: Beyond Journey&#039;s End" class="dynamic-name" data-jname="Sousou no Frieren">Frieren</a></h3></div>`)
	})
	mux.HandleFunc("/api/theme/episode/list/481", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"html":"<a data-number=\"1\" data-id=\"9227\">1</a><a data-number=\"2\" data-id=\"9228\">2</a>"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSearchAndEpisodes(t *testing.T) {
	srv := testServer(t)
	c := New(srv.URL)
	results, err := c.Search(context.Background(), "Frieren", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != "frieren-beyond-journeys-end-481" {
		t.Fatalf("hasil pencarian salah: %+v", results)
	}
	if results[0].Title != "Sousou no Frieren" {
		t.Errorf("judul harus memakai nama romaji: %q", results[0].Title)
	}
	eps, err := c.Episodes(context.Background(), "frieren-beyond-journeys-end-481")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 2 || eps[0].Number != 1 || !strings.Contains(eps[0].URL, "ep=9227") {
		t.Errorf("episode salah: %+v", eps)
	}
}

func TestLangFromLabel(t *testing.T) {
	if langFromLabel("English") != "en" || langFromLabel("Indonesian") != "id" || langFromLabel("Arabic") != "ar" {
		t.Error("pemetaan bahasa salah")
	}
	if langFromLabel("Thumbnails") != "" {
		t.Error("label asing harusnya kosong")
	}
}
