package subtitle

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTranslateCuesViaMockGTX(t *testing.T) {
	var lastQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQuery = r.FormValue("q")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[[["Halo dunia\n","Hello world\n",null,null,10],["Apa kabar?","How are you?",null,null,10]],null,"en"]`)
	}))
	defer srv.Close()
	old := gtxBase
	gtxBase = srv.URL
	defer func() { gtxBase = old }()

	cues := []Cue{
		{Start: 1, End: 2.5, Text: "Hello world"},
		{Start: 3, End: 4.5, Text: "How are you?"},
	}
	out, err := TranslateCues(context.Background(), cues, "auto", "id")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("harus 2 cue, dapat %d", len(out))
	}
	if out[0].Text != "Halo dunia" || out[1].Text != "Apa kabar?" {
		t.Errorf("hasil terjemahan salah: %q / %q", out[0].Text, out[1].Text)
	}
	if out[0].Start != 1 || out[1].End != 4.5 {
		t.Error("timing tidak boleh berubah")
	}
	if !strings.Contains(lastQuery, "Hello world") {
		t.Errorf("permintaan GTX tidak membawa teks sumber: %q", lastQuery)
	}
}

func TestExpandContractions(t *testing.T) {
	in := "I don't know, and he can't come."
	out := expandContractions(in)
	if !strings.Contains(out, "do not") || !strings.Contains(out, "cannot") {
		t.Errorf("kontraksi tidak diperluas: %q", out)
	}
}
