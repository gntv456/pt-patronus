package keepseed

import "testing"

func TestScore(t *testing.T) {
	// 完全相同标题 + 相同大小 → 高分
	s := Score("The.Movie.2023.1080p", "The Movie 2023 1080p", 1000, 1000)
	if s < 90 {
		t.Errorf("identical title/size score = %d, want >=90", s)
	}
	// 标题相近，大小悬殊 → 中低分
	s = Score("The Movie 2023", "The Movie 2023 1080p", 1000, 10)
	if s > 80 {
		t.Errorf("size mismatch score = %d, should be lower", s)
	}
	// 完全不同标题 → 低分
	s = Score("Alpha Beta", "Gamma Delta", 1000, 1000)
	if s > 30 {
		t.Errorf("unrelated score = %d", s)
	}
	// 空标题 → 0
	if Score("", "x", 1, 1) != 0 {
		t.Error("empty want should be 0")
	}
}
