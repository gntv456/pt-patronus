package entertainment

import (
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// ── v5 MCTS 引擎 ──────────────────────────────────────────────────────────
//
// 背景：v4 全局搜索（Alpha-Beta depth4-6）实战复盘证明零战术失误仍连败——
// 败于 HARD AI 的中盘多线布局。树搜索的窄深（每节点 8-18 分支）看不见
// 「20 手后多条弱线汇成杀」的布局价值；MCTS 的宽浅+随机恰好补这个维度：
// 大量自演对局自然评估「子力位置的长远价值」，不受固定深度限制。
//
// 设计：
//   - UCT 选点 + AMAF/RAVE 启发（五子棋着法强相关：某点赢的局里「后来下
//     该点」也常赢——经典 RAVE 加速收敛）。节点价值一律存「落子方视角」，
//     回传时按各节点 side 换算，UCT 双向（对手节点最小化我方）。
//   - 候选剪枝：每节点只展开邻域 2 格空点，按轻量评分取前 20；可由调用方
//     限定根候选（safe 过滤后的生存集）。
//   - 仿真（rollout）：战术贪心快进而非纯随机——成五/堵五无条件优先，
//     其余 ε=0.25 概率随机、否则采样 14 点评分最优；最多 24 手后用 v4
//     全盘评估判胜负倾向（>±3000 分胜负，否则 0.5 平）。
//   - 全败选顽强：根上所有着法胜率 <5% 时（实战「89 候选全死」局面），
//     改选「平均对局长度最长」的着法——同样输，撑最久等对手失误。
//   - 时间预算：默认 5s（env GOBANG_MCTS_BUDGET_MS 覆盖）。
//     站点每手 10s 时钟：战术层 ≤2.1s + MCTS 5s，留有余量。
//
// 纯本地计算，无网络请求。

const (
	mctsWidth        = 20   // 每节点展开宽度
	mctsRolloutMoves = 24   // 快进手数上限
	mctsSample       = 14   // rollout 每手评分采样数
	mctsEpsilon      = 0.25 // rollout 随机比例
	mctsExploreC     = 1.4  // UCT 探索系数
)

type mctsNode struct {
	move     [2]int // 到达该节点的着法（根为 {-1,-1}）
	parent   *mctsNode
	children []*mctsNode
	untried  [][2]int // 该节点待展开着法（下一落子方）
	wins     float64  // 落子方（side）视角的累计得分
	visits   int
	side     int // 落下 move 的一方（根 = 对方）
	raveW    float64
	raveN    int
	terminal bool // 该节点落子后已成五
	winner   int
	lenSum   int // 累计对局长度（全败时选「撑最久」）
}

// mctsPlayedMove 树+仿真中实际落过的子（AMAF 统计用）。
type mctsPlayedMove struct {
	mv   [2]int
	side int
}

// MCTSResult 产物。
type MCTSResult struct {
	X, Y        int
	WinRate     float64
	Simulations int
	Elapsed     time.Duration
}

// gobangMCTS 主入口：board 为当前局面，mine 为落子方。
// rootMoves 非 nil 时限定根候选（safe 生存集）；nil 则自动生成。
func gobangMCTS(board [][]int, mine, theirs int, budget time.Duration, rootMoves [][2]int) MCTSResult {
	size := len(board)
	t0 := time.Now()
	deadline := t0.Add(budget)
	res := MCTSResult{X: -1, Y: -1}

	var cands [][2]int
	if len(rootMoves) > 0 {
		cands = cloneMoves(rootMoves)
	} else {
		cands = mctsCandidates(board, mine, theirs, size)
	}
	if len(cands) == 0 {
		return res
	}
	if len(cands) == 1 {
		return MCTSResult{X: cands[0][0], Y: cands[0][1], WinRate: 0.5, Elapsed: time.Since(t0)}
	}

	root := &mctsNode{move: [2]int{-1, -1}, side: theirs, untried: cloneMoves(cands)}
	rng := rand.New(rand.NewSource(mctsSeed()))

	sims := 0
	for time.Now().Before(deadline) && sims < mctsSimCap() {
		node := root
		b := cloneBoard(board)
		rs := newRolloutState(b)
		played := []mctsPlayedMove{}
		path := []*mctsNode{root}
		pathAt := []int{0} // 站在该节点时 played 的长度（AMAF 只统计其后的子）

		// 1. Selection（UCT+RAVE）——全展开节点下行，遇终局停
		for !node.terminal && len(node.untried) == 0 && len(node.children) > 0 {
			node = mctsSelect(node)
			b[node.move[1]][node.move[0]] = node.side
			rs.place(node.move[0], node.move[1])
			played = append(played, mctsPlayedMove{node.move, node.side})
			path = append(path, node)
			pathAt = append(pathAt, len(played))
			if gobangHasFiveAtXY(b, node.move[0], node.move[1], node.side, size) {
				node.terminal = true
				node.winner = node.side
			}
		}
		// 2. Expansion：随机取一个未试着法（落子后再生成子层候选）
		if !node.terminal && len(node.untried) > 0 {
			i := rng.Intn(len(node.untried))
			mv := node.untried[i]
			node.untried = append(node.untried[:i], node.untried[i+1:]...)
			side := 3 - node.side
			if b[mv[1]][mv[0]] == 0 {
				b[mv[1]][mv[0]] = side
				rs.place(mv[0], mv[1])
				child := &mctsNode{move: mv, parent: node, side: side}
				if gobangHasFiveAtXY(b, mv[0], mv[1], side, size) {
					child.terminal = true
					child.winner = side
				} else {
					next := 3 - side
					child.untried = cloneMoves(mctsCandidates(b, next, side, size))
				}
				played = append(played, mctsPlayedMove{mv, side})
				node.children = append(node.children, child)
				node = child
				path = append(path, node)
				pathAt = append(pathAt, len(played))
			}
		}

		// 3. Simulation（终局节点直接定分，否则战术贪心快进）
		var score float64
		gameLen := len(played)
		if node.terminal {
			if node.winner == mine {
				score = 1
			}
		} else {
			var plies int
			score, plies = mctsRollout(rs, b, 3-node.side, mine, size, rng)
			gameLen += plies
		}

		// 4. Backpropagation：价值按各节点落子方视角换算；AMAF 只统计
		//    该节点之后落过的子，且排除直达子（它已有精确统计）。
		for k := len(path) - 1; k >= 0; k-- {
			n := path[k]
			n.visits++
			n.wins += mctsPersp(score, n.side, mine)
			n.lenSum += gameLen
			var direct *mctsNode
			if k+1 < len(path) {
				direct = path[k+1]
			}
			for _, pm := range played[pathAt[k]:] {
				for _, c := range n.children {
					if c == direct || c.move != pm.mv || c.side != pm.side {
						continue
					}
					c.raveN++
					c.raveW += mctsPersp(score, c.side, mine)
				}
			}
		}
		sims++
	}

	// 最优子：访问数最多（robust child）
	var best *mctsNode
	for _, c := range root.children {
		if best == nil || c.visits > best.visits {
			best = c
		}
	}
	// 全败（胜率 <5%）改选最顽强：平均对局长度最长，等对手失误
	if best != nil && best.visits >= 20 && best.wins/float64(best.visits) < 0.05 {
		var resist *mctsNode
		for _, c := range root.children {
			if c.visits < 20 {
				continue
			}
			if resist == nil || float64(c.lenSum)/float64(c.visits) > float64(resist.lenSum)/float64(resist.visits) {
				resist = c
			}
		}
		if resist != nil {
			best = resist
		}
	}
	if best != nil {
		res.X, res.Y = best.move[0], best.move[1]
		if best.visits > 0 {
			res.WinRate = best.wins / float64(best.visits)
		}
	}
	res.Simulations = sims
	res.Elapsed = time.Since(t0)
	return res
}

// mctsRootStat 根子统计（诊断/测试用）。
type mctsRootStat struct {
	move   [2]int
	visits int
	q      float64
	raveN  int
	raveQ  float64
	avgLen float64
}

// mctsRootStats 跑一次 MCTS 返回根子统计降序（诊断用，非热路径）。
func mctsRootStats(board [][]int, mine, theirs int, budget time.Duration) []mctsRootStat {
	size := len(board)
	cands := mctsCandidates(board, mine, theirs, size)
	root := &mctsNode{move: [2]int{-1, -1}, side: theirs, untried: cloneMoves(cands)}
	deadline := time.Now().Add(budget)
	rng := rand.New(rand.NewSource(1))
	sims := 0
	for time.Now().Before(deadline) && sims < mctsSimCap() {
		node := root
		b := cloneBoard(board)
		rs := newRolloutState(b)
		played := []mctsPlayedMove{}
		path := []*mctsNode{root}
		pathAt := []int{0}
		for !node.terminal && len(node.untried) == 0 && len(node.children) > 0 {
			node = mctsSelect(node)
			b[node.move[1]][node.move[0]] = node.side
			rs.place(node.move[0], node.move[1])
			played = append(played, mctsPlayedMove{node.move, node.side})
			path = append(path, node)
			pathAt = append(pathAt, len(played))
			if gobangHasFiveAtXY(b, node.move[0], node.move[1], node.side, size) {
				node.terminal = true
				node.winner = node.side
			}
		}
		if !node.terminal && len(node.untried) > 0 {
			i := rng.Intn(len(node.untried))
			mv := node.untried[i]
			node.untried = append(node.untried[:i], node.untried[i+1:]...)
			side := 3 - node.side
			if b[mv[1]][mv[0]] == 0 {
				b[mv[1]][mv[0]] = side
				rs.place(mv[0], mv[1])
				child := &mctsNode{move: mv, parent: node, side: side}
				if gobangHasFiveAtXY(b, mv[0], mv[1], side, size) {
					child.terminal = true
					child.winner = side
				} else {
					child.untried = cloneMoves(mctsCandidates(b, 3-side, side, size))
				}
				played = append(played, mctsPlayedMove{mv, side})
				node.children = append(node.children, child)
				node = child
				path = append(path, node)
				pathAt = append(pathAt, len(played))
			}
		}
		var score float64
		gameLen := len(played)
		if node.terminal {
			if node.winner == mine {
				score = 1
			}
		} else {
			var plies int
			score, plies = mctsRollout(rs, b, 3-node.side, mine, size, rng)
			gameLen += plies
		}
		for k := len(path) - 1; k >= 0; k-- {
			n := path[k]
			n.visits++
			n.wins += mctsPersp(score, n.side, mine)
			n.lenSum += gameLen
			var direct *mctsNode
			if k+1 < len(path) {
				direct = path[k+1]
			}
			for _, pm := range played[pathAt[k]:] {
				for _, c := range n.children {
					if c == direct || c.move != pm.mv || c.side != pm.side {
						continue
					}
					c.raveN++
					c.raveW += mctsPersp(score, c.side, mine)
				}
			}
		}
		sims++
	}
	out := make([]mctsRootStat, 0, len(root.children))
	for _, c := range root.children {
		st := mctsRootStat{move: c.move, visits: c.visits, raveN: c.raveN}
		if c.visits > 0 {
			st.q = c.wins / float64(c.visits)
			st.avgLen = float64(c.lenSum) / float64(c.visits)
		}
		if c.raveN > 0 {
			st.raveQ = c.raveW / float64(c.raveN)
		}
		out = append(out, st)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].visits > out[j-1].visits; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out[:min(6, len(out))]
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// mctsPersp 把 mine 视角的得分换算为 side 视角。
func mctsPersp(score float64, side, mine int) float64 {
	if side == mine {
		return score
	}
	return 1 - score
}

// mctsSelect UCT+RAVE 混合选点（子价值已统一为各子落子方视角，直接最大化）。
func mctsSelect(n *mctsNode) *mctsNode {
	best := n.children[0]
	bestV := -1.0
	for _, c := range n.children {
		if c.visits == 0 {
			return c
		}
		q := c.wins / float64(c.visits)
		if c.raveN > 0 {
			beta := float64(c.raveN) / (float64(c.raveN) + float64(c.visits) + 4.0*float64(c.visits)*float64(c.raveN)/50.0)
			q = (1-beta)*q + beta*(c.raveW/float64(c.raveN))
		}
		v := q + mctsExploreC*math.Sqrt(2*math.Log(float64(n.visits))/float64(c.visits))
		if v > bestV {
			bestV, best = v, c
		}
	}
	return best
}

// ── rollout：增量候选集 + 战术贪心 ─────────────────────────────────────

type rolloutState struct {
	b    [][]int
	grid [][]bool // 候选标记（已落子处为 false）
	list [][2]int // 允许含 stale 项，policy 内原地压缩
}

func newRolloutState(b [][]int) *rolloutState {
	rs := &rolloutState{b: b}
	rs.grid = make([][]bool, len(b))
	for i := range rs.grid {
		rs.grid[i] = make([]bool, len(b[i]))
	}
	for _, c := range gobangCandidates(b, 1) {
		rs.add(c[0], c[1])
	}
	return rs
}

func (rs *rolloutState) add(x, y int) {
	if x < 0 || y < 0 || y >= len(rs.b) || x >= len(rs.b[y]) || rs.b[y][x] != 0 || rs.grid[y][x] {
		return
	}
	rs.grid[y][x] = true
	rs.list = append(rs.list, [2]int{x, y})
}

// place 落子后维护候选集（调用方已写盘面）。
func (rs *rolloutState) place(x, y int) {
	rs.grid[y][x] = false
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx != 0 || dy != 0 {
				rs.add(x+dx, y+dy)
			}
		}
	}
}

