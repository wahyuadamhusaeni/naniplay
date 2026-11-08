// Package aniskip is a small client for the public AniSkip API, which
// crowdsources opening/ending skip intervals keyed by MyAnimeList ID.
package aniskip

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Result is one skip interval. Times are in seconds, expressed against
// EpisodeLength; callers rescale them to the actual file duration.
type Result struct {
	SkipType      string  `json:"type"`
	Start         float64 `json:"start"`
	End           float64 `json:"end"`
	EpisodeLength float64 `json:"episodeLength"`
}

type apiResponse struct {
	Found   bool `json:"found"`
	Results []struct {
		Interval struct {
			StartTime float64 `json:"startTime"`
			EndTime   float64 `json:"endTime"`
		} `json:"interval"`
		SkipType      string  `json:"skipType"`
		EpisodeLength float64 `json:"episodeLength"`
	} `json:"results"`
}

// Client caches results for a day; skip times rarely change.
type Client struct {
	http  *http.Client
	mu    sync.Mutex
	cache map[string][]Result
}

// New returns a ready client.
func New() *Client {
	return &Client{
		http:  &http.Client{Timeout: 12 * time.Second},
		cache: make(map[string][]Result),
	}
}

// SkipTimes returns the OP/ED/recap intervals for one episode.
// A nil slice (with nil error) means no skip times are known.
func (c *Client) SkipTimes(ctx context.Context, malID, episode int) ([]Result, error) {
	key := fmt.Sprintf("%d/%d", malID, episode)
	c.mu.Lock()
	if r, ok := c.cache[key]; ok {
		c.mu.Unlock()
		return r, nil
	}
	c.mu.Unlock()

	url := fmt.Sprintf("https://api.aniskip.com/v2/skip-times/%d/%d?types=op&types=ed&types=mixed-op&types=mixed-ed&types=recap&episodeLength=0", malID, episode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "NaniPlay/0.2 (+https://github.com/wahyuadamhusaeni/naniplay)")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aniskip: HTTP %d", resp.StatusCode)
	}
	var body apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := []Result{}
	if body.Found {
		for _, r := range body.Results {
			out = append(out, Result{
				SkipType:      r.SkipType,
				Start:         r.Interval.StartTime,
				End:           r.Interval.EndTime,
				EpisodeLength: r.EpisodeLength,
			})
		}
	}
	c.mu.Lock()
	c.cache[key] = out
	c.mu.Unlock()
	return out, nil
}
