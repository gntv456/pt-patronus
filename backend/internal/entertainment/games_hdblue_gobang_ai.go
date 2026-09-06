package entertainment

// gobangBestMove 贪心五子棋选点：对每个空位按「我方连五潜力 + 封堵对方连五潜力」打分取最优。
// board[y][x]：0 空 / mine 己方 / theirs 对方（调用方传 1=玩家 2=AI）。15×15 无禁手。
// 纯本地决策（不上传任何东西），只用于 Play 自动弈完一局。
func gobangBestMove(board [][]int, mine, theirs int) (int, int, bool) {
	size := len(board)
	if size == 0 {
		return 0, 0, false
	}
	bestX, bestY, bestScore := -1, -1, -1
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
			s := gobangScorePoint(board, x, y, mine)*10 + gobangScorePoint(board, x, y, theirs)
			// 中心微加成（开局更快形成威胁）
			s += 15 - maxAbs(x-size/2, y-size/2)
			if s > bestScore {
				bestScore, bestX, bestY = s, x, y
			}
		}
	}
	if bestX < 0 {
		return 0, 0, false
	}
	return bestX, bestY, true
}

// gobangScorePoint 在 (x,y) 假设落 side 子后，四个方向的连子形态打分。
// 五连=100000、活四=10000、冲四=5000、活三=1000、眠三=100、活二=100、眠二=10。
func gobangScorePoint(board [][]int, x, y, side int) int {
	size := len(board)
	total := 0
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		cnt, openEnds := 1, 0
		for _, sgn := range [2]int{1, -1} {
			nx, ny := x+d[0]*sgn, y+d[1]*sgn
			for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) && board[ny][nx] == side {
				cnt++
				nx += d[0] * sgn
				ny += d[1] * sgn
			}
			if nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) && board[ny][nx] == 0 {
				openEnds++
			}
		}
		total += gobangLineScore(cnt, openEnds)
	}
	return total
}

func gobangLineScore(cnt, openEnds int) int {
	switch {
	case cnt >= 5:
		return 100000
	case cnt == 4 && openEnds == 2:
		return 10000
	case cnt == 4 && openEnds == 1:
		return 5000
	case cnt == 3 && openEnds == 2:
		return 1000
	case cnt == 3 && openEnds == 1:
		return 100
	case cnt == 2 && openEnds == 2:
		return 100
	case cnt == 2 && openEnds == 1:
		return 10
	default:
		return openEnds // 单子贴边价值极低
	}
}

func maxAbs(a, b int) int {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	if a > b {
		return a
	}
	return b
}
