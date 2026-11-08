// Package source defines the streaming-source provider contract.
// Every source (Otakudesu today, more later) implements Provider so a
// broken or changed source stays isolated in its own module.
package source

import "context"

// Series is one anime found on a source.
type Series struct {
	ID     string `json:"id"` // source-local slug
	Title  string `json:"title"`
	Poster string `json:"poster"`
	URL    string `json:"url"`
}

// Episode is one playable episode of a series.
type Episode struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"` // episode page URL on the source
}

// Stream is a resolved, playable stream for an episode.
type Stream struct {
	MasterURL string `json:"masterUrl"` // HLS master playlist
	Referer   string `json:"referer"`   // Referer the CDN expects
	Server    string `json:"server"`    // human label, e.g. "Otakudesu (Sub Indo)"
	SubLang   string `json:"subLang"`   // subtitle note, e.g. "Indonesian (Hardsub)"
}

// Provider is the contract every streaming source implements.
type Provider interface {
	// Name is the stable provider id, e.g. "otakudesu".
	Name() string
	// Search finds series on the source by title.
	Search(ctx context.Context, query string, limit int) ([]Series, error)
	// Episodes lists the episodes of a series id (slug), ascending.
	Episodes(ctx context.Context, seriesID string) ([]Episode, error)
	// Resolve turns an episode page URL into a playable stream.
	Resolve(ctx context.Context, episodeURL string) (*Stream, error)
}
