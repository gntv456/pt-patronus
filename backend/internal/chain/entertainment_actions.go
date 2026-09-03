package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/entertainment"
)

// Farm 农场全景（仅 Farmable 游戏支持，如 NovaHD 魔力农场）。
func (c *EntertainmentChain) Farm(ctx context.Context, siteID uint, gameID string) (*entertainment.FarmState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Farmable)
	if !ok {
		return nil, errors.New("该游戏不支持农场")
	}
	return f.Farm(ctx, c.hc, base, cr)
}

// Plant 种植/养殖（section=crop→plant / animal→breed；花钱扣魔力，spendGuard 防重复）。
func (c *EntertainmentChain) Plant(ctx context.Context, siteID uint, gameID, section string, itemID int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Farmable)
	if !ok {
		return nil, errors.New("该游戏不支持农场")
	}
	key := fmt.Sprintf("%d:plant:%s:%s:%d", siteID, gameID, section, itemID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return f.Plant(ctx, c.hc, base, cr, section, itemID)
}

// Harvest 收获单个（防双击重复收获）。
func (c *EntertainmentChain) Harvest(ctx context.Context, siteID uint, gameID, section string, itemID int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Farmable)
	if !ok {
		return nil, errors.New("该游戏不支持农场")
	}
	key := fmt.Sprintf("%d:harvest:%s:%s:%d", siteID, gameID, section, itemID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return f.Harvest(ctx, c.hc, base, cr, section, itemID)
}

// HarvestAll 一键收获。
func (c *EntertainmentChain) HarvestAll(ctx context.Context, siteID uint, gameID string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Farmable)
	if !ok {
		return nil, errors.New("该游戏不支持农场")
	}
	key := fmt.Sprintf("%d:harvestall:%s", siteID, gameID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return f.HarvestAll(ctx, c.hc, base, cr)
}

// Sell 出售仓库物品（按市场价，花钱操作）。
func (c *EntertainmentChain) Sell(ctx context.Context, siteID uint, gameID, sellKey string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Farmable)
	if !ok {
		return nil, errors.New("该游戏不支持农场")
	}
	key := fmt.Sprintf("%d:sell:%s:%s", siteID, gameID, sellKey)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return f.Sell(ctx, c.hc, base, cr, sellKey)
}

// Bet 下注/选号（仅 Bettable 游戏支持；花钱操作，spendGuard 防重复扣币）。
func (c *EntertainmentChain) Bet(ctx context.Context, siteID uint, gameID string, req entertainment.BetRequest) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.Bettable)
	if !ok {
		return nil, errors.New("该游戏不支持下注")
	}
	key := fmt.Sprintf("%d:bet:%s:%s:%v", siteID, gameID, req.Selection, req.Amount)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return b.Bet(ctx, c.hc, base, cr, req)
}

// Events 列竞猜主题（仅 EventBettable 游戏支持，如 OurBits 赛事押注）。
func (c *EntertainmentChain) Events(ctx context.Context, siteID uint, gameID string) ([]entertainment.BetEvent, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	e, ok := g.(entertainment.EventBettable)
	if !ok {
		return nil, errors.New("该游戏不支持赛事押注")
	}
	return e.Events(ctx, c.hc, base, cr)
}

// EventDetail 某主题盘口（选项+赔率）。
func (c *EntertainmentChain) EventDetail(ctx context.Context, siteID uint, gameID string, eventID int) (*entertainment.BetEventDetail, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	e, ok := g.(entertainment.EventBettable)
	if !ok {
		return nil, errors.New("该游戏不支持赛事押注")
	}
	return e.EventDetail(ctx, c.hc, base, cr, eventID)
}