// mctsRollout 战术贪心快进：返回（mine 视角得分, 消耗手数）。
func mctsRollout(rs *rolloutState, b [][]int, sideToMove, mine, size int, rng *rand.Rand) (float64, int) {
	cur := sideToMove
	plies := 0
	for i := 0; i < mctsRolloutMoves; i++ {
		mv := rs.policy(cur, size, rng)
		if mv == nil {
			break
		}
		b[mv[1]][mv[0]] = cur
		rs.place(mv[0], mv[1])
		plies++
		if gobangHasFiveAtXY(b, mv[0], mv[1], cur, size) {
			if cur == mine {
				return 1, plies
			}
			return 0, plies
		}
		cur = 3 - cur
	}
	sc := gobangEvaluate(&gobangSearchCtx{board: b, mine: mine, theirs: 3 - mine, size: size}, mine)
	switch {
	case sc > 3000:
		return 1, plies
	case sc < -3000:
		return 0, plies
	default:
		return 0.5, plies
	}
}

// policy rollout 着法：成五 > 堵五 > 采样评分最优（ε 概率随机）。
func (rs *rolloutState) policy(side, size int, rng *rand.Rand) *[2]int {
	opp := 3 - side
	var block *[2]int
	n := 0
	for _, c := range rs.list {
		if !rs.grid[c[1]][c[0]] {
			continue
		}
		rs.list[n] = c // 原地压缩
		n++
		if runCount(rs.b, c[0], c[1], side, size) >= 5 {
			return &c
		}
		if block == nil && runCount(rs.b, c[0], c[1], opp, size) >= 5 {
			cc := c
			block = &cc
		}
	}
	rs.list = rs.list[:n]
	if block != nil {
		return block
	}
	if n == 0 {
		return nil
	}
	if rng.Float64() < mctsEpsilon {
		mv := rs.list[rng.Intn(n)]
		return &mv
	}
	k := mctsSample
	if k > n {
		k = n
	}
	var pick [2]int
	found := false
	best := -1 << 60
	for i := 0; i < k; i++ {
		c := rs.list[rng.Intn(n)]
		s := gobangScorePoint(rs.b, c[0], c[1], side)*22 + gobangScorePoint(rs.b, c[0], c[1], opp)*6
		s += gobangDoubleThreatBonus(rs.b, c[0], c[1], side, size) * 40000
		if s > best {
			best = s
			pick = c
			found = true
		}
	}
	if !found {
		pick = rs.list[0]
	}
	return &pick
}

