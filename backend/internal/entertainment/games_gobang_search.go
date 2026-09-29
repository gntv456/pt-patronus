package entertainment

import (
	"math"
	"os"
	"strconv"
	"time"
)

// ── v4 全局搜索引擎 ──────────────────────────────────────────────────────
//
// 背景：v3 战术层（必堵/防活四/VCF/交汇点）在 HARD 实战验证为「零战术失误仍连败」——
// 败因在布局：黑用多线骨架在中盘完成包围，白在威胁可见前一手已无解。
// v4 在保留全部战术层的前提下，把核心决策换成全局搜索：
//
//	gobangSearch(board, mine, theirs, budget) → 迭代加深 Alpha-Beta
//
// 设计要点：
//   - 评估函数：5 窗口滑动模式计分（活四/冲四/活三/眠三/活二…）双方差值，
//     外加「骨架位加成」——与 v3 point_eval 同语义但作用于全盘而非单点。
//   - 候选生成：落子 2 格邻域 + 战术点（五点/活四点/骨架点）强制并入，按启发分排序截断。
//   - 剪枝：Alpha-Beta + 迭代加深（用上一层结果排序下一层）+ 简单置换表。
//   - 杀手启发：每层记录引发 beta 截断的着法，同层优先试。
//   - 时间预算：默认 3s（对局节奏允许；站点每手 10s 时钟）。
//
// 纯本地计算，无网络请求。

const (
	gobangSearchNodesLimit = 400000 // 硬节点上限，防长局超时
)