// Blackjack 二十一点动作（仅 Blackjackable 游戏支持；hit/stand/double 走 spendGuard 防双击）。
func (c *EntertainmentChain) Blackjack(ctx context.Context, siteID uint, gameID, action string) (*entertainment.BlackjackResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.Blackjackable)
	if !ok {
		return nil, errors.New("该游戏不支持二十一点")
	}
	if action != "new" {
		key := fmt.Sprintf("%d:blackjack:%s:%s", siteID, gameID, action)
		if !c.guard.acquire(key) {
			return nil, ErrDuplicateSpend
		}
		defer c.guard.release(key)
	}
	return b.Blackjack(ctx, c.hc, base, cr, action)
}

// FlipState 翻面夺宝全景（仅 Flipable 游戏支持，如 PandaPT 翻面夺宝）。只读，不走 spendGuard。
func (c *EntertainmentChain) FlipState(ctx context.Context, siteID uint, gameID string) (*entertainment.FlipState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Flipable)
	if !ok {
		return nil, errors.New("该游戏不支持翻面夺宝")
	}
	return f.FlipState(ctx, c.hc, base, cr)
}

// BigSmallState 猜大小全景快照（仅 BigSmallable 游戏支持：好学 hxpt_bigsmall / 熊猫 panda_bigsmall）。只读。
func (c *EntertainmentChain) BigSmallState(ctx context.Context, siteID uint, gameID string) (*entertainment.BigSmallState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	return b.BigSmallState(ctx, c.hc, base, cr)
}

// BigSmallChat 拉新聊天（只读）。
func (c *EntertainmentChain) BigSmallChat(ctx context.Context, siteID uint, gameID string, since int) ([]entertainment.BigSmallChat, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	return b.BigSmallChat(ctx, c.hc, base, cr, since)
}

