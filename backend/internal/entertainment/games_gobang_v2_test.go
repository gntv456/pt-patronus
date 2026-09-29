package entertainment

import "testing"

// 引擎自对弈：应在有限手内分出胜负（自洽性——引擎能终结对局且不崩溃）。
// v5 起 safe 段决策带随机性（MCTS），残局偶发「全盘填满无五」——
// 这是 MCTS 评估噪声下双方互堵的合法平局形态（FREESTYLE 无禁手），
// 224 手后无非法落子即视为通过；成五则提前通过。
func TestGobangV2SelfPlayTerminates(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	board[7][7] = 1 // 黑先手中心
	turn := 2
	for step := 0; step < 224; step++ {
		mine, theirs := turn, 3-turn
		x, y, ok := gobangBestMove(board, mine, theirs)
		if !ok {
			t.Fatalf("no move at step %d", step)
		}
		if board[y][x] != 0 {
			t.Fatalf("illegal move (%d,%d) at step %d", x, y, step)
		}
		board[y][x] = mine
		// 落子后检查连五
		if gobangHasFiveAt(board, x, y, mine) {
			return // 正常终结
		}
		turn = theirs
	}
	// 满盘无五 = 平局（可接受）；只有非法/无子才是失败（上面已断言）
}

// 战术层：己方四连一端空 → 直接补五。
func TestGobangV2TakesWinningMove(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	for x := 3; x <= 6; x++ {
		board[7][x] = 1 // 黑横向四连，两端(2,7)(8,7)空
	}
	board[5][5] = 2
	x, y, ok := gobangBestMove(board, 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	if y != 7 || (x != 2 && x != 8) {
		t.Fatalf("should complete five, got (%d,%d)", x, y)
	}
}

// 战术层：对方四连一端空 → 必堵（另一端被己方封死时只有单点可堵）。
func TestGobangV2BlocksOpponentFive(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	for x := 4; x <= 7; x++ {
		board[9][x] = 2 // 白横向四连
	}
	board[9][8] = 1 // 右端被黑封
	board[5][5] = 1
	x, y, ok := gobangBestMove(board, 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	if x != 3 || y != 9 {
		t.Fatalf("should block at (3,9), got (%d,%d)", x, y)
	}
}

// gobangHasFiveAt (x,y) 落 side 后是否存在 ≥5 连（FREESTYLE）。
func gobangHasFiveAt(board [][]int, x, y, side int) bool {
	size := len(board)
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		cnt := 1
		for _, sgn := range [2]int{1, -1} {
			nx, ny := x+d[0]*sgn, y+d[1]*sgn
			for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) && board[ny][nx] == side {
				cnt++
				nx += d[0] * sgn
				ny += d[1] * sgn
			}
		}
		if cnt >= 5 {
			return true
		}
	}
	return false
}

// 防活四规则：对方两端皆空的活三，我方必须抢占其成四延伸点
// （HARD 实战负例回归：白放任黑活三，黑一手成活四后白堵端无效）。
func TestGobangV2BlocksOpenThreeExtension(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑横向活三 (8,7)(9,7)(10,7)，两端 (7,7)(11,7) 空
	board[7][8], board[7][9], board[7][10] = 2, 2, 2
	board[5][5] = 1
	board[5][6] = 1
	x, y, ok := gobangBestMove(board, 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	// 白没有自身四连威胁，必须堵黑活三的两个延伸点之一
	if y != 7 || (x != 7 && x != 11) {
		t.Fatalf("should block open-three extension at (7,7) or (11,7), got (%d,%d)", x, y)
	}
}
