package entertainment

import "time"

// gobangBestMove 五子棋选点（v3 威胁空间搜索）。
// board[y][x]：0 空 / mine 己方 / theirs 对方。15×15 无禁手（FREESTYLE 长连有效）。
//
// 演进：v1 单层贪心 → v2 战术规则+深度2 minimax → v3 威胁空间搜索。
// v2 的实战败因（HARD 五连负复盘）：对手用「冲四+跳三」连环双杀，防守方必须在威胁
// 成形前 2-3 手预判拆解，静态评分做不到。v3 的核心是三层：
//
//  1. VCF（Victory by Continuous Four）：只搜「我连续冲四」的强制胜利序列。
//     冲四每步都逼对方必堵，分支因子极小，可搜很深（默认 9 层）。
//  2. 防守检验（防对手 VCF + 双杀拆解）：对我每个候选，模拟我落子后对手是否存在
//     VCF 胜或「冲四+活三」式不可防双杀；存在则该候选视为必败，换下一个。
//     这是 v2 最缺的「看得见对手两步杀」。
//  3. 带防守检验的深度2评估：通过了检验的候选按 v2 评分（含双威胁加成）排序取最优。
//
// 纯本地决策，无网络请求。总时间上限约 1.5s（VCF 单独 600ms + 主搜索 900ms）。
func gobangBestMove(board [][]int, mine, theirs int) (int, int, bool) {
	size := len(board)
	if size == 0 {
		return 0, 0, false
	}

	// 空盘/近乎空盘直接走中心
	if gobangStoneCount(board) <= 1 {
		c := size / 2
		if board[c][c] == 0 {
			return c, c, true
		}
	}

	cands := gobangCandidates(board, 2)
	if len(cands) == 0 {
		return 0, 0, false
	}

	// ── 1. 战术层：立即终结/必堵（保持 v2 语义）──
	if x, y, ok := gobangFindLineFive(board, mine); ok {
		return x, y, true
	}
	if x, y, ok := gobangFindLineFive(board, theirs); ok {
		return x, y, true
	}
	// 堵对方活四延伸（FindOpenFour 与 FindMakeOpenFour 同语义：落一子成活四）：
	// 多点命中时不能随便取第一个——挑「同时废掉对方更多后续威胁」的点。
	// 实战负例（a2d7 第24手）：黑斜活三延伸点有 (1,5)/(5,9)，(5,9) 同时是
	// 黑 VCF 杀链首手，堵 (1,5) 放任 VCF = 送死。
	// 判据：模拟我占该点后，对方是否仍有 VCF / 双五点——无杀者（kill 最小）胜出。
	// 后手必须抢主动：自己也能成活四延伸时优先攻（防守永远赢不了先手；
	// 离线验证：对战术型对手，防守优先的后手 0/6 全败，进攻优先提到 1/3 胜率）。
	if x, y, ok := gobangFindMakeOpenFour(board, mine); ok {
		return x, y, true
	}
	if x, y, ok := gobangFindOpenFour(board, mine); ok {
		return x, y, true
	}
	if mvs := gobangAllMakeOpenFour(board, theirs); len(mvs) > 0 {
		blockDeadline := time.Now().Add(1200 * time.Millisecond)
		best := mvs[0]
		bestKill := 3 // 3 = 未测上限
		for _, mv := range mvs {
			board[mv[1]][mv[0]] = mine
			kill := 0
			if len(gobangAllFiveSpots(board, theirs, 2)) >= 2 {
				kill = 2
			} else if _, _, ok := gobangVCFSearch(board, theirs, mine, 7, blockDeadline); ok {
				kill = 1
			}
			board[mv[1]][mv[0]] = 0
			better := kill < bestKill ||
				(kill == bestKill && gobangPointEval(board, mv[0], mv[1], mine, theirs, size) > gobangPointEval(board, best[0], best[1], mine, theirs, size))
			if better {
				bestKill, best = kill, mv
			}
			if time.Now().After(blockDeadline) {
				break
			}
		}
		return best[0], best[1], true
	}

	// ── 2. 我方 VCF：连续冲四强制取胜则直接下第一手 ──
	vcfDeadline := time.Now().Add(600 * time.Millisecond)
	if x, y, ok := gobangVCFSearch(board, mine, theirs, 9, vcfDeadline); ok {
		return x, y, true
	}
	// ── 2.5 交汇点必抢：对方 ≥2 条骨架线（各含≥2对方子、跨度≤4、遇我子截断）
	// 交汇的空点 = 四三双杀的物理载体。抢占即拆解（实战负例：黑 (2,8) 交汇横2子
	// +斜3子一手成四三杀）。多点命中时按「对方落此后的即时威胁(成五数,活四延伸)」
	// 降序，再按强度、战术分决胜。
	if x, y, ok := gobangForkPoint(board, theirs, size); ok {
		return x, y, true
	}

	// ── 3.5 MCTS（v5 布局搜索）：safe 集生存着法内的宽评估选点 ──
	// 树搜索（窄深）看不见「多线汇杀」的布局价值；MCTS 用大量随机自演补足。
	// 全败时（wr<5%）自动选「平均对局长度最长」的顽强点。
	// 大优/大劣直接判定时（wr≥95% 或 ≤5% 且收敛）无需 MCTS——战术层已覆盖。
	{
		defenseDeadline := time.Now().Add(1500 * time.Millisecond)
		scored := make([][3]int, 0, 64)
		for _, cc := range cands {
			sv := gobangPointEval(board, cc[0], cc[1], mine, theirs, size) // 含活三压制+骨架（v3 实战验证）
			scored = append(scored, [3]int{sv, cc[0], cc[1]})
		}
		gobangSortDesc(scored)
		top := scored
		if len(top) > 14 {
			top = top[:14]
		}
		var safe [][2]int
		for _, t := range top {
			x, y := t[1], t[2]
			board[y][x] = mine
			doomed := gobangOpponentWins(board, mine, theirs, defenseDeadline)
			board[y][x] = 0
			if !doomed {
				safe = append(safe, [2]int{x, y})
			}
			if time.Now().After(defenseDeadline) && len(safe) > 0 {
				break
			}
		}
		if len(safe) > 0 {
			if res := gobangMCTS(board, mine, theirs, mctsBudget(), safe); res.X >= 0 {
				return res.X, res.Y, true
			}
			// MCTS 异常兜底：safe 首点（评分最高）
			return safe[0][0], safe[0][1], true
		}
		// safe 空 = 战术层已尽：直接全局搜索全力找活路
	}

	// ── 4. 全局搜索（v4）：迭代加深 Alpha-Beta + 置换表 + 全盘模式评估 ──
	if res := gobangSearch(board, mine, theirs, gobangSearchBudget()); res.X >= 0 {
		return res.X, res.Y, true
	}

	// 搜索异常兜底：邻域首点
	c := gobangCandidates(board, 2)
	if len(c) == 0 {
		return 0, 0, false
	}
	return c[0][0], c[0][1], true
}

