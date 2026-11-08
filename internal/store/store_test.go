package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestHistoryRoundtripAndContinue(t *testing.T) {
	s := openTestStore(t)
	if err := s.SaveHistory(HistoryEntry{AnilistID: 20, Title: "Naruto", Episode: 1, Position: 100, Duration: 1400}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := s.SaveHistory(HistoryEntry{AnilistID: 20, Title: "Naruto", Episode: 2, Position: 50, Duration: 1400}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := s.SaveHistory(HistoryEntry{AnilistID: 21, Title: "One Piece", Episode: 5, Position: 900, Duration: 1400}); err != nil {
		t.Fatal(err)
	}
	h, err := s.HistoryFor(AnimeKey(20), 1)
	if err != nil || h == nil {
		t.Fatalf("history ep1: %v / %v", h, err)
	}
	if h.Position != 100 {
		t.Errorf("posisi salah: %v", h.Position)
	}
	cw, err := s.ContinueWatching(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cw) != 2 {
		t.Fatalf("continue watching harus 2 anime, dapat %d", len(cw))
	}
	// Newest update overall is One Piece ep 5.
	if cw[0].AnilistID != 21 {
		t.Errorf("yang terbaru harusnya One Piece, dapat %d", cw[0].AnilistID)
	}
}

func TestFavoritesToggle(t *testing.T) {
	s := openTestStore(t)
	f := Favorite{AnilistID: 99, Title: "Frieren"}
	on, err := s.ToggleFavorite(f)
	if err != nil || !on {
		t.Fatalf("toggle pertama: on=%v err=%v", on, err)
	}
	if !s.IsFavorite(AnimeKey(99)) {
		t.Error("harus tercatat favorit")
	}
	on, err = s.ToggleFavorite(f)
	if err != nil || on {
		t.Fatalf("toggle kedua: on=%v err=%v", on, err)
	}
	if s.IsFavorite(AnimeKey(99)) {
		t.Error("harusnya sudah tidak favorit")
	}
}

func TestSubtitleTracksRoundtrip(t *testing.T) {
	s := openTestStore(t)
	tr := SubtitleTrack{ID: "sub_test1", AnilistID: 20, Episode: 3, Label: "uji.srt", Lang: "id", Source: "upload", Content: "1\n00:00:01,000 --> 00:00:02,000\nHalo\n"}
	if err := s.SaveSubtitleTrack(tr); err != nil {
		t.Fatal(err)
	}
	list, err := s.SubtitleTracks(AnimeKey(20), 3)
	if err != nil || len(list) != 1 {
		t.Fatalf("daftar track: %v / %d", err, len(list))
	}
	if list[0].Content != "" {
		t.Error("daftar tidak boleh membawa isi subtitle")
	}
	got, err := s.SubtitleTrack("sub_test1")
	if err != nil || got == nil {
		t.Fatalf("ambil track: %v / %v", got, err)
	}
	if got.Content != tr.Content || got.Label != "uji.srt" {
		t.Errorf("isi track salah: %+v", got)
	}
	if missing, _ := s.SubtitleTrack("sub_tidakada"); missing != nil {
		t.Error("track tak dikenal harusnya nil")
	}
	if err := s.DeleteSubtitleTrack("sub_test1"); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.SubtitleTracks(AnimeKey(20), 3); len(list) != 0 {
		t.Error("track harusnya terhapus")
	}
}

func TestSettingsRoundtrip(t *testing.T) {
	s := openTestStore(t)
	if got := s.GetSetting("otakudesu_base"); got != "" {
		t.Errorf("setting kosong harusnya \"\", dapat %q", got)
	}
	if err := s.SetSetting("otakudesu_base", "https://contoh.example"); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSetting("otakudesu_base"); got != "https://contoh.example" {
		t.Errorf("setting salah: %q", got)
	}
	if err := s.SetSetting("otakudesu_base", "https://baru.example"); err != nil {
		t.Fatal(err)
	}
	if got := s.GetSetting("otakudesu_base"); got != "https://baru.example" {
		t.Errorf("setting setelah update salah: %q", got)
	}
}

// TestMigrationV1ToV2 builds a legacy (v1) database exactly as the old
// versions wrote it, then checks Open migrates it in place: rows keep
// their data under anime: media keys and the schema is stamped v2.
func TestMigrationV1ToV2(t *testing.T) {
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "naniplay.db")
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	legacy := []string{
		`CREATE TABLE history (anilist_id INTEGER NOT NULL, episode INTEGER NOT NULL, title TEXT NOT NULL DEFAULT '', poster TEXT NOT NULL DEFAULT '', position REAL NOT NULL DEFAULT 0, duration REAL NOT NULL DEFAULT 0, updated_at INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (anilist_id, episode))`,
		`CREATE TABLE favorites (anilist_id INTEGER PRIMARY KEY, title TEXT NOT NULL DEFAULT '', poster TEXT NOT NULL DEFAULT '', added_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE subtitle_tracks (id TEXT PRIMARY KEY, anilist_id INTEGER NOT NULL, episode INTEGER NOT NULL, label TEXT NOT NULL, lang TEXT NOT NULL DEFAULT '', source TEXT NOT NULL, content TEXT NOT NULL, created_at INTEGER NOT NULL)`,
		`INSERT INTO history VALUES (154587, 3, 'Frieren', 'p.jpg', 500, 1420, 1730000000000)`,
		`INSERT INTO favorites VALUES (154587, 'Frieren', 'p.jpg', 1730000000)`,
		`INSERT INTO subtitle_tracks VALUES ('sub_x', 154587, 3, 'a.srt', 'id', 'upload', 'isi', 1730000000)`,
		`INSERT INTO settings VALUES ('llm_model', 'metaai/muse-spark-1.3-contributor')`,
	}
	for _, q := range legacy {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("buat skema lama: %v", err)
		}
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open + migrasi: %v", err)
	}
	defer s.Close()

	h, err := s.HistoryFor(AnimeKey(154587), 3)
	if err != nil || h == nil {
		t.Fatalf("riwayat hasil migrasi: %v / %v", h, err)
	}
	if h.Position != 500 || h.Title != "Frieren" || h.AnilistID != 154587 {
		t.Errorf("riwayat berubah: %+v", h)
	}
	if !s.IsFavorite(AnimeKey(154587)) {
		t.Error("favorit hasil migrasi hilang")
	}
	tracks, err := s.SubtitleTracks(AnimeKey(154587), 3)
	if err != nil || len(tracks) != 1 || tracks[0].Label != "a.srt" {
		t.Errorf("subtitle hasil migrasi: %v / %+v", err, tracks)
	}
	if got := s.GetSetting("llm_model"); got != "metaai/muse-spark-1.3-contributor" {
		t.Errorf("setting hilang saat migrasi: %q", got)
	}
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Errorf("user_version harusnya 2, dapat %d (%v)", version, err)
	}
}
