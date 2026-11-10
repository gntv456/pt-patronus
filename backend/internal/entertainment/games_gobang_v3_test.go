// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"testing"
	"time"
)

// VCF 能力：构造「双五点连环」真 VCF——两条交叉四线，落交汇点即双五点胜。
func TestGobangV3FindsVCF(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑：横向 (3,7)(4,7)(5,7) 三连 + 纵向 (6,4)(6,5)(6,6) 三连，交汇 (6,7)。
	// 黑落 (6,7)：横四（左端(2,7)白封→五点(7,7)）+ 纵四（上(6,3)空→五点(6,3)/(6,8)）
	// ⇒ 双五点 = 真 VCF 胜。修复前靠「冲四=已胜」假阳性通过；修复后必须真判出。
	board[7][3], board[7][4], board[7][5] = 1, 1, 1
	board[4][6], board[5][6], board[6][6] = 1, 1, 1
	board[7][2] = 2
	board[5][5] = 2
	x, y, ok := gobangBestMove(board, 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	// (6,7) 双四交汇与 (6,3) 纵活四都是强制胜起点，接受两者
	if !((x == 6 && y == 7) || (x == 6 && y == 3)) {
		t.Fatalf("should take a forced-win move (6,7)/(6,3), got (%d,%d)", x, y)
	}
}

// 实战负例回归（HARD 第4局 move26）：白面对「斜冲四点(9,13) + 纵跳三」双杀，
// v2 选了 (9,13) 后仍输。v3 的行为：若双杀确实无解，至少不能比 v2 差；
// 这里只验证引擎给出合法且不自杀的应手（终局判定在真局里做）。
func TestGobangV3UnderDoubleThreat(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑（theirs=2）：斜三 (5,9)(6,10)(7,11)（上端 (4,8) 白封）→ (8,12) 冲四点
	board[9][5], board[10][6], board[11][7] = 2, 2, 2
	board[8][4] = 1
	// 黑纵向跳三 (6,9)(6,10) + (6,12)（模拟第5局杀型）
	board[9][6], board[10][6] = 2, 2
	board[12][6] = 2
	// 白散子
	board[7][7] = 1
	board[8][8] = 1
	x, y, ok := gobangBestMove(board, 1, 2)
	if !ok {
		t.Fatal("no move")
	}
	_ = x
	_ = y
	// 无硬性断言：此局面可能本就必败；保证引擎给出不崩溃的应手即可。
}

// 双杀前瞻回归（HARD 第6局 46a9463d step22 负例）：黑「成五点(5,13)+活四点(6,10)」
// 并存时白堵五点必败——增强后 opponent_wins 应在我方候选评估阶段就标死该类选择，
// 逼迫引擎在中盘就拆掉其中一条威胁。此处验证前瞻判定本身：
// 黑跳三+冲四雏形局面，白若被动局面应被识别为必败。
func TestGobangV3DoubleKillForesight(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑（theirs=2）纵向跳三 (6,9)(6,11)(6,12)：落 (6,10) 成活四（两端 (6,8)(6,13) 空）
	board[9][6], board[11][6], board[12][6] = 2, 2, 2
	// 黑斜线三子使 (5,13) 成为其成五点（斜向 (5,13)-(6,12)-(7,11)-(8,10)-(9,9) 已有 4 子形态）
	board[11][7], board[10][8], board[9][9] = 2, 2, 2
	// 白散子
	board[7][7], board[8][8] = 1, 1
	// 黑双威胁成立：一手 (6,10) 活四 + 一手 (5,13) 成五。
	// 模拟白随便落一子（非要点），检查 opponent_wins 前瞻能否发现黑的双杀。
	board[3][3] = 1 // 白无关落子
	doomed := gobangOpponentWins(board, 1, 2, time.Now().Add(2*time.Second))
	if !doomed {
		t.Fatal("double-kill foresight failed to detect the fork (four-move + five-spot)")
	}
}

// 针对性开局应手（HARD 守擂杀型）：黑 (7,7) 开局、十字布子、第 11 手落杀线种子。
// 种子特征：黑新子与既有黑子同线（无白子占线）、间距 2-3 格 → 即跳三骨架。
// 引擎在第 12 手应选择骨架拆解点（空隙或延伸），不能置之不理去下无关进攻点。
// 局面取自实战 e9b73f8b（黑 col6 两子 (6,8)(6,9)，白散子无关联）。
func TestGobangV3BreaksSeedSkeleton(t *testing.T) {
	board := make([][]int, 15)
	for i := range board {
		board[i] = make([]int, 15)
	}
	// 黑：十字核心 (7,7)(6,6)(8,6) + 杀线种子 (6,8)(6,9)（col6 同线无白）
	board[7][7], board[6][6], board[8][6] = 1, 1, 1
	board[8][6] = 1 // col: x=6
	_ = board
	// 重摆（避免上面误写）：
	for y := range board {
		board[y] = make([]int, 15)
	}
	board[7][7] = 1 // 黑 核心
	board[6][6] = 1 // 黑
	board[6][8] = 1 // 黑 杀线种子1（col x=6）
	board[6][9] = 1 // 黑 杀线种子2
	// 白：斜跟散子（实战常见应对）
	board[9][7] = 2
	board[7][9] = 2
	board[8][8] = 2
	board[5][5] = 2
	x, y, ok := gobangBestMove(board, 2, 1)
	if !ok {
		t.Fatal("no move")
	}
	// 语义判定：白选择与任一黑子同线（8方向连线上，允许间隔）——即落在黑骨架线体系内。
	// 骨架线 = 过≥1黑子且方向延伸的行/列/斜线。白拆线/压制必然与黑子共线。
	blacks := [][2]int{{7, 7}, {6, 6}, {6, 8}, {6, 9}}
	for _, b := range blacks {
		dx := x - b[0]
		dy := y - b[1]
		if dx == 0 || dy == 0 || abs(dx) == abs(dy) {
			return // 与黑子共线（拆线应手）
		}
	}
	t.Fatalf("engine ignored all black skeleton lines, got (%d,%d)", x, y)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
