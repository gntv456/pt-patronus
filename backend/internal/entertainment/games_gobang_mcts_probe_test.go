// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"fmt"
	"testing"
	"time"
)

// 505efbe6 局 ply-26 白方局面（HARD 实战，v4 判 89 候选全死）。
func board505ePly26() [][]int {
	b := newBoard15()
	// 黑（对手，系统执黑）
	for _, p := range [][2]int{{7, 7}, {6, 8}, {6, 6}, {5, 6}, {6, 5}, {8, 5}, {5, 5}, {7, 3}, {4, 4}, {5, 3}, {8, 3}, {8, 4}, {9, 5}} {
		b[p[1]][p[0]] = 1
	}
	// 白（我方）
	for _, p := range [][2]int{{5, 7}, {5, 9}, {8, 8}, {7, 6}, {6, 7}, {7, 4}, {7, 5}, {6, 4}, {3, 3}, {3, 5}, {6, 3}, {8, 2}, {6, 2}} {
		b[p[1]][p[0]] = 2
	}
	return b
}

// 探针：MCTS 在必败局面的读数与选点（-run ProbeMCTS 手动跑）。
func TestProbeMCTSDoomedPosition(t *testing.T) {
	board := board505ePly26()
	for _, budget := range []time.Duration{2 * time.Second, 8 * time.Second} {
		res := gobangMCTS(board, 2, 1, budget, nil)
		t.Logf("budget=%v → (%d,%d) wr=%.3f sims=%d elapsed=%v",
			budget, res.X, res.Y, res.WinRate, res.Simulations, res.Elapsed)
		if res.X < 0 || board[res.Y][res.X] != 0 {
			t.Fatalf("illegal move (%d,%d)", res.X, res.Y)
		}
	}
	// 对照：更早的手（ply20/22/24 白方视角），看败势滑落曲线与 MCTS 选点
	mv := [][2]int{{7, 7}, {5, 7}, {6, 8}, {5, 9}, {6, 6}, {8, 8}, {5, 6}, {7, 6}, {6, 5}, {6, 7}, {8, 5}, {7, 4}, {5, 5}, {7, 5}, {7, 3}, {6, 4}, {4, 4}, {3, 3}, {5, 3}, {3, 5}, {8, 3}, {6, 3}, {8, 4}, {8, 2}, {9, 5}, {6, 2}}
	for _, n := range []int{20, 22, 24} {
		b := newBoard15()
		for i := 0; i < n; i++ {
			side := 1
			if i%2 == 1 {
				side = 2
			}
			b[mv[i][1]][mv[i][0]] = side
		}
		res := gobangMCTS(b, 2, 1, 3*time.Second, nil)
		x4, y4, ok4 := gobangBestMove(b, 2, 1)
		t.Logf("ply%d: MCTS=(%d,%d) wr=%.3f sims=%d | v4=(%d,%d ok=%v)", n, res.X, res.Y, res.WinRate, res.Simulations, x4, y4, ok4)
	}
	_ = fmt.Sprint()
}
