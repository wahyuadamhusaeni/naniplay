package otakudesu

import "testing"

func TestScoreTitleExactBeatsPartial(t *testing.T) {
	exact := ScoreTitle("Naruto", "Naruto")
	partial := ScoreTitle("Naruto", "Naruto Shippuden")
	if exact < 0.99 {
		t.Errorf("judul identik harusnya ~1.0, dapat %.2f", exact)
	}
	if partial >= exact {
		t.Errorf("judul parsial (%.2f) tidak boleh mengalahkan identik (%.2f)", partial, exact)
	}
}

func TestScoreTitleUnrelatedIsZero(t *testing.T) {
	if s := ScoreTitle("Naruto", "One Piece"); s != 0 {
		t.Errorf("judul tak berkaitan harusnya 0, dapat %.2f", s)
	}
}

func TestScoreTitleSeasonMismatchPenalized(t *testing.T) {
	right := ScoreTitle("Mushoku Tensei Season 2", "Mushoku Tensei Season 2")
	wrong := ScoreTitle("Mushoku Tensei Season 2", "Mushoku Tensei Season 1")
	if wrong >= right {
		t.Errorf("musim salah (%.2f) harus kalah dari musim benar (%.2f)", wrong, right)
	}
	if wrong > 0.55 {
		t.Errorf("musim salah harus terpenalti di bawah ambang, dapat %.2f", wrong)
	}
}

func TestTitlesMatchThreshold(t *testing.T) {
	if !TitlesMatch("Sousou no Frieren", "Sousou no Frieren") {
		t.Error("judul sama harus cocok")
	}
	if TitlesMatch("Frieren", "Naruto") {
		t.Error("judul berbeda tidak boleh cocok")
	}
}
