// 五子棋 MCTS 的可复现开关。
//
// gobangMCTS 默认用墙钟播种、按墙钟预算收工：同一局面两次搜索可以给出不同着法，
// 模拟数还随机器负载上下波动。这让门禁变成抛硬币——整仓 `go test ./...` 实测出现过
// 「move far from battlefield」随机红。测试侧注入固定种子 + 固定模拟数（并配超长预算，
// 让终止只由模拟数决定）后，搜索结果逐子可复现；生产路径两个变量都是零值，行为不变。
package entertainment

import "time"

var (
	mctsForceSeed int64 // 0 = 按墙钟播种（生产默认）
	mctsForceSims int   // 0 = 沿用原 200000 上限（生产默认）
)

// mctsSeed 搜索 RNG 的播种值。
func mctsSeed() int64 {
	if mctsForceSeed != 0 {
		return mctsForceSeed
	}
	return time.Now().UnixNano()
}

// mctsSimCap 单次搜索的模拟数上限。
func mctsSimCap() int {
	if mctsForceSims > 0 {
		return mctsForceSims
	}
	return 200000
}
