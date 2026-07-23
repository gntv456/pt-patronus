package keepseed

import "testing"

func TestScore(t *testing.T) {
	// 完全相同标题 + 相同大小 → 高分
	s := Score("The.Movie.2023.1080p", "The Movie 2023 1080p", 1000, 1000)
	if s < 90 {
		t.Errorf("identical title/size score = %d, want >=90", s)
	}
	// 完全不同
	s2 := Score("Alpha", "Beta Gamma", 1000, 999999)
	if s2 > 40 {
		t.Errorf("different title score = %d, want low", s2)
	}
}

func TestScoreExactSizeBoost(t *testing.T) {
	// 标题中等相似但体积精确 → 至少 92
	s := ScoreWith("Some.Show.S01E01.1080p", "Some Show S01E01 BluRay", 1234567890, 1234567890, IDHints{}, "", "")
	if s < 92 {
		t.Fatalf("exact size boost = %d, want >=92", s)
	}
}

func TestScoreImdbBoost(t *testing.T) {
	s := ScoreWith("Renamed.File.mkv", "Totally.Different.Name", 100, 200, IDHints{ImdbID: "tt1234567"}, "tt1234567", "")
	if s < 95 {
		t.Fatalf("imdb boost = %d, want >=95", s)
	}
}

func TestExtractTmdbID(t *testing.T) {
	if got := ExtractTmdbID("Movie.tmdb-550.1080p"); got != "550" {
		t.Fatalf("got %q", got)
	}
	if got := ExtractTmdbID("no id"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestScoreFileTree(t *testing.T) {
	s := ScoreFileTree(
		[]string{"Movie/Movie.mkv", "Movie/sample.mkv"},
		[]string{"Movie.mkv", "sample.mkv"},
	)
	if s < 50 {
		t.Fatalf("file tree score = %d", s)
	}
}

func TestNormalizeTmdbID(t *testing.T) {
	if got := NormalizeTmdbID("movie:12345"); got != "12345" {
		t.Fatalf("got %q", got)
	}
}
