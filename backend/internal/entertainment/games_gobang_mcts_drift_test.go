package entertainment

import (
	"testing"
	"time"
)

// 漂移复现：同一局面多预算/多种子读数（手动 -run ProbeDrift）。
func TestProbeMCTSDrift(t *testing.T) {
	board := board505ePly26()
	for _, budget := range []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second, 8 * time.Second} {
		for rep := 0; rep < 3; rep++ {
			res := gobangMCTS(board, 2, 1, budget, nil)
			t.Logf("budget=%v rep=%d → (%d,%d) wr=%.3f sims=%d",
				budget, rep, res.X, res.Y, res.WinRate, res.Simulations)
		}
	}
}

// 漂移定位：dump 根子 top6 的访问数/胜率/RAVE（6s 预算，一次）。
func TestProbeMCTSRootStats(t *testing.T) {
	board := board505ePly26()
	res := mctsRootStats(board, 2, 1, 6*time.Second)
	for _, s := range res {
		t.Logf("(%2d,%2d) visits=%6d q=%.3f rave=%d/%.3f avgLen=%.1f",
			s.move[0], s.move[1], s.visits, s.q, s.raveN, s.raveQ, s.avgLen)
	}
}
