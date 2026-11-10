// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package entertainment 实现站点娱乐/抽奖游戏：LotteryGame 抽象 + 按站点 host 注册。
// 游戏契约来自好学源码（jgg.php / magic_scratch.php 等），POST 带 cookie 调用站点端点、解析 JSON。
package entertainment

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// PrizeOutcome 单次抽奖结果（连抽时 PlayResult.Prizes 放多份；LongPT 大转盘 luckyDraw 返回数组）。
type PrizeOutcome struct {
	PrizeID   int     `json:"prize_id"`
	PrizeName string  `json:"prize_name"`
	PrizeType string  `json:"prize_type"`          // LongPT 类型码：01魔力 02VIP 03上传 04彩虹ID … 99未中奖
	Amount    float64 `json:"amount"`              // prizeValue
	UnitName  string  `json:"unit_name,omitempty"` // GB / 天 / 个 …
	IsWin     bool    `json:"is_win"`              // prizeType != "99"
}

// PlayResult 是一次游戏的结果。
type PlayResult struct {
	Success bool           `json:"success"`
	Message string         `json:"message,omitempty"`  // 中奖信息 / 失败原因
	Prize   string         `json:"prize,omitempty"`    // 奖项名/描述
	PrizeID int            `json:"prize_id,omitempty"` // 奖品 id（九宫格/转盘动画定位用）
	Amount  float64        `json:"amount,omitempty"`   // 奖励数值（火花/魔力）
	Balance float64        `json:"balance,omitempty"`  // 剩余火花
	Prizes  []PrizeOutcome `json:"prizes,omitempty"`   // 多次抽奖结果（连抽）；单抽时含 1 项
	Raw     map[string]any `json:"raw,omitempty"`      // 原始响应
}

// Creds 是玩游戏所需的站点凭据：cookie 为主；财神等需 username/password 做 API 登录。
type Creds struct {
	Cookie   string
	Username string
	Password string // 明文（oper 已解密）
}

// LotteryGame 是一个游戏。
type LotteryGame interface {
	ID() string   // jgg | magic_scratch | ...
	Name() string // 九宫格 / 魔力刮刮乐 / ...
	Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error)
}

// StatsResult 游戏统计（富 UI 用：余额/剩余次数/价格）。
type StatsResult struct {
	Balance     float64 `json:"balance"`     // 货币余额（火花/金元宝/魔力）
	CardNumber  int     `json:"card_number"` // 剩余刮卡数（cspt）
	TodaySpins  int     `json:"today_spins"` // 今日已抽
	MaxDaily    int     `json:"max_daily"`   // 每日上限
	CostPerPlay float64 `json:"cost_per_play"`
	// 以下为 LongPT 等站点特有可选字段（omitempty 向后兼容现有游戏）：
	Discount     int    `json:"discount,omitempty"`     // 连抽折扣（百分比，75 = 7.5 折）
	Participants int    `json:"participants,omitempty"` // 每日抽奖当前参与人数
	StartTime    string `json:"start_time,omitempty"`   // 每日抽奖可参与开始时间
	EndTime      string `json:"end_time,omitempty"`     // 每日抽奖可参与结束时间
	OpenTime     string `json:"open_time,omitempty"`    // 每日抽奖开奖时间
}

// BuyOption 购卡/充值档位（cspt 刮刮乐：1/10/50/100 张）。
type BuyOption struct {
	Type      int     `json:"type"`
	BuyCount  int     `json:"buy_count"`
	GiftCount int     `json:"gift_count"`
	UnitPrice float64 `json:"unit_price"`
}

// PrizeDef 奖项展示（奖品展示网格用）。
type PrizeDef struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Color string `json:"color"`
	Type  string `json:"type"`
}

// GameRecord 一次历史记录（中奖记录区用）。
type GameRecord struct {
	PrizeName string  `json:"prize_name"`
	PrizeType string  `json:"prize_type"`
	Amount    float64 `json:"amount"`
	Cost      float64 `json:"cost"`
	CreatedAt string  `json:"created_at"`
}

// SynthFragment 单个碎片库存项（合成面板碎片网格用）。
type SynthFragment struct {
	Name     string `json:"name"`
	Image    string `json:"image"`
	Current  int    `json:"current"`  // 持有数
	Required int    `json:"required"` // 所需数
	IsRare   bool   `json:"is_rare"`
}

// SynthesisTarget 勋章合成目标（碎片抽取/合成面板用）。
type SynthesisTarget struct {
	ID            int             `json:"id"`
	Name          string          `json:"name"`
	Image         string          `json:"image"`
	Got           int             `json:"got"`       // 已集碎片数（汇总）
	Need          int             `json:"need"`      // 所需碎片数（汇总）
	Fragments     []SynthFragment `json:"fragments"` // 各碎片持有/所需
	DrawPrice     int             `json:"draw_price"`
	TenPrice      int             `json:"ten_price"`
	HundredPrice  int             `json:"hundred_price"`
	CanSynthesize bool            `json:"can_synthesize"`
	Pity          string          `json:"pity"` // 保底进度文本
}

// 可选能力接口：游戏按需实现，chain/API 按类型断言分发（对齐 Kotlin LotteryCardShop 等）。
type Statable interface {
	Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error)
}
type Purchasable interface {
	BuyOptions() []BuyOption
	Buy(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, optionType int) (*PlayResult, error)
}
type Prizeable interface {
	Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error)
}
type Historable interface {
	History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error)
}

// MultiPlayable 连抽能力（LongPT 大转盘 luckyDraw?size=N）。单抽走 LotteryGame.Play；连抽按 count 聚合。
type MultiPlayable interface {
	PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error)
}
