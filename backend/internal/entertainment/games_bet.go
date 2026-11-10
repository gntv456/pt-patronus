// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

// 下注/选号类娱乐（Bettable）：猜大小/骰宝/双色球/翻面/Betting/红包。纯 Cookie。
// 各 Game 实现 LotteryGame.Play（默认下注兜底）+ Bettable.Bet + Statable。
// 按站点拆分见 games_bet_<站点>_<玩法>.go（每文件 ≤400 行）。

// 校验加固常量：防负数/超大值透传站点（真实单注上限仍由各站 max_bet 字段优先钳位）。
const (
	maxBetCeiling     = 1e8 // 魔力/积分下注硬上限（1 亿，超出即钳位）
	maxMultiCount     = 100 // 连抽次数硬上限
	maxRedpacketClick = 200 // 红包雨单轮点击硬上限（服务端 click_limit 可能被篡改/超大）
)
