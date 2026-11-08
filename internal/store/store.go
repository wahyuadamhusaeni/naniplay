// Package store persists NaniPlay user data (history, favorites) in SQLite.
//
// Since schema v2 (v0.9.0) every user-data row is keyed by a media key —
// "anime:{anilistID}", "movie:{slug}", "series:{slug}" — so anime, movies
// and series share one history/favorites/subtitle system. Databases created
// by older versions are migrated in place on Open.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// AnimeKey builds the media key for an AniList title.
func AnimeKey(id int) string { return "anime:" + strconv.Itoa(id) }

// ParseAnimeKey extracts the AniList ID from an anime media key.
func ParseAnimeKey(key string) (int, bool) {
	rest, ok := strings.CutPrefix(key, "anime:")
	if !ok {
		return 0, false
	}
	id, err := strconv.Atoi(rest)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// HistoryEntry is one row of watch history for one episode (or a movie,
// where Episode is 1). Key is the media key; AnilistID is derived from
// the key on reads for anime titles (0 for other kinds) to keep older
// templates and JSON consumers working.
type HistoryEntry struct {
	Key       string    `json:"key"`
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

// Favorite is one bookmarked title.
type Favorite struct {
	Key       string    `json:"key"`
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

const schemaVersion = 2

var schemaV2 = []string{
	`CREATE TABLE IF NOT EXISTS history (
		media_key  TEXT NOT NULL,
		episode    INTEGER NOT NULL,
		title      TEXT NOT NULL DEFAULT '',
		poster     TEXT NOT NULL DEFAULT '',
		position   REAL NOT NULL DEFAULT 0,
		duration   REAL NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (media_key, episode)
	)`,
	`CREATE TABLE IF NOT EXISTS favorites (
		media_key TEXT PRIMARY KEY,
		title     TEXT NOT NULL DEFAULT '',
		poster    TEXT NOT NULL DEFAULT '',
		added_at  INTEGER NOT NULL DEFAULT 0
	)`,
	`CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`,
	`CREATE TABLE IF NOT EXISTS subtitle_tracks (
		id         TEXT PRIMARY KEY,
		media_key  TEXT NOT NULL,
		episode    INTEGER NOT NULL,
		label      TEXT NOT NULL,
		lang       TEXT NOT NULL DEFAULT '',
		source     TEXT NOT NULL,
		content    TEXT NOT NULL,
		created_at INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_subtitle_tracks ON subtitle_tracks(media_key, episode)`,
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version >= schemaVersion {
		return s.createSchema()
	}
	legacy, err := s.hasLegacyHistory()
	if err != nil {
		return err
	}
	if !legacy {
		if err := s.createSchema(); err != nil {
			return err
		}
		_, err = s.db.Exec(`PRAGMA user_version = 2`)
		return err
	}
	return s.migrateV1toV2()
}

func (s *Store) createSchema() error {
	for _, q := range schemaV2 {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// hasLegacyHistory reports whether a v1 history table (keyed by
// anilist_id) exists.
func (s *Store) hasLegacyHistory() (bool, error) {
	rows, err := s.db.Query(`PRAGMA table_info(history)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == "anilist_id" {
			found = true
		}
		if name == "media_key" {
			return false, nil // already v2-shaped; version stamp handled by caller path
		}
	}
	return found, rows.Err()
}

// migrateV1toV2 rebuilds the three user-data tables around media keys in
// a single transaction; existing rows keep their data with
// "anime:{anilist_id}" keys.
func (s *Store) migrateV1toV2() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE history_v2 (
			media_key  TEXT NOT NULL,
			episode    INTEGER NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			poster     TEXT NOT NULL DEFAULT '',
			position   REAL NOT NULL DEFAULT 0,
			duration   REAL NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (media_key, episode)
		)`,
		`INSERT INTO history_v2 (media_key, episode, title, poster, position, duration, updated_at)
		 SELECT 'anime:' || anilist_id, episode, title, poster, position, duration, updated_at FROM history`,
		`DROP TABLE history`,
		`ALTER TABLE history_v2 RENAME TO history`,
		`CREATE TABLE favorites_v2 (
			media_key TEXT PRIMARY KEY,
			title     TEXT NOT NULL DEFAULT '',
			poster    TEXT NOT NULL DEFAULT '',
			added_at  INTEGER NOT NULL DEFAULT 0
		)`,
		`INSERT INTO favorites_v2 (media_key, title, poster, added_at)
		 SELECT 'anime:' || anilist_id, title, poster, added_at FROM favorites`,
		`DROP TABLE favorites`,
		`ALTER TABLE favorites_v2 RENAME TO favorites`,
		`CREATE TABLE subtitle_tracks_v2 (
			id         TEXT PRIMARY KEY,
			media_key  TEXT NOT NULL,
			episode    INTEGER NOT NULL,
			label      TEXT NOT NULL,
			lang       TEXT NOT NULL DEFAULT '',
			source     TEXT NOT NULL,
			content    TEXT NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`INSERT INTO subtitle_tracks_v2 (id, media_key, episode, label, lang, source, content, created_at)
		 SELECT id, 'anime:' || anilist_id, episode, label, lang, source, content, created_at FROM subtitle_tracks`,
		`DROP TABLE subtitle_tracks`,
		`ALTER TABLE subtitle_tracks_v2 RENAME TO subtitle_tracks`,
		`CREATE INDEX IF NOT EXISTS idx_subtitle_tracks ON subtitle_tracks(media_key, episode)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`PRAGMA user_version = 2`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("migrasi skema v2: %w", err)
		}
	}
	return tx.Commit()
}

// keyOf resolves the effective media key of an entry: an explicit Key
// wins; an anime AnilistID is converted for backward compatibility.
func keyOf(key string, anilistID int) string {
	if key != "" {
		return key
	}
	if anilistID > 0 {
		return AnimeKey(anilistID)
	}
	return ""
}

func (h *HistoryEntry) fillDerived() {
	if id, ok := ParseAnimeKey(h.Key); ok {
		h.AnilistID = id
	}
}

func (f *Favorite) fillDerived() {
	if id, ok := ParseAnimeKey(f.Key); ok {
		f.AnilistID = id
	}
}

// SaveHistory upserts a history entry. Timestamps are unix milliseconds:
// second precision would tie rapid consecutive saves.
func (s *Store) SaveHistory(h HistoryEntry) error {
	h.Key = keyOf(h.Key, h.AnilistID)
	_, err := s.db.Exec(
		`INSERT INTO history (media_key, episode, title, poster, position, duration, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(media_key, episode) DO UPDATE SET
		   title=excluded.title, poster=excluded.poster,
		   position=excluded.position, duration=excluded.duration,
		   updated_at=excluded.updated_at`,
		h.Key, h.Episode, h.Title, h.Poster, h.Position, h.Duration, time.Now().UnixMilli())
	return err
}

// HistoryFor returns the saved entry for one episode, if any.
func (s *Store) HistoryFor(key string, episode int) (*HistoryEntry, error) {
	row := s.db.QueryRow(
		`SELECT media_key, title, poster, episode, position, duration, updated_at
		 FROM history WHERE media_key = ? AND episode = ?`, key, episode)
	var h HistoryEntry
	var ts int64
	if err := row.Scan(&h.Key, &h.Title, &h.Poster, &h.Episode, &h.Position, &h.Duration, &ts); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	h.UpdatedAt = time.UnixMilli(ts)
	h.fillDerived()
	return &h, nil
}

// EpisodeProgress returns the watched percentage (0-100) per episode of
// one title, for episode-picker watched markers.
func (s *Store) EpisodeProgress(key string) (map[int]int, error) {
	rows, err := s.db.Query(`SELECT episode, position, duration FROM history WHERE media_key = ? AND duration > 0`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]int{}
	for rows.Next() {
		var ep int
		var pos, dur float64
		if err := rows.Scan(&ep, &pos, &dur); err != nil {
			return nil, err
		}
		pct := int(pos / dur * 100)
		if pct > 100 {
			pct = 100
		}
		if pct > out[ep] {
			out[ep] = pct
		}
	}
	return out, rows.Err()
}

// ContinueWatching returns the most recent episode per title, newest
// first. Deduplication happens in Go so tied timestamps can never
// surface the same title twice.
func (s *Store) ContinueWatching(limit int) ([]HistoryEntry, error) {
	fetch := limit * 50
	if fetch < 500 {
		fetch = 500
	}
	rows, err := s.db.Query(
		`SELECT media_key, title, poster, episode, position, duration, updated_at
		 FROM history ORDER BY updated_at DESC LIMIT ?`, fetch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all, err := scanHistory(rows)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []HistoryEntry{}
	for _, h := range all {
		if seen[h.Key] {
			continue
		}
		seen[h.Key] = true
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
		`SELECT media_key, title, poster, episode, position, duration, updated_at
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
		if err := rows.Scan(&h.Key, &h.Title, &h.Poster, &h.Episode, &h.Position, &h.Duration, &ts); err != nil {
			return nil, err
		}
		h.UpdatedAt = time.UnixMilli(ts)
		h.fillDerived()
		out = append(out, h)
	}
	return out, rows.Err()
}

// DeleteHistory removes one title (all episodes) from history.
func (s *Store) DeleteHistory(key string) error {
	_, err := s.db.Exec(`DELETE FROM history WHERE media_key = ?`, key)
	return err
}

// ClearHistory removes every history row.
func (s *Store) ClearHistory() error {
	_, err := s.db.Exec(`DELETE FROM history`)
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
	Key       string    `json:"key"`
	AnilistID int       `json:"anilistId"`
	Episode   int       `json:"episode"`
	Label     string    `json:"label"`
	Lang      string    `json:"lang"`
	Source    string    `json:"source"` // upload | external | opensubtitles
	Content   string    `json:"-"`
	CreatedAt time.Time `json:"createdAt"`
}

func (t *SubtitleTrack) fillDerived() {
	if id, ok := ParseAnimeKey(t.Key); ok {
		t.AnilistID = id
	}
}

// SaveSubtitleTrack stores a subtitle track.
func (s *Store) SaveSubtitleTrack(t SubtitleTrack) error {
	t.Key = keyOf(t.Key, t.AnilistID)
	_, err := s.db.Exec(
		`INSERT INTO subtitle_tracks (id, media_key, episode, label, lang, source, content, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Key, t.Episode, t.Label, t.Lang, t.Source, t.Content, time.Now().Unix())
	return err
}

// SubtitleTracks lists saved tracks for an episode (without content).
func (s *Store) SubtitleTracks(key string, episode int) ([]SubtitleTrack, error) {
	rows, err := s.db.Query(
		`SELECT id, media_key, episode, label, lang, source, created_at
		 FROM subtitle_tracks WHERE media_key = ? AND episode = ?
		 ORDER BY created_at ASC`, key, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SubtitleTrack{}
	for rows.Next() {
		var t SubtitleTrack
		var ts int64
		if err := rows.Scan(&t.ID, &t.Key, &t.Episode, &t.Label, &t.Lang, &t.Source, &ts); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(ts, 0)
		t.fillDerived()
		out = append(out, t)
	}
	return out, rows.Err()
}

// SubtitleTrack returns one track including its content, or nil.
func (s *Store) SubtitleTrack(id string) (*SubtitleTrack, error) {
	var t SubtitleTrack
	var ts int64
	err := s.db.QueryRow(
		`SELECT id, media_key, episode, label, lang, source, content, created_at
		 FROM subtitle_tracks WHERE id = ?`, id).
		Scan(&t.ID, &t.Key, &t.Episode, &t.Label, &t.Lang, &t.Source, &t.Content, &ts)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(ts, 0)
	t.fillDerived()
	return &t, nil
}

// DeleteSubtitleTrack removes a saved track.
func (s *Store) DeleteSubtitleTrack(id string) error {
	_, err := s.db.Exec(`DELETE FROM subtitle_tracks WHERE id = ?`, id)
	return err
}

// ToggleFavorite adds the title if absent, removes it if present.
// It reports the state after the toggle (true = now a favorite).
func (s *Store) ToggleFavorite(f Favorite) (bool, error) {
	f.Key = keyOf(f.Key, f.AnilistID)
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE media_key = ?`, f.Key).Scan(&n)
	if err != nil {
		return false, err
	}
	if n > 0 {
		_, err = s.db.Exec(`DELETE FROM favorites WHERE media_key = ?`, f.Key)
		return false, err
	}
	_, err = s.db.Exec(`INSERT INTO favorites (media_key, title, poster, added_at) VALUES (?, ?, ?, ?)`,
		f.Key, f.Title, f.Poster, time.Now().Unix())
	return err == nil, err
}

// IsFavorite reports whether the title is bookmarked.
func (s *Store) IsFavorite(key string) bool {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM favorites WHERE media_key = ?`, key).Scan(&n); err != nil {
		return false
	}
	return n > 0
}

// Favorites returns all favorites, newest first.
func (s *Store) Favorites() ([]Favorite, error) {
	rows, err := s.db.Query(`SELECT media_key, title, poster, added_at FROM favorites ORDER BY added_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Favorite{}
	for rows.Next() {
		var f Favorite
		var ts int64
		if err := rows.Scan(&f.Key, &f.Title, &f.Poster, &ts); err != nil {
			return nil, err
		}
		f.AddedAt = time.Unix(ts, 0)
		f.fillDerived()
		out = append(out, f)
	}
	return out, rows.Err()
}
