package entertainment

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// BetRequest 通用下注请求（selection=big/small/点数/赛事id/tile_index；extra=双色球号码等）。
type BetRequest struct {
	Selection string            `json:"selection"`
	Amount    float64           `json:"amount"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// Bettable 下注/选号能力（猜大小/骰宝/双色球/翻面/Betting/红包）。
type Bettable interface {
	Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error)
}

// BetEvent 一场竞猜主题（OurBits bet.php 列表行：主题/结束时间/庄家/下注总数/投注总额/我的下注/状态）。
type BetEvent struct {
	ID         int     `json:"id"`          // 主题 id（bet_odds.php?id=N）
	Title      string  `json:"title"`       // 2026年美加墨世界杯冠军
	EndTime    string  `json:"end_time"`    // 2026-07-12 00:00:00
	Remain     string  `json:"remain"`      // 10天07:33:50
	Bookmaker  string  `json:"bookmaker"`   // 庄家用户名
	BetCount   int     `json:"bet_count"`   // 下注总人数
	TotalStake float64 `json:"total_stake"` // 投注总额（魔力）
	MyStake    string  `json:"my_stake"`    // 我的下注（"-"=未押 / 金额文本）
	Status     string  `json:"status"`      // open（点击押注）/ ended（已结束）
}

// BetOption 一个押注选项（bet_odds.php?id=N 盘口：队名/人名 + 赔率）。
type BetOption struct {
	ID    string  `json:"id"`              // 选项标识（下单 POST 用）
	Name  string  `json:"name"`            // 队名/人名
	Odds  float64 `json:"odds"`            // 赔率
	Stake float64 `json:"stake,omitempty"` // 该选项当前投注额
}

// BetEventDetail 某主题盘口（bet_odds.php?id=N：主题信息 + 选项赔率列表）。
type BetEventDetail struct {
	Event    BetEvent    `json:"event"`
	Options  []BetOption `json:"options"`
	MyOption string      `json:"my_option,omitempty"` // 已押选项
	Note     string      `json:"note,omitempty"`      // 规则/赔付提示
}

// EventBettable 赛事押注能力（OurBits 菠菜）：列竞猜主题 + 某主题盘口。
// 下单仍走 Bettable.Bet（Extra["event_id"]=主题id / Selection=选项id / Amount=魔力），
// 历史仍走 Historable.History（bet_coupon.php 我的押注）。
type EventBettable interface {
	Events(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]BetEvent, error)
	EventDetail(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, eventID int) (*BetEventDetail, error)
}

// BlackjackResult 二十一点状态（玩家/庄家牌、点数、阶段、结果）。
type BlackjackResult struct {
	PlayerCards []string `json:"player_cards"`
	DealerCards []string `json:"dealer_cards"`
	PlayerScore int      `json:"player_score"`
	DealerScore int      `json:"dealer_score"`
	Stage       string   `json:"stage"`  // playing | finished
	Result      string   `json:"result"` // win | lose | push | blackjack | playing
	Balance     float64  `json:"balance"`
	Message     string   `json:"message"`
}

// Blackjackable 二十一点状态机（new/hit/stand/double）。
type Blackjackable interface {
	Blackjack(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string) (*BlackjackResult, error)
}

// FlipTier 翻面夺宝奖项档位（PandaPT getFlipTiers：特等/一等/…/五等）。
type FlipTier struct {
	ID         string  `json:"id"`              // a/b/c/d/e/f 或站点 tierId
	Name       string  `json:"name"`            // 特等奖 …
	Reward     float64 `json:"reward"`          // 50000（魔力）
	RewardType string  `json:"reward_type"`     // coins/medal/vip/none
	Count      int     `json:"count"`           // 该档卡牌张数（合计 100）
	Color      string  `json:"color,omitempty"` // 前端格子配色
}

// FlipTile 翻面夺宝单格（100 格之一）。
type FlipTile struct {
	Index   int    `json:"index"`             // 1..100
	Flipped bool   `json:"flipped"`           // 是否已翻开
	TierID  string `json:"tier_id,omitempty"` // 已翻开则填奖项 id
	ByUser  string `json:"by_user,omitempty"` // 谁翻的（本局记录）
}

// FlipState 翻面夺宝全景（PandaPT ajax_flip.php gameState+getFlipTiers 聚合）。
type FlipState struct {
	RoundNumber  int          `json:"round_number"`
	Balance      float64      `json:"balance"`
	CostPerFlip  float64      `json:"cost_per_flip"`
	TotalTiles   int          `json:"total_tiles"` // 100
	FlippedCount int          `json:"flipped_count"`
	Tiers        []FlipTier   `json:"tiers"`
	Tiles        []FlipTile   `json:"tiles"`
	History      []GameRecord `json:"history,omitempty"`
}

// Flipable 翻面夺宝能力（PandaPT flip.php：gameState+getFlipTiers 取全景，recordFlip 翻牌走 Bettable）。
type Flipable interface {
	FlipState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FlipState, error)
}