// gobangForkPoint 找对方 ≥2 条骨架线的最强交汇空点（详见战术层注释）。
func gobangForkPoint(board [][]int, side, size int) (int, int, bool) {
	type fork struct{ strength, x, y int }
	var forks []fork
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
			lines, strength := 0, 0
			for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
				cnt, nearest := 0, 99
				for _, sgn := range [2]int{1, -1} {
					nx, ny := x+d[0]*sgn, y+d[1]*sgn
					dist := 1
					for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) {
						v := board[ny][nx]
						if v != 0 && v != side {
							break // 我方子截断
						}
						if v == side {
							cnt++
							if dist < nearest {
								nearest = dist
							}
						}
						nx += d[0] * sgn
						ny += d[1] * sgn
						dist++
					}
				}
				if cnt >= 2 && nearest <= 4 {
					lines++
					strength += cnt
				}
			}
			if lines >= 2 {
				forks = append(forks, fork{strength, x, y})
			}
		}
	}
	if len(forks) == 0 {
		return 0, 0, false
	}
	// 即时威胁主排序：对方落此点后 (成五点数, 有无成活四延伸)
	threat := func(f fork) (int, int) {
		board[f.y][f.x] = side
		fives := gobangAllFiveSpots(board, side, 3)
		mo4 := 0
		if _, _, ok := gobangFindMakeOpenFour(board, side); ok {
			mo4 = 1
		}
		board[f.y][f.x] = 0
		return len(fives), mo4
	}
	best := forks[0]
	bf, bm := threat(best)
	for _, f := range forks[1:] {
		tf, tm := threat(f)
		if tf > bf || (tf == bf && (tm > bm || (tm == bm && f.strength > best.strength))) {
			best, bf, bm = f, tf, tm
		}
	}
	return best.x, best.y, true
}