// runCount (x,y) 落 side 子后过该点的最长连续同色数（快速五连判定用）。
func runCount(b [][]int, x, y, side, size int) int {
	bestN := 1
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		cnt := 1
		for _, sgn := range [2]int{1, -1} {
			nx, ny := x+d[0]*sgn, y+d[1]*sgn
			for nx >= 0 && ny >= 0 && ny < size && nx < len(b[ny]) && b[ny][nx] == side {
				cnt++
				nx += d[0] * sgn
				ny += d[1] * sgn
			}
		}
		if cnt > bestN {
			bestN = cnt
		}
	}
	return bestN
}

// mctsCandidates 展开候选：邻域空点按轻量评分取前 mctsWidth 个。
func mctsCandidates(b [][]int, mine, theirs, size int) [][2]int {
	base := gobangCandidates(b, 2)
	type sm struct {
		s int
		m [2]int
	}
	scored := make([]sm, 0, len(base))
	for _, mv := range base {
		if b[mv[1]][mv[0]] != 0 {
			continue
		}
		s := gobangScorePoint(b, mv[0], mv[1], mine)*22 + gobangScorePoint(b, mv[0], mv[1], theirs)*6
		s += gobangDoubleThreatBonus(b, mv[0], mv[1], mine, size) * 40000
		s += gobangDoubleThreatBonus(b, mv[0], mv[1], theirs, size) * 25000
		scored = append(scored, sm{s, mv})
	}
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].s > scored[j-1].s; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}
	if len(scored) > mctsWidth {
		scored = scored[:mctsWidth]
	}
	out := make([][2]int, len(scored))
	for i, v := range scored {
		out[i] = v.m
	}
	return out
}

func cloneBoard(b [][]int) [][]int {
	out := make([][]int, len(b))
	for i := range b {
		out[i] = make([]int, len(b[i]))
		copy(out[i], b[i])
	}
	return out
}

func cloneMoves(m [][2]int) [][2]int {
	out := make([][2]int, len(m))
	copy(out, m)
	return out
}

// mctsBudget MCTS 时间预算（env 覆盖，测试加速）。
func mctsBudget() time.Duration {
	if v := os.Getenv("GOBANG_MCTS_BUDGET_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return 5 * time.Second
}