// gobangSearchBudget 搜索时间预算：默认 4s（对局节奏），env 可覆盖（测试加速）。
func gobangSearchBudget() time.Duration {
	if v := os.Getenv("GOBANG_SEARCH_BUDGET_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 4 * time.Second
}

// gobangSearchResult 全局搜索产物。
type gobangSearchResult struct {
	X, Y   int
	Score  int64    // 我方视角
	Depth  int      // 完成的完整迭代层深
	Nodes  int      // 搜索节点数
	Win    bool     // 找到强制胜（评分 >= winScore）
	Cutoff bool     // 被时间/节点截断
	PV     [][2]int // 主变化线（浅层）
}

type gobangSearchCtx struct {
	board    [][]int
	mine     int
	theirs   int
	size     int
	deadline time.Time
	nodes    int
	cutoff   bool
	tt       map[uint64]gobangTTEntry
	killers  [][2]int
	pv       [][2]int
}

type gobangTTEntry struct {
	depth int
	score int64
	flag  int // 0 exact / 1 lower / 2 upper
}

// gobangZobrist 简式 Zobrist：坐标×颜色哈希（FNV 基数展开）。
var gobangZobrist [15][15][2]uint64

func init() {
	var s uint64 = 14695981039346656037
	next := func() uint64 {
		s ^= s << 13
		s ^= s >> 7
		s ^= s << 17
		return s
	}
	for y := 0; y < 15; y++ {
		for x := 0; x < 15; x++ {
			for c := 0; c < 2; c++ {
				gobangZobrist[y][x][c] = next()
			}
		}
	}
}

func gobangHash(ctx *gobangSearchCtx) uint64 {
	var h uint64 = 1469598103934665603
	for y := 0; y < ctx.size; y++ {
		for x := 0; x < len(ctx.board[y]); x++ {
			v := ctx.board[y][x]
			if v == 1 || v == 2 {
				h ^= gobangZobrist[y][x][v-1]
				h *= 1099511628211
			}
		}
	}
	return h
}

// gobangSearch 迭代加深 Alpha-Beta 主入口。budget 为总时间预算。
func gobangSearch(board [][]int, mine, theirs int, budget time.Duration) gobangSearchResult {
	size := len(board)
	ctx := &gobangSearchCtx{
		board: board, mine: mine, theirs: theirs, size: size,
		deadline: time.Now().Add(budget),
		tt:       nil, // A/B: disabled
	}
	res := gobangSearchResult{X: -1, Y: -1, Score: math.MinInt64 / 4}
	if size == 0 {
		return res
	}

	// 根节点候选（含战术点强制并入）
	rootMoves := gobangRootMoves(ctx)
	if len(rootMoves) == 0 {
		return res
	}
	res.X, res.Y = rootMoves[0][0], rootMoves[0][1]

	// 迭代加深：2,4,6,8 层（黑+白成对）
	for depth := 2; depth <= 10; depth += 2 {
		ctx.killers = make([][2]int, depth+2)
		bestScore := int64(math.MinInt64 / 4)
		bestMove := [2]int{res.X, res.Y}
		alpha, beta := int64(math.MinInt64/4), int64(math.MaxInt64/4)
		abort := false
		for _, mv := range rootMoves {
			x, y := mv[0], mv[1]
			board[y][x] = mine
			score := -gobangNegamax(ctx, theirs, mine, depth-1, -beta, -alpha)
			board[y][x] = 0
			if ctx.cutoff {
				abort = true
				break // 丢弃本轮所有部分结果（含本轮 bestMove），沿用上一完整层
			}
			if score > bestScore {
				bestScore = score
				bestMove = [2]int{x, y}
				if score > alpha {
					alpha = score
				}
			}
		}
		if !abort && (bestScore > res.Score || res.Score == math.MinInt64/4) {
			res.Score = bestScore
			res.X, res.Y = bestMove[0], bestMove[1]
			res.Depth = depth
			res.Win = bestScore >= gobangWinScore
		}
		if abort || res.Win {
			res.Cutoff = abort
			break
		}
		if ctx.nodes > gobangSearchNodesLimit {
			res.Cutoff = true
			break
		}
		// 下一层用本层最优排前（简单 PV 排序）
		for i, mv := range rootMoves {
			if mv == bestMove {
				copy(rootMoves[1:i+1], rootMoves[0:i])
				rootMoves[0] = mv
				break
			}
		}
	}
	res.Nodes = ctx.nodes
	return res
}

const gobangWinScore = int64(1) << 40

// gobangNegamax 负极大搜索。sideToMove 落子。
func gobangNegamax(ctx *gobangSearchCtx, sideToMove, opp int, depth int, alpha, beta int64) int64 {
	ctx.nodes++
	if ctx.nodes%2048 == 0 && time.Now().After(ctx.deadline) {
		ctx.cutoff = true
	}
	if ctx.cutoff {
		return 0
	}

	if depth <= 0 {
		return gobangEvaluate(ctx, sideToMove)
	}

	var key uint64
	if ctx.tt != nil {
		key = gobangHash(ctx)
		if e, ok := ctx.tt[key]; ok && e.depth >= depth {
			switch e.flag {
			case 0:
				return e.score
			case 1:
				if e.score > alpha {
					alpha = e.score
				}
			case 2:
				if e.score < beta {
					beta = e.score
				}
			}
			if alpha >= beta {
				return e.score
			}
		}
	}

	moves := gobangOrderedMoves(ctx, sideToMove, depth)
	if len(moves) == 0 {
		return gobangEvaluate(ctx, sideToMove) // 满盘
	}

	origAlpha := alpha
	best := int64(math.MinInt64 / 4)
	bestMove := [2]int{-1, -1}
	for _, mv := range moves {
		x, y := mv[0], mv[1]
		ctx.board[y][x] = sideToMove
		// 快速终局检查：我落子成五？
		var score int64
		if gobangHasFiveAtXY(ctx.board, x, y, sideToMove, ctx.size) {
			score = gobangWinScore + int64(depth)
		} else {
			score = -gobangNegamax(ctx, opp, sideToMove, depth-1, -beta, -alpha)
		}
		ctx.board[y][x] = 0
		if ctx.cutoff {
			return 0
		}
		if score > best {
			best = score
			bestMove = [2]int{x, y}
			if score > alpha {
				alpha = score
				if alpha >= beta {
					// 记录杀手
					if d := len(ctx.killers); depth < d {
						ctx.killers[depth] = [2]int{x, y}
					}
					break
				}
			}
		}
	}

	flag := 0
	if best <= origAlpha {
		flag = 2
	} else if best >= beta {
		flag = 1
	}
	if ctx.tt != nil {
		ctx.tt[key] = gobangTTEntry{depth: depth, score: best, flag: flag}
	}
	_ = bestMove
	return best
}

// gobangHasFiveAtXY (x,y) 的 side 子是否属于某 ≥5 连。
func gobangHasFiveAtXY(board [][]int, x, y, side, size int) bool {
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

// gobangRootMoves 根候选：邻域点 + 战术点，按启发分排序。
func gobangRootMoves(ctx *gobangSearchCtx) [][2]int {
	return gobangOrderedMovesAt(ctx, ctx.mine, 0)
}

// gobangOrderedMoves 深层候选（带杀手优先）。
func gobangOrderedMoves(ctx *gobangSearchCtx, side, depth int) [][2]int {
	moves := gobangOrderedMovesAt(ctx, side, 0)
	if depth < len(ctx.killers) {
		k := ctx.killers[depth]
		for i, mv := range moves {
			if mv == k {
				copy(moves[1:i+1], moves[0:i])
				moves[0] = mv
				break
			}
		}
	}
	return moves
}

// gobangOrderedMovesAt 生成 side 的候选着法（排序截断）。截断宽度按深度收紧。
func gobangOrderedMovesAt(ctx *gobangSearchCtx, side, depth int) [][2]int {
	size := ctx.size
	opp := 3 - side
	base := gobangCandidates(ctx.board, 2)
	if len(base) == 0 {
		return nil
	}

	type sm struct {
		s int
		m [2]int
	}
	scored := make([]sm, 0, len(base))
	for _, mv := range base {
		x, y := mv[0], mv[1]
		// 轻量启发（热路径）：单点形态分。战术加成内联（避免全盘 Find* 每节点 4-6 次）：
		// 五点（任一方向落子成五）+终局分；活四延伸点 +次级分。
		s := gobangScorePoint(ctx.board, x, y, side)*22 + gobangScorePoint(ctx.board, x, y, opp)*6
		for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
			cnt := 1
			for _, sgn := range [2]int{1, -1} {
				nx, ny := x+d[0]*sgn, y+d[1]*sgn
				for nx >= 0 && ny >= 0 && ny < size && nx < len(ctx.board[ny]) && ctx.board[ny][nx] == side {
					cnt++
					nx += d[0] * sgn
					ny += d[1] * sgn
				}
			}
			if cnt >= 5 {
				s += 1000000 // 一手五连
				break
			}
		}
		for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
			cnt, oe := 1, 0
			for _, sgn := range [2]int{1, -1} {
				nx, ny := x+d[0]*sgn, y+d[1]*sgn
				for nx >= 0 && ny >= 0 && ny < size && nx < len(ctx.board[ny]) && ctx.board[ny][nx] == side {
					cnt++
					nx += d[0] * sgn
					ny += d[1] * sgn
				}
				if nx >= 0 && ny >= 0 && ny < size && nx < len(ctx.board[ny]) && ctx.board[ny][nx] == 0 {
					oe++
				}
			}
			if cnt == 4 && oe == 2 {
				s += 500000 // 成活四
				break
			}
		}
		s += gobangDoubleThreatBonus(ctx.board, x, y, side, size) * 40000
		s += gobangDoubleThreatBonus(ctx.board, x, y, opp, size) * 25000
		scored = append(scored, sm{s, mv})
	}
	// 降序插入排序
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].s > scored[j-1].s; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}
	width := 18
	if depth > 4 {
		width = 12
	}
	if len(scored) > width {
		scored = scored[:width]
	}
	out := make([][2]int, len(scored))
	for i, v := range scored {
		out[i] = v.m
	}
	return out
}