// gobangVCFSearch 深度优先搜「side 连续冲四」的强制胜利：side 每步落子后要么成五，
// 要么形成冲四（对方唯一堵点已知），对方堵后继续。返回首手。
// 只考虑「落子后立即产生四连威胁（冲四或活四）」的点，分支因子小。
func gobangVCFSearch(board [][]int, side, opp int, depth int, deadline time.Time) (int, int, bool) {
	size := len(board)
	if depth <= 0 || time.Now().After(deadline) {
		return 0, 0, false
	}
	// 立即成五？
	if x, y, ok := gobangFindLineFive(board, side); ok {
		return x, y, true
	}
	// 所有「落子后成四连威胁」的点（冲四=对方必堵一端；活四=直接赢）
	fours := gobangFourMoves(board, side, 8)
	for _, m := range fours {
		if time.Now().After(deadline) {
			return 0, 0, false
		}
		x, y := m[0], m[1]
		board[y][x] = side
		won := false
		if gobangHasFiveAtXY(board, x, y, side, size) {
			won = true // 落子即真五（FREESTYLE ≥5 连）
		} else {
			// 对方必堵点：我成四后的两端之一（或缺口）。用「对方必堵=对方唯一五点」逻辑：
			// 我冲四后，对方存在唯一/多个成五点（我的五点），对方必须堵其中一个；
			// 对 VCF 而言我们检查对方所有可能堵法是否都挡不住（严格版）或取主堵点（近似）。
			// 近似：对方堵「我的成五点」；若我有两个成五点（活四效果），对方堵不完 → 胜。
			myFives := gobangAllFiveSpots(board, side, 2)
			if len(myFives) >= 2 {
				won = true // 双五点，对方一手堵不完
			} else if len(myFives) == 1 {
				bx, by := myFives[0][0], myFives[0][1]
				board[by][bx] = opp // 对方必堵
				if _, _, ok := gobangVCFSearch(board, side, opp, depth-1, deadline); ok {
					won = true
				}
				board[by][bx] = 0
			}
		}
		board[y][x] = 0
		if won {
			return x, y, true
		}
	}
	return 0, 0, false
}

// gobangFourMoves 找 side 落一子后形成「四连威胁」的点：
// 连续四连（cnt≥4）或跳冲四（落子后产生 ≥1 成五点，如 X_XXX 型）。
// VCF 候选必须含跳型——HARD 实战负例（a2d7 第24手）：杀链首手 (5,9) 是
// 跳冲四，纯连续版完全看不到，safe 过滤因此放行了必败的 (1,5)。
func gobangFourMoves(board [][]int, side int, n int) [][2]int {
	size := len(board)
	var out [][2]int
	for y := 0; y < size && len(out) < 40; y++ {
		for x := 0; x < len(board[y]) && len(out) < 40; x++ {
			if board[y][x] != 0 {
				continue
			}
			// 连续四连
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
				if cnt >= 4 {
					out = append(out, [2]int{x, y})
					break
				}
			}
			if len(out) > 0 && out[len(out)-1] == [2]int{x, y} {
				continue
			}
			// 跳冲四：落子后出现成五点（含缺口型四）
			board[y][x] = side
			hasFive := len(gobangAllFiveSpots(board, side, 1)) > 0
			board[y][x] = 0
			if hasFive {
				out = append(out, [2]int{x, y})
			}
		}
	}
	return out
}

// gobangAllFiveSpots side 所有「落一子即成五」的点（最多收集 n 个；VCF 判双五用）。
func gobangAllFiveSpots(board [][]int, side int, n int) [][2]int {
	size := len(board)
	var out [][2]int
	for y := 0; y < size && len(out) < n; y++ {
		for x := 0; x < len(board[y]) && len(out) < n; x++ {
			if board[y][x] != 0 {
				continue
			}
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
					out = append(out, [2]int{x, y})
					break
				}
			}
		}
	}
	return out
}

// gobangOpponentWins 我（mine）刚落完子、轮对方（theirs）时，对方是否有强制杀。
// 只判「确定性必败」，不做悲观猜测——过度悲观会让双方都不敢进攻，自对弈满盘不终局：
//  1. 对方一步成五点 ≥2（活四将成，我一手堵不完）
//  2. 对方 VCF：连续冲四强制取胜序列（深度限制内严格成立）
//
// 「堵四后对方仍有活三延伸」这类软双杀不计入——它还能被后续防守拆解，
// 由第 4 层评估的残留威胁惩罚处理（-80000/-200000 分而非直接淘汰）。
func gobangOpponentWins(board [][]int, mine, theirs int, deadline time.Time) bool {
	if fives := gobangAllFiveSpots(board, theirs, 2); len(fives) >= 2 {
		return true
	}
	if _, _, ok := gobangVCFSearch(board, theirs, mine, 7, deadline); ok {
		return true
	}
	if time.Now().After(deadline) {
		return false
	}
	// 一层双杀前瞻：对方落「四威胁点」→ 我必堵其成五点 → 对方仍有成五/成活四点
	// ⇒ 该候选必败（HARD 实战双杀的标准形态：冲四逼迫 + 活三/跳三待收）。
	for _, fm := range gobangFourMoves(board, theirs, 12) {
		if time.Now().After(deadline) {
			return false
		}
		fx, fy := fm[0], fm[1]
		board[fy][fx] = theirs
		fives := gobangAllFiveSpots(board, theirs, 2)
		if len(fives) >= 2 {
			board[fy][fx] = 0
			return true
		}
		if len(fives) == 1 {
			bx, by := fives[0][0], fives[0][1]
			board[by][bx] = mine // 我必堵
			still := false
			if _, _, ok := gobangFindLineFive(board, theirs); ok {
				still = true
			} else if _, _, ok := gobangFindMakeOpenFour(board, theirs); ok {
				still = true
			}
			board[by][bx] = 0
			board[fy][fx] = 0
			if still {
				return true
			}
		} else {
			board[fy][fx] = 0
		}
	}
	return false
}

