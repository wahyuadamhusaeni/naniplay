package subtitle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// AI fansub (BYOK): translate subtitles with any OpenAI-compatible
// chat endpoint — OpenAI, OpenRouter, Gemini, Groq, a local Ollama,
// or a personal router. The model receives numbered cue lines and
// must answer with the same numbering, which keeps timing alignment
// exact. Lines a model drops are filled by the GTX machine
// translator (the hybrid idea: never all-or-nothing), and a line
// that still fails keeps its original text.

// LLMConfig is a user-supplied OpenAI-compatible endpoint.
type LLMConfig struct {
	BaseURL string // e.g. https://api.openai.com/v1
	APIKey  string // may be empty for local endpoints
	Model   string // e.g. gpt-4o-mini
}

// Configured reports whether the endpoint can be used at all.
func (c LLMConfig) Configured() bool {
	return strings.TrimSpace(c.BaseURL) != "" && strings.TrimSpace(c.Model) != ""
}

var llmHTTP = &http.Client{Timeout: 120 * time.Second}

const fansubSystemPrompt = `Kamu adalah fansubber anime profesional yang menerjemahkan subtitle ke bahasa Indonesia.

Aturan:
- Terjemahkan setiap baris bernomor ke bahasa Indonesia yang alami dan luwes seperti percakapan sehari-hari, BUKAN terjemahan harfiah yang kaku.
- Pertahankan sapaan khas anime (-san, -kun, -chan, -senpai, Sensei) dan semua nama orang/tempat/jurus.
- Jangan menambahkan, menghapus, menggabungkan, atau membagi baris.
- Keluarkan HANYA baris-baris bernomor dengan nomor yang sama persis seperti masukan, tanpa penjelasan apa pun.`

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// llmChat sends one chat completion and returns the text content.
func llmChat(ctx context.Context, cfg LLMConfig, userPrompt string) (string, error) {
	base := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	payload, err := json.Marshal(chatRequest{
		Model: cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: fansubSystemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", browserUA)
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	resp, err := llmHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("endpoint AI tidak terjangkau: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("endpoint AI menjawab HTTP %d — periksa alamat, kunci, dan nama model", resp.StatusCode)
	}
	var data chatResponse
	if err := json.Unmarshal(body, &data); err != nil || len(data.Choices) == 0 {
		return "", fmt.Errorf("respons endpoint AI tidak terbaca")
	}
	return strings.TrimSpace(data.Choices[0].Message.Content), nil
}

var numberedLineRe = regexp.MustCompile(`^\s*(\d+)[\.\)]\s*(.+)$`)

// parseNumbered extracts "N. text" lines into a 1-based map.
func parseNumbered(out string) map[int]string {
	res := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		if m := numberedLineRe.FindStringSubmatch(line); m != nil {
			n, err := strconv.Atoi(m[1])
			if err == nil {
				res[n] = strings.TrimSpace(m[2])
			}
		}
	}
	return res
}

// TranslateCuesLLM translates all cue texts with the configured
// model, batching by numbered lines. Timings are untouched. A batch
// that the endpoint rejects aborts the whole translation — a track
// labeled "AI Fansub" must never silently be half machine output.
func TranslateCuesLLM(ctx context.Context, cues []Cue, cfg LLMConfig) ([]Cue, error) {
	if !cfg.Configured() {
		return nil, fmt.Errorf("endpoint AI belum diatur")
	}
	if len(cues) == 0 {
		return cues, nil
	}
	texts := make([]string, len(cues))
	for i, cue := range cues {
		flat := strings.Join(strings.Fields(strings.ReplaceAll(cue.Text, "\n", " ")), " ")
		texts[i] = expandContractions(flat)
	}
	translated := make([]string, len(texts))

	const batchSize = 30
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		var sb strings.Builder
		for i := start; i < end; i++ {
			fmt.Fprintf(&sb, "%d. %s\n", i-start+1, texts[i])
		}
		out, err := llmChat(ctx, cfg, sb.String())
		if err != nil {
			return nil, err
		}
		got := parseNumbered(out)
		for i := start; i < end; i++ {
			local := i - start + 1
			if text, ok := got[local]; ok && text != "" {
				translated[i] = text
				continue
			}
			// Hybrid fill: a dropped line falls back to GTX, then to
			// the original text — the episode still completes.
			if filled, err := gtxTranslateText(ctx, texts[i], "auto", "id"); err == nil && filled != "" {
				translated[i] = filled
			} else {
				translated[i] = texts[i]
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	out := make([]Cue, len(cues))
	for i, cue := range cues {
		out[i] = Cue{Start: cue.Start, End: cue.End, Text: translated[i]}
	}
	return out, nil
}
