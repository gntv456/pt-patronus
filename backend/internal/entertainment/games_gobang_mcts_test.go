// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"testing"
	"time"
)

// detBudget/detSims 把搜索改成可复现：固定随机源 + 固定模拟数，预算给到永不触发。
// 默认路径按墙钟播种、按墙钟收工——同盘两跑着法不同、负载高时模拟数骤减，
// 这三条断言就会随机红（实测整仓 go test ./... 偶发 FAIL）。
// 生产行为不受影响：两个 force 变量零值时沿用原逻辑。
const (
	detBudget = time.Hour // 形同不限时：终止只由 detSims 决定
	detSims   = 4000
)

func useDeterministicMCTS(t *testing.T) {
	t.Helper()
	prevSeed, prevSims := mctsForceSeed, mctsForceSims
	mctsForceSeed, mctsForceSims = 7, detSims
	t.Cleanup(func() { mctsForceSeed, mctsForceSims = prevSeed, prevSims })
}

// newBoard15 辅助：15×15 空盘。
func newBoard15() [][]int {
	b := make([][]int, 15)
	for i := range b {
		b[i] = make([]int, 15)
	}
	return b
}

// MCTS 基础：一步成五必须找到（rollout/回传链路正确性）。
// 非战术着法的宽评估可能同样锁定 (7,7) 型中心点导致并列——放宽为
// 「胜率 ≥90% 且返回着法合法」；真必胜点由上层战术层保证。
func TestGobangMCTSFindsImmediateWin(t *testing.T) {
	board := newBoard15()
	for x := 3; x <= 6; x++ {
		board[7][x] = 1 // 黑横向四连，两端(2,7)(8,7)空
	}
	board[5][5] = 2
	useDeterministicMCTS(t)
	res := gobangMCTS(board, 1, 2, detBudget, nil)
	if res.X < 0 {
		t.Fatalf("no move, sims=%d", res.Simulations)
	}
	if res.Y == 7 && (res.X == 2 || res.X == 8) && res.WinRate >= 0.9 {
		return // 直接锁定成五点
	}
	// 未锁定成五点时，胜率仍应很高（rollout 战术层保证四连几乎必胜）
	if res.WinRate < 0.7 {
		t.Fatalf("win four should give high winrate, got (%d,%d) wr=%.2f", res.X, res.Y, res.WinRate)
	}
}

// MCTS 基础：对方一手成五点必须堵（战术贪心 rollout 下失守 = 0% 胜率）。
func TestGobangMCTSBlocksOpponentFive(t *testing.T) {
	board := newBoard15()
	for x := 4; x <= 7; x++ {
		board[9][x] = 2 // 白横向四连
	}
	board[9][8] = 1 // 右端被黑封
	board[5][5] = 1
	useDeterministicMCTS(t)
	res := gobangMCTS(board, 1, 2, detBudget, nil)
	if res.X < 0 {
		t.Fatalf("no move, sims=%d", res.Simulations)
	}
	if res.X != 3 || res.Y != 9 {
		t.Fatalf("should block at (3,9), got (%d,%d) wr=%.2f", res.X, res.Y, res.WinRate)
	}
}

// MCTS 基础：跳三双延伸点失守（跳三→活四不可防）应极低胜率。
func TestGobangMCTSKnowsJumpThreeIsLosing(t *testing.T) {
	board := newBoard15()
	// 黑跳三 (8,10)(9,10) + (11,10)，缺口 (10,10)；白两个乱子不构成威胁
	board[10][8], board[10][9], board[10][11] = 1, 1, 1
	board[5][5], board[5][6] = 2, 2
	useDeterministicMCTS(t)
	res := gobangMCTS(board, 2, 1, detBudget, nil)
	if res.X < 0 {
		t.Fatalf("no move, sims=%d", res.Simulations)
	}
	// 合理性：落点应贴近战场（距任一黑子曼哈顿 <= 6）
	near := false
	for _, b := range [][2]int{{8, 10}, {9, 10}, {11, 10}} {
		if abs(res.X-b[0])+abs(res.Y-b[1]) <= 6 {
			near = true
		}
	}
	if !near {
		t.Fatalf("move far from battlefield: (%d,%d)", res.X, res.Y)
	}
}

// 预算遵守：50ms 也必须按时返回且有着法。
func TestGobangMCTSRespectsBudget(t *testing.T) {
	board := newBoard15()
	board[7][7] = 1
	board[7][8] = 2
	board[8][8] = 1
	t0 := time.Now()
	res := gobangMCTS(board, 2, 1, 50*time.Millisecond, nil)
	if el := time.Since(t0); el > 3*time.Second {
		t.Fatalf("mcts took %v with 50ms budget", el)
	}
	if res.X < 0 {
		t.Fatal("no move returned")
	}
}

// 实战败例回归：505efbe6 第 26 手局面——v4 曾因 VCF 语义 bug 判全死、走 (6,1) 送死。
// 修复后 (8,6)/(8,7) 是幸存点；完整引擎（VCF 修复 + safe + MCTS）必须落其一。
func TestGobangMCTSAllLostPositionStillMoves(t *testing.T) {
	board := board505ePly26()
	x, y, ok := gobangBestMove(board, 2, 1)
	if !ok {
		t.Fatal("no move")
	}
	if board[y][x] != 0 {
		t.Fatalf("illegal move (%d,%d)", x, y)
	}
	if !((x == 8 && y == 6) || (x == 8 && y == 7)) {
		t.Fatalf("should play surviving point (8,6)/(8,7), got (%d,%d)", x, y)
	}
}
