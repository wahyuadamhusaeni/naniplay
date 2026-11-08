package idlix

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Fetcher is extended for playback: JSON POSTs (session claim against
// the catalog site, redemption against the video host — hence full
// URLs rather than API paths).
//
// The playback flow mirrors the provider's own contract:
//
//	play-info (GET)  → gate token + countdown timestamps
//	(wait until unlockAt)
//	claim    (POST)  → one-shot claim at the video host
//	redeem   (POST)  → HLS master URL + subtitle files

// PostJSON issues a JSON POST to a full URL with extra headers and
// returns the HTTP status and body.
type poster interface {
	PostJSON(ctx context.Context, fullURL string, body []byte, headers map[string]string) (int, []byte, error)
}

// SubFile is one subtitle file offered by the video host.
type SubFile struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	Path  string `json:"path"`
}

// Playback is a resolved, ready-to-play stream.
type Playback struct {
	Title       string
	MasterURL   string
	Subtitles   []SubFile
	DurationSec int
}

type playInfo struct {
	GateToken string `json:"gateToken"`
	ServerNow int64  `json:"serverNow"`
	UnlockAt  int64  `json:"unlockAt"`
}

type claimResp struct {
	Kind        string `json:"kind"`
	Claim       string `json:"claim"`
	RedeemURL   string `json:"redeemUrl"`
	Title       string `json:"title"`
	DurationSec int    `json:"durationSec"`
}

type redeemResp struct {
	URL       string    `json:"url"`
	Subtitles []SubFile `json:"subtitles"`
}

const defaultRedeemURL = "https://e2e.majorplay.net/api/play"

// gateDID extracts the "did" cookie value embedded in a gate token:
// the first segment base64-decodes to JSON whose "owner" is "d:<v>".
func gateDID(token string) string {
	seg, _, _ := strings.Cut(token, ".")
	if seg == "" {
		return ""
	}
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		pad := (4 - len(seg)%4) % 4
		raw, err = base64.URLEncoding.DecodeString(seg + strings.Repeat("=", pad))
		if err != nil {
			return ""
		}
	}
	var obj struct {
		Owner string `json:"owner"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	if v, ok := strings.CutPrefix(obj.Owner, "d:"); ok {
		return "did=" + v
	}
	return ""
}

// ResolvePlayback runs the full playback gate for one movie or
// episode content ID ("movie" / "episode"). The countdown wait
// (usually ~15 s, capped at 20 s) honours ctx cancellation.
func (c *Client) ResolvePlayback(ctx context.Context, contentType, contentID string) (*Playback, error) {
	pj, ok := c.f.(poster)
	if !ok {
		return nil, errors.New("idlix: fetcher tidak mendukung POST")
	}
	base := c.Base()

	var pi playInfo
	if err := c.getJSON(ctx, "/api/watch/play-info/"+contentType+"/"+url.PathEscape(contentID), 0, &pi); err != nil {
		return nil, fmt.Errorf("play-info: %w", err)
	}
	if pi.GateToken == "" {
		return nil, errors.New("play-info: gerbang tidak menerbitkan token")
	}
	wait := time.Duration(pi.UnlockAt-pi.ServerNow)*time.Millisecond + 600*time.Millisecond
	if wait > 20*time.Second {
		wait = 20 * time.Second
	}
	if wait > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}

	claimBody, _ := json.Marshal(map[string]string{"gateToken": pi.GateToken})
	claimHeaders := map[string]string{"Origin": base, "Referer": base + "/"}
	if did := gateDID(pi.GateToken); did != "" {
		claimHeaders["Cookie"] = did
	}
	var claim claimResp
	for attempt := 0; attempt < 10; attempt++ {
		code, raw, err := pj.PostJSON(ctx, base+"/api/watch/session/claim", claimBody, claimHeaders)
		if err == nil && code == 200 {
			_ = json.Unmarshal(raw, &claim)
			if claim.Kind == "pentos" && claim.Claim != "" {
				break
			}
			claim = claimResp{}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(600 * time.Millisecond):
		}
	}
	if claim.Claim == "" {
		return nil, errors.New("klaim sesi tidak terbit setelah 10 percobaan")
	}

	redeemURL := claim.RedeemURL
	if redeemURL == "" {
		redeemURL = defaultRedeemURL
	}
	redeemBody, _ := json.Marshal(map[string]string{"claim": claim.Claim, "mode": "browser"})
	code, raw, err := pj.PostJSON(ctx, redeemURL, redeemBody, map[string]string{"Origin": base, "Referer": base + "/"})
	if err != nil {
		return nil, fmt.Errorf("redeem: %w", err)
	}
	if code != 200 {
		return nil, fmt.Errorf("redeem: HTTP %d", code)
	}
	var rd redeemResp
	if err := json.Unmarshal(raw, &rd); err != nil || rd.URL == "" {
		return nil, errors.New("redeem: respons tanpa URL putar")
	}
	return &Playback{
		Title:       claim.Title,
		MasterURL:   rd.URL,
		Subtitles:   rd.Subtitles,
		DurationSec: claim.DurationSec,
	}, nil
}

// EpisodeResponse is the full payload of the episode endpoint:
// the series, the season (with episodes), and the episode itself.
type EpisodeResponse struct {
	Series  *Series       `json:"series"`
	Season  *SeasonDetail `json:"season"`
	Episode *Episode      `json:"episode"`
}

// Episode fetches one episode with its series and season context —
// the source of the content ID playback needs.
func (c *Client) Episode(ctx context.Context, slug string, season, episode int) (*EpisodeResponse, error) {
	path := fmt.Sprintf("/api/series/%s/season/%d/episode/%d", url.PathEscape(slug), season, episode)
	var out EpisodeResponse
	if err := c.getJSON(ctx, path, 20*time.Minute, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
