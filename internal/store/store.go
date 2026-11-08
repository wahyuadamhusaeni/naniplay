// Package store persists NaniPlay user data (history, favorites) in SQLite.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// HistoryEntry is one row of watch history for an anime episode.
type HistoryEntry struct {
	AnilistID int       `json:"anilistId"`
	Title     string    `json:"title"`
	Poster    string    `json:"poster"`
	Episode   int       `json:"episode"`
	Position  float64   `json:"position"` // seconds
	Duration  float64   `json:"duration"` // seconds
	UpdatedAt time.Time `json:"updatedAt"`
}

// Progress is the fraction watched, 0..1.
func (h HistoryEntry) Progress() float64 {
	if h.Duration <= 0 {
		return 0
	}
	p := h.Position / h.Duration
	if p > 1 {
		return 1
	}
	return p
}

// Favorite is one bookmarked anime.
type Favorite struct {
	AnilistID int       `json:"anilistId"`
	Title     string    `json:"title"`
	Poster    string    `json:"poster"`
	AddedAt   time.Time `json:"addedAt"`
}

// Store wraps the SQLite database.
type Store struct {
	db *sql.DB
}

// Open creates (if needed) and migrates the database inside dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		filepath.Join(dir, "naniplay.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database handle.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS history (
			anilist_id INTEGER NOT NULL,
			episode    INTEGER NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			poster     TEXT NOT NULL DEFAULT '',
			position   REAL NOT NULL DEFAULT 0,
			duration   REAL NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (anilist_id, episode)
		)`,
		`CREATE TABLE IF NOT EXISTS favorites (
			anilist_id INTEGER PRIMARY KEY,
			title      TEXT NOT NULL DEFAULT '',
			poster     TEXT NOT NULL DEFAULT '',
			added_at   INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS subtitle_tracks (
			id         TEXT PRIMARY KEY,
			anilist_id INTEGER NOT NULL,
			episode    INTEGER NOT NULL,
			label      TEXT NOT NULL,
			lang       TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subtitle_tracks ON subtitle_tracks(anilist_id, episode)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// SaveHistory upserts a history entry. Timestamps are unix milliseconds:
// second precision would tie rapid consecutive saves.
func (s *Store) SaveHistory(h HistoryEntry) error {
	_, err := s.db.Exec(
		`INSERT INTO history (anilist_id, episode, title, poster, position, duration, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(anilist_id, episode) DO UPDATE SET
		   title=excluded.title, poster=excluded.poster,
		   position=excluded.position, duration=excluded.duration,
		   updated_at=excluded.updated_at`,
		h.AnilistID, h.Episode, h.Title, h.Poster, h.Position, h.Duration, time.Now().UnixMilli())
	return err
}

// HistoryFor returns the saved entry for one anime episode, if any.
func (s *Store) HistoryFor(anilistID, episode int) (*HistoryEntry, error) {
	row := s.db.QueryRow(
		`SELECT anilist_id, title, poster, episode, position, duration, updated_at
		 FROM history WHERE anilist_id = ? AND episode = ?`, anilistID, episode)
	var h HistoryEntry
	var ts int64
	if err := row.Scan(&h.AnilistID, &h.Title, &h.Poster, &h.Episode, &h.Position, &h.Duration, &ts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	h.UpdatedAt = time.UnixMilli(ts)
	return &h, nil
}

// ContinueWatching returns the most recent episode per anime, newest first.
// Deduplication happens in Go so tied timestamps can never surface the
// same anime twice.
func (s *Store) ContinueWatching(limit int) ([]HistoryEntry, error) {
	fetch := limit * 5
	if fetch < 50 {
		fetch = 50
	}
	rows, err := s.db.Query(
		`SELECT anilist_id, title, poster, episode, position, duration, updated_at
		 FROM history ORDER BY updated_at DESC LIMIT ?`, fetch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanHistory(rows)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := []HistoryEntry{}
	for _, h := range all {
		if seen[h.AnilistID] {
			continue
		}
		seen[h.AnilistID] = true
		out = append(out, h)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// AllHistory returns every history entry, newest first.
func (s *Store) AllHistory(limit int) ([]HistoryEntry, error) {
	rows, err := s.db.Query(
		`SELECT anilist_id, title, poster, episode, position, duration, updated_at
		 FROM history ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanHistory(rows)
}

func scanHistory(rows *sql.Rows) ([]HistoryEntry, error) {
	out := []HistoryEntry{}
	for rows.Next() {
		var h HistoryEntry
		var ts int64
		if err := rows.Scan(&h.AnilistID, &h.Title, &h.Poster, &h.Episode, &h.Position, &h.Duration, &ts); err != nil {
			return nil, err
		}
		h.UpdatedAt = time.UnixMilli(ts)
		out = append(out, h)
	}
	return out, rows.Err()
}

// DeleteHistory removes one anime (all episodes) from history.
func (s *Store) DeleteHistory(anilistID int) error {
	_, err := s.db.Exec(`DELETE FROM history WHERE anilist_id = ?`, anilistID)
	return err
}

// GetSetting returns a settings value, or "" when unset.
func (s *Store) GetSetting(key string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v); err != nil {
		return ""
	}
	return v
}

// SetSetting upserts a settings value.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// SubtitleTrack is a subtitle file saved for one episode. The raw
// content (SRT/VTT/ASS as fetched or uploaded) is stored as-is and
// converted to WebVTT when served, so timing offsets never mutate it.
type SubtitleTrack struct {
	ID        string    `json:"id"`
	AnilistID int       `json:"anilistId"`
	Episode   int       `json:"episode"`
	Label     string    `json:"label"`
	Lang      string    `json:"lang"`
	Source    string    `json:"source"` // upload | external | opensubtitles
	Content   string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

// SaveSubtitleTrack stores a subtitle track.
func (s *Store) SaveSubtitleTrack(t SubtitleTrack) error {
	_, err := s.db.Exec(
		`INSERT INTO subtitle_tracks (id, anilist_id, episode, label, lang, source, content, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.AnilistID, t.Episode, t.Label, t.Lang, t.Source, t.Content, time.Now().Unix())
	return err
}

// SubtitleTracks lists saved tracks for an episode (without content).
func (s *Store) SubtitleTracks(anilistID, episode int) ([]SubtitleTrack, error) {
	rows, err := s.db.Query(
		`SELECT id, anilist_id, episode, label, lang, source, created_at
		 FROM subtitle_tracks WHERE anilist_id = ? AND episode = ?
		 ORDER BY created_at ASC`, anilistID, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubtitleTrack{}
	for rows.Next() {
		var t SubtitleTrack
		var ts int64
		if err := rows.Scan(&t.ID, &t.AnilistID, &t.Episode, &t.Label, &t.Lang, &t.Source, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(ts, 0)
		out = append(out, t)
	}
	return out, rows.Err()
}

// SubtitleTrack returns one track including its content, or nil.
func (s *Store) SubtitleTrack(id string) (*SubtitleTrack, error) {
	var t SubtitleTrack
	var ts int64
	err := s.db.QueryRow(
		`SELECT id, anilist_id, episode, label, lang, source, content, created_at
		 FROM subtitle_tracks WHERE id = ?`, id).
		Scan(&t.ID, &t.AnilistID, &t.Episode, &t.Label, &t.Lang, &t.Source, &t.Content, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(ts, 0)
	return &t, nil
}

// DeleteSubtitleTrack removes a saved track.
func (s *Store) DeleteSubtitleTrack(id string) error {
	_, err := s.db.Exec(`DELETE FROM subtitle_tracks WHERE id = ?`, id)
	return err
}

// ToggleFavorite adds the anime if absent, removes it if present.
// It reports the state after the toggle (true = now a favorite).
func (s *Store) ToggleFavorite(f Favorite) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE anilist_id = ?`, f.AnilistID).Scan(&n)
	if err != nil {
		return false, err
	}
	if n > 0 {
		_, err = s.db.Exec(`DELETE FROM favorites WHERE anilist_id = ?`, f.AnilistID)
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO favorites (anilist_id, title, poster, added_at) VALUES (?, ?, ?, ?)`,
		f.AnilistID, f.Title, f.Poster, time.Now().Unix())
	return err == nil, err
}

// IsFavorite reports whether the anime is bookmarked.
func (s *Store) IsFavorite(anilistID int) bool {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE anilist_id = ?`, anilistID).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// Favorites returns all favorites, newest first.
func (s *Store) Favorites() ([]Favorite, error) {
	rows, err := s.db.Query(`SELECT anilist_id, title, poster, added_at FROM favorites ORDER BY added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		var ts int64
		if err := rows.Scan(&f.AnilistID, &f.Title, &f.Poster, &ts); err != nil {
			return nil, err
		}
		f.AddedAt = time.Unix(ts, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}
