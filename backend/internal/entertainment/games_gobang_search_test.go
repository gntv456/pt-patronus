package entertainment

import (
	"testing"
	"time"
)

// 全局搜索：一步取胜必须找到。
func TestGobangSearchFindsImmediateWin(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	for x := 3; x <= 6; x++ {
		board[7][x] = 1
	}
	board[5][5] = 2
	res := gobangSearch(board, 1, 2, 2*time.Second)
	if !res.Win {
		t.Fatalf("should detect forced win, got score=%d depth=%d pick=(%d,%d)", res.Score, res.Depth, res.X, res.Y)
	}
	if !(res.Y == 7 && (res.X == 2 || res.X == 7)) {
		t.Fatalf("should complete five, got (%d,%d)", res.X, res.Y)
	}
}

// 全局搜索：对方活三（跳三）成杀前，浅层搜索应给出拆解着（不指定点位，验证不崩+合理分）。
func TestGobangSearchDefendsFork(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑跳三骨架：(8,10)(9,10) 连续 + (11,10) 跳，缺口 (10,10)
	board[10][8], board[10][9], board[10][11] = 1, 1, 1
	board[5][5], board[5][6] = 2, 2
	res := gobangSearch(board, 2, 1, 2*time.Second)
	if res.X < 0 {
		t.Fatal("no move")
	}
	// 合理性：不应落在远离战场的角落（距任一黑子曼哈顿 <= 6）
	blacks := [][2]int{{8, 10}, {9, 10}, {11, 10}}
	near := false
	for _, b := range blacks {
		if abs(res.X-b[0])+abs(res.Y-b[1]) <= 6 {
			near = true
		}
	}
	if !near {
		t.Fatalf("search moved far from the battlefield: (%d,%d)", res.X, res.Y)
	}
}

// 搜索预算：50ms 也要在时限内返回（可用性）。
func TestGobangSearchRespectsBudget(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	board[7][7] = 1
	board[7][8] = 2
	board[8][8] = 1
	t0 := time.Now()
	res := gobangSearch(board, 2, 1, 50*time.Millisecond)
	el := time.Since(t0)
	if el > 3*time.Second {
		t.Fatalf("search took %v with 50ms budget", el)
	}
	if res.X < 0 {
		t.Fatal("no move returned")
	}
}