// BigSmallSendChat 发聊天（非花钱，仅 in-flight 去重防连点刷屏）。
func (c *EntertainmentChain) BigSmallSendChat(ctx context.Context, siteID uint, gameID, message string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	key := fmt.Sprintf("%d:bschat:%s", siteID, gameID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return b.BigSmallSendChat(ctx, c.hc, base, cr, message)
}

// BigSmallTaskAction 任务操作（claim/reward/refund；花钱，spendGuard 防重复）。
func (c *EntertainmentChain) BigSmallTaskAction(ctx context.Context, siteID uint, gameID, action string, taskID int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	key := fmt.Sprintf("%d:bstask:%s:%s:%d", siteID, gameID, action, taskID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return b.BigSmallTaskAction(ctx, c.hc, base, cr, action, taskID)
}

// BigSmallUseItem 使用道具（花钱/有效果，spendGuard 防重复）。
func (c *EntertainmentChain) BigSmallUseItem(ctx context.Context, siteID uint, gameID, tag string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	key := fmt.Sprintf("%d:bsitem:%s:%s", siteID, gameID, tag)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return b.BigSmallUseItem(ctx, c.hc, base, cr, tag)
}

// BigSmallRedpacket 红包 send/claim/info（花钱，spendGuard 防重复）。
func (c *EntertainmentChain) BigSmallRedpacket(ctx context.Context, siteID uint, gameID, action string, extra map[string]string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	b, ok := g.(entertainment.BigSmallable)
	if !ok {
		return nil, errors.New("该游戏不支持猜大小")
	}
	key := fmt.Sprintf("%d:bsred:%s:%s:%s", siteID, gameID, action, extra["id"])
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return b.BigSmallRedpacket(ctx, c.hc, base, cr, action, extra)
}

// GobangState 五子棋全景快照（仅 Gobangable 游戏支持，如蜂巢五子棋）。只读。
func (c *EntertainmentChain) GobangState(ctx context.Context, siteID uint, gameID string) (*entertainment.GobangState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	gb, ok := g.(entertainment.Gobangable)
	if !ok {
		return nil, errors.New("该游戏不支持五子棋")
	}
	return gb.GobangState(ctx, c.hc, base, cr)
}

// GobangAction 五子棋动作（create/choose-color/move/undo/forfeit/retry-ai）。
// create 首次落子才扣门票、move/undo 实际花钱——统一走 spendGuard 防双击重复扣蜂蜜；
// choose-color/forfeit/retry-ai 语义上无扣费，但同键防连点。
func (c *EntertainmentChain) GobangAction(ctx context.Context, siteID uint, gameID, action string, params map[string]any) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	gb, ok := g.(entertainment.Gobangable)
	if !ok {
		return nil, errors.New("该游戏不支持五子棋")
	}
	// 参数序列化进防重键：同一动作+同一参数未返回前再来一次 → 429。
	pk, _ := json.Marshal(params)
	key := fmt.Sprintf("%d:gobang:%s:%s:%s", siteID, gameID, action, pk)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return gb.GobangAction(ctx, c.hc, base, cr, action, params)
}

// FriendTradeState 好友买卖全景快照（仅 FriendTradeable 游戏支持：PANDA panda_friendtrade）。只读。
func (c *EntertainmentChain) FriendTradeState(ctx context.Context, siteID uint, gameID string) (*entertainment.FriendTradeState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	ft, ok := g.(entertainment.FriendTradeable)
	if !ok {
		return nil, errors.New("该游戏不支持好友买卖")
	}
	return ft.FriendTradeState(ctx, c.hc, base, cr)
}

// City13WheelState 13City 大转盘全景（余额/今日次数/奖品环/最近开奖）。只读。
func (c *EntertainmentChain) City13WheelState(ctx context.Context, siteID uint, gameID string) (*entertainment.City13WheelState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	w, ok := g.(entertainment.City13Wheelable)
	if !ok {
		return nil, errors.New("该游戏不支持大转盘")
	}
	return w.City13WheelState(ctx, c.hc, base, cr)
}

// City13DiceState 13City 幸运骰子全景（余额/玩法/最近开奖）。只读。
func (c *EntertainmentChain) City13DiceState(ctx context.Context, siteID uint, gameID string) (*entertainment.City13DiceState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	d, ok := g.(entertainment.City13Diceable)
	if !ok {
		return nil, errors.New("该游戏不支持幸运骰子")
	}
	return d.City13DiceState(ctx, c.hc, base, cr)
}

// FishingState 钓鱼全景快照（仅 Fishingable 游戏支持：思齐 siqi_fishing）。只读。
func (c *EntertainmentChain) FishingState(ctx context.Context, siteID uint, gameID string) (*entertainment.FishingState, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	f, ok := g.(entertainment.Fishingable)
	if !ok {
		return nil, errors.New("该游戏不支持钓鱼")
	}
	return f.FishingState(ctx, c.hc, base, cr)
}

// FlipBridge 翻面夺宝 websocket 桥接：用站点 cookie 连 wss://host/wss，
// 返回 tileFlip 事件流（自动重连）。每会话一连接（等同一个浏览器开着翻面页）。
func (c *EntertainmentChain) FlipBridge(ctx context.Context, siteID uint, gameID string) (<-chan entertainment.FlipEvent, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	if _, ok := g.(entertainment.Flipable); !ok {
		return nil, errors.New("该游戏不支持翻面夺宝")
	}
	return entertainment.ServeFlipBridge(ctx, base, cr.Cookie), nil
}

// resolve 取站点+游戏+凭据+baseURL（公共前置）。
func (c *EntertainmentChain) resolve(siteID uint, gameID string) (entertainment.LotteryGame, string, entertainment.Creds, error) {
	s, err := c.siteOp.Get(siteID)
	if err != nil {
		return nil, "", entertainment.Creds{}, err
	}
	g, ok := c.reg.Game(s.BaseURL, gameID)
	if !ok {
		return nil, "", entertainment.Creds{}, ErrGameNotFound
	}
	cookie, _ := c.siteOp.Cookie(s)
	pw, _ := c.siteOp.Password(s) // 财神等 API 登录用；其余站点为空不影响
	return g, s.BaseURL, entertainment.Creds{Cookie: cookie, Username: s.Username, Password: pw}, nil
}
