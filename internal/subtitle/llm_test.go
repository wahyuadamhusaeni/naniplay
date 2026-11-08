package subtitle

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockLLM serves a canned numbered translation; when dropLine > 0 it
// omits that line number so the GTX fill path is exercised.
func mockLLM(t *testing.T, dropLine int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("request tidak terbaca: %v", err)
		}
		var lines []string
		for _, ln := range strings.Split(req.Messages[1].Content, "\n") {
			m := numberedLineRe.FindStringSubmatch(ln)
			if m == nil {
				continue
			}
			if dropLine > 0 && m[1] == fmt.Sprint(dropLine) {
				continue // model "lupa" baris ini
			}
			lines = append(lines, m[1]+". [ID] "+m[2])
		}
		resp := map[string]any{"choices": []any{map[string]any{"message": map[string]string{
			"role": "assistant", "content": strings.Join(lines, "\n"),
		}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockGTXServer answers any GTX request with a "[GTX] "-prefixed echo.
func mockGTXServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.FormValue("q")
		resp := []any{
			[]any{[]any{"[GTX] " + q, q, nil, nil, 3}},
			nil, "en",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func llmTestCues() []Cue {
	return []Cue{
		{Start: 1.0, End: 2.0, Text: "Hello world"},
		{Start: 3.0, End: 4.0, Text: "I don't give up!"},
	}
}

func TestTranslateCuesLLM(t *testing.T) {
	srv := mockLLM(t, 0)
	cfg := LLMConfig{BaseURL: srv.URL + "/v1", Model: "mock-1"}
	out, err := TranslateCuesLLM(context.Background(), llmTestCues(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Text != "[ID] Hello world" {
		t.Fatalf("hasil salah: %+v", out)
	}
	if out[1].Text != "[ID] I do not give up!" {
		t.Errorf("kontraksi harus diperluas sebelum dikirim: %q", out[1].Text)
	}
	if out[0].Start != 1.0 || out[1].End != 4.0 {
		t.Error("timing tidak boleh berubah")
	}
}

func TestTranslateCuesLLMDroppedLineFilledByGTX(t *testing.T) {
	srv := mockLLM(t, 2)
	// Mock GTX echoes a marker translation.
	oldBase := gtxBase
	gtxBase = mockGTXServer(t).URL
	defer func() { gtxBase = oldBase }()

	cfg := LLMConfig{BaseURL: srv.URL + "/v1", Model: "mock-1"}
	out, err := TranslateCuesLLM(context.Background(), llmTestCues(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Text != "[ID] Hello world" {
		t.Errorf("baris 1 harus dari LLM: %q", out[0].Text)
	}
	if !strings.HasPrefix(out[1].Text, "[GTX]") {
		t.Errorf("baris 2 yang dijatuhkan model harus diisi GTX, dapat: %q", out[1].Text)
	}
}

func TestTranslateCuesLLMUnconfigured(t *testing.T) {
	if _, err := TranslateCuesLLM(context.Background(), llmTestCues(), LLMConfig{}); err == nil {
		t.Error("endpoint kosong harus error")
	}
}
