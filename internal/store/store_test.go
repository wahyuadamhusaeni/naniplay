package store

import (
	"testing"
	"time"
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
	h, err := s.HistoryFor(20, 1)
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
	if !s.IsFavorite(99) {
		t.Error("harus tercatat favorit")
	}
	on, err = s.ToggleFavorite(f)
	if err != nil || on {
		t.Fatalf("toggle kedua: on=%v err=%v", on, err)
	}
	if s.IsFavorite(99) {
		t.Error("harusnya sudah tidak favorit")
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