// gobangEvaluate 全盘评估（sideToMove 视角）：5 窗口滑动模式差 + 骨架加成。
func gobangEvaluate(ctx *gobangSearchCtx, sideToMove int) int64 {
	size := ctx.size
	myS := gobangBoardPatternScore(ctx.board, sideToMove, size)
	opS := gobangBoardPatternScore(ctx.board, 3-sideToMove, size)
	// 对方威胁略偏重（后手防御性）；活三/跳三形态已由 pattern 分覆盖
	// （gobangRunScore 对 cntSide==3 的连续/跳变体分别计分）。
	return int64(myS) - int64(opS)*5/2
}

// gobangBoardPatternScore 全盘模式计分：对每个 5 窗口统计「仅含 side 子」的窗口形态。
func gobangBoardPatternScore(board [][]int, side, size int) int {
	total := 0
	dirs := [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}
	// 用已落 side 子作为窗口锚点，避免全盘滑窗重复
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != side {
				continue
			}
			for _, d := range dirs {
				// 仅从「线的起点」计一次（左侧/上侧无同 side 延续时）
				px, py := x-d[0], y-d[1]
				if px >= 0 && py >= 0 && py < size && px < len(board[py]) && board[py][px] == side {
					continue
				}
				// 沿方向数连续（允许中间空格：用窗口法）
				n := 0
				nx, ny := x, y
				for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) && n < 6 {
					v := board[ny][nx]
					if v == side || v == 0 {
						n++
						nx += d[0]
						ny += d[1]
					} else {
						break
					}
				}
				// n 是从锚点起「side+空」的可用长度；统计窗口内 side 数
				cntSide := 1
				gap := 0
				cx, cy := x+d[0], y+d[1]
				for k := 1; k < n && k < 5; k++ {
					v := board[cy][cx]
					if v == side {
						cntSide++
					} else {
						gap++
					}
					if cy+d[1] >= 0 && cx+d[0] >= 0 && cy+d[1] < size && cx+d[0] < len(board[cy]) {
						cx += d[0]
						cy += d[1]
					} else {
						break
					}
				}
				total += gobangRunScore(cntSide, gap, n)
			}
		}
	}
	return total
}

// gobangRunScore 一条「side+空」连续段的形态分。
func gobangRunScore(cntSide, gap, avail int) int {
	if avail < 5 {
		return 0
	}
	switch {
	case cntSide >= 5:
		return 100000
	case cntSide == 4 && gap <= 0:
		return 12000 // 连续四（对手必堵）
	case cntSide == 4:
		return 6000 // 跳四
	case cntSide == 3 && gap == 0:
		return 2500 // 活三
	case cntSide == 3 && gap == 1:
		return 1500 // 跳三
	case cntSide == 2 && gap == 0:
		return 400
	case cntSide == 2:
		return 150
	default:
		return 40
	}
}

// GobangBestMoveForHint 导出单点决策（API hint 端点桥接）。
func GobangBestMoveForHint(board [][]int, mine, theirs int) (int, int, bool) {
	return gobangBestMove(board, mine, theirs)
}