// gobangPointEval 单点综合分：我方潜力×10 + 对方潜力（堵）+ 中心加成。
// 双威胁（四三杀）识别：一手同时形成冲四与活三时大幅加成——此类点攻防价值均近于必胜/必防
// （HARD 实战负例：AI 连续用纵向冲四+横向跳三双杀，单层贪心看不见）。
func gobangPointEval(board [][]int, x, y, mine, theirs, size int) int {
	s := gobangScorePoint(board, x, y, mine)*22 + gobangScorePoint(board, x, y, theirs)*3
	s += 15 - maxAbs(x-size/2, y-size/2)
	s += gobangDoubleThreatBonus(board, x, y, mine, size) * 50000
	s += gobangDoubleThreatBonus(board, x, y, theirs, size) * 30000
	// 活三压制：该点落子后对方剩余「能成活四的点」数计入负分（越少越好）。
	// HARD 连败根因：黑中盘攒 2~3 条活三线（跳三），白等威胁成形再防已晚一手；
	// 中期主动拆线（离线对弈胜率 1/4 → 3/4）。逐点比较时基线项相消，等价于「计数差 × 6000」。
	board[y][x] = mine
	s -= gobangCountOpenThrees(board, theirs, size) * 6000
	board[y][x] = 0
	// 双活三杀手点前瞻（0ffa073e 负例根因）：黑 (8,11) 一手造出 3 个
	// 「落子成活四」点 = 双活三网，白怎么堵都漏一。对方若在此点落子将
	// 新增 ≥2 个成活四点时，该点必须重罚（抢占/预防价值）——单纯的
	// VCF/safe 看不到这种非冲四杀形。
	s -= gobangOpenFourSeedsAfter(board, x, y, theirs, size) * 26000
	// 跳三骨架预见（落子前计算）：该点处于对方「同线≥2子且线上无我子」的骨架位时
	// 按跨度加权——跨度≥3（跳三雏形，实战杀型种子）必须压过自身进攻收益。
	s += gobangSkeletonBonus(board, x, y, theirs, size) * 12000
	return s
}

// gobangOpenFourSeedsAfter 假设 side 落 (x,y) 后，side 的「落一子成活四」
// 点新增数量（相对当前）。≥2 = 双活三杀网成形（对方一手堵不完）。
func gobangOpenFourSeedsAfter(board [][]int, x, y, side, size int) int {
	before := gobangCountMakeOpenFour(board, side, size)
	if board[y][x] != 0 {
		return 0
	}
	board[y][x] = side
	after := gobangCountMakeOpenFour(board, side, size)
	board[y][x] = 0
	if after <= before {
		return 0
	}
	return after - before
}

// gobangCountMakeOpenFour side 当前「落一子成活四」的点数（上限 4，够判双）。
func gobangCountMakeOpenFour(board [][]int, side, size int) int {
	n := 0
	for y := 0; y < size && n < 4; y++ {
		for x := 0; x < len(board[y]) && n < 4; x++ {
			if board[y][x] != 0 {
				continue
			}
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
				if cnt == 4 && openEnds == 2 {
					n++
					break
				}
			}
		}
	}
	return n
}

// gobangSkeletonBonus (x,y) 若为对方骨架位（该点与某对方子同线、线上无我方子、
// 且对方在线上另有≥1子）返回该线跨度，否则 0。跨度≥3 即跳三雏形。
func gobangSkeletonBonus(board [][]int, x, y, side, size int) int {
	if board[y][x] != 0 {
		return 0
	}
	best := 0
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}} {
		dx, dy := d[0], d[1]
		mine := 0
		blocked := false
		for _, sgn := range [2]int{1, -1} {
			nx, ny := x+dx*sgn, y+dy*sgn
			for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) {
				v := board[ny][nx]
				if v == side {
					mine++
				} else if v != 0 {
					blocked = true
					break
				}
				nx += dx * sgn
				ny += dy * sgn
			}
			if blocked {
				break
			}
		}
		if blocked || mine < 1 {
			continue
		}
		// 跨度 = (x,y) 与最近对方子的距离 + 对方子间最大延伸（近似取 mine+1 线上子数）
		span := 1
		for _, sgn := range [2]int{1, -1} {
			dist := 0
			nx, ny := x+dx*sgn, y+dy*sgn
			for nx >= 0 && ny >= 0 && ny < size && nx < len(board[ny]) && board[ny][nx] != 2 {
				if board[ny][nx] == 1 {
					if dist+1 > span {
						span = dist + 1
					}
					break
				}
				dist++
				nx += dx * sgn
				ny += dy * sgn
			}
		}
		if span > best {
			best = span
		}
	}
	if best >= 2 {
		return best
	}
	return 0
}

// gobangCountOpenThrees 数 side「能一手成活四」的空点数（连续活三/跳三各贡献其补位点）。
func gobangCountOpenThrees(board [][]int, side, size int) int {
	n := 0
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
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
				if cnt == 3 && openEnds == 2 {
					n++
					break
				}
			}
		}
	}
	return n
}

// gobangDoubleThreatBonus (x,y) 落 side 子后是否同时含「冲四及以上」与「活三」方向：1=是 0=否。
func gobangDoubleThreatBonus(board [][]int, x, y, side, size int) int {
	hasFour, hasOpen3 := false, false
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
		if cnt >= 4 {
			hasFour = true
		}
		if cnt == 3 && openEnds == 2 {
			hasOpen3 = true
		}
	}
	if hasFour && hasOpen3 {
		return 1
	}
	return 0
}

// gobangFindLineFive 找 side 落一子即成五（含长连≥5，FREESTYLE）的点。
func gobangFindLineFive(board [][]int, side int) (int, int, bool) {
	size := len(board)
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
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
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// gobangFindOpenFour 找 side 落一子成「活四」（两端全空的四）的点——下一手双点连五无解。
func gobangFindOpenFour(board [][]int, side int) (int, int, bool) {
	size := len(board)
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
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
				if cnt == 4 && openEnds == 2 {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// gobangFindMakeOpenFour 找 side 落一子后形成「活四」的点位：
// 即当前已有两端皆空的活三（含跳三），延伸一手即成活四（此后双点连五无解）。
// 实现为「模拟落子后该方向四连+两端空」——天然覆盖连续三与跳三（HARD 第二局负例根因）。
func gobangFindMakeOpenFour(board [][]int, side int) (int, int, bool) {
	size := len(board)
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
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
				if cnt == 4 && openEnds == 2 {
					return x, y, true
				}
			}
		}
	}
	return 0, 0, false
}

// gobangAllMakeOpenFour 同上但收集全部命中点（堵活四延伸需比较优劣）。
func gobangAllMakeOpenFour(board [][]int, side int) [][2]int {
	size := len(board)
	var out [][2]int
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] != 0 {
				continue
			}
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
				if cnt == 4 && openEnds == 2 {
					out = append(out, [2]int{x, y})
					break
				}
			}
		}
	}
	return out
}

// gobangCandidates 已落子 radius 格内的空点（去重天然由坐标唯一保证）。
func gobangCandidates(board [][]int, radius int) [][2]int {
	size := len(board)
	seen := make(map[[2]int]struct{})
	var out [][2]int
	for y := 0; y < size; y++ {
		for x := 0; x < len(board[y]); x++ {
			if board[y][x] == 0 {
				continue
			}
			for dy := -radius; dy <= radius; dy++ {
				for dx := -radius; dx <= radius; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || ny >= size || nx >= len(board[ny]) || board[ny][nx] != 0 {
						continue
					}
					k := [2]int{nx, ny}
					if _, ok := seen[k]; !ok {
						seen[k] = struct{}{}
						out = append(out, k)
					}
				}
			}
		}
	}
	if len(out) == 0 && size > 0 {
		out = append(out, [2]int{size / 2, size / 2}) // 空盘兜底
	}
	return out
}

func gobangStoneCount(board [][]int) int {
	n := 0
	for _, row := range board {
		for _, v := range row {
			if v != 0 {
				n++
			}
		}
	}
	return n
}

// gobangSortDesc 按 [3]int{score, x, y} 的 score 降序（简单插入，n 小）。
func gobangSortDesc(a [][3]int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j][0] > a[j-1][0]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
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
