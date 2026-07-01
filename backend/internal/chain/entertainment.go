package chain

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ErrGameNotFound 该站点未注册此游戏。
var ErrGameNotFound = errors.New("game not found for site")

// ErrDuplicateSpend 同一"花钱"操作正在执行（并发双击/重放）。handler 映射为 429。
// 消耗的是站点游戏币（用站点 cookie 调外部，跨 PTPatronus 用户共享），故去重键按站点+操作。
var ErrDuplicateSpend = errors.New("操作正在处理，请勿重复提交")

// spendGuard 防止"花钱"操作被并发重复触发（双击/网络重放会向站点发两次请求，重复扣游戏币）。
// 仅 in-flight 去重，不做 cooldown——零误报：合法的连续操作（前一次已返回）永不被拦，
// 只挡"同一操作尚未返回又来一个"的明确异常。键 = site:action:game:params。
type spendGuard struct {
	mu       sync.Mutex
	inflight map[string]struct{}
}

func newSpendGuard() *spendGuard { return &spendGuard{inflight: map[string]struct{}{}} }

// acquire 占位；返回 false 表示同一操作正在执行（拒绝，不扣币）。
func (g *spendGuard) acquire(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.inflight[key]; ok {
		return false
	}
	g.inflight[key] = struct{}{}
	return true
}

// release 释放占位（操作结束，无论成功失败都要调，否则该键永久占用）。
func (g *spendGuard) release(key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inflight, key)
}

// EntertainmentChain 编排娱乐游戏：按站点 host 找游戏，带 cookie 调用站点端点。
type EntertainmentChain struct {
	Base
	reg    *entertainment.Registry
	hc     *httpclient.Client
	siteOp *oper.SiteOper
	guard  *spendGuard // "花钱"操作并发去重（防双击/重放重复扣站点游戏币）
}

// NewEntertainmentChain 构造。
func NewEntertainmentChain(b Base, reg *entertainment.Registry, hc *httpclient.Client, siteOp *oper.SiteOper) *EntertainmentChain {
	return &EntertainmentChain{Base: b, reg: reg, hc: hc, siteOp: siteOp, guard: newSpendGuard()}
}

// Games 列出某站点可用的游戏。
func (c *EntertainmentChain) Games(siteID uint) ([]entertainment.LotteryGame, error) {
	s, err := c.siteOp.Get(siteID)
	if err != nil {
		return nil, err
	}
	return c.reg.Games(s.BaseURL), nil
}

// Play 玩一次游戏（带站点凭据调站点端点）。
func (c *EntertainmentChain) Play(ctx context.Context, siteID uint, gameID string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%d:play:%s", siteID, gameID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return g.Play(ctx, c.hc, base, cr)
}

// PlayMulti 连抽（仅 MultiPlayable 游戏支持，如 LongPT 大转盘 size=N）。
func (c *EntertainmentChain) PlayMulti(ctx context.Context, siteID uint, gameID string, count int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mp, ok := g.(entertainment.MultiPlayable)
	if !ok {
		return nil, errors.New("该游戏不支持连抽")
	}
	key := fmt.Sprintf("%d:playmulti:%s:%d", siteID, gameID, count)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return mp.PlayMulti(ctx, c.hc, base, cr, count)
}

// Stats 游戏统计（仅 Statable 游戏支持，如财神刮刮乐）。
func (c *EntertainmentChain) Stats(ctx context.Context, siteID uint, gameID string) (*entertainment.StatsResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	st, ok := g.(entertainment.Statable)
	if !ok {
		return nil, errors.New("该游戏不支持统计")
	}
	return st.Stats(ctx, c.hc, base, cr)
}

// BuyOptions 购卡档位（仅 Purchasable 游戏支持）。
func (c *EntertainmentChain) BuyOptions(siteID uint, gameID string) ([]entertainment.BuyOption, error) {
	g, _, _, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	p, ok := g.(entertainment.Purchasable)
	if !ok {
		return nil, errors.New("该游戏不支持购卡")
	}
	return p.BuyOptions(), nil
}

// Buy 购卡（仅 Purchasable 游戏支持）。
func (c *EntertainmentChain) Buy(ctx context.Context, siteID uint, gameID string, optionType int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	p, ok := g.(entertainment.Purchasable)
	if !ok {
		return nil, errors.New("该游戏不支持购卡")
	}
	key := fmt.Sprintf("%d:buy:%s:%d", siteID, gameID, optionType)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return p.Buy(ctx, c.hc, base, cr, optionType)
}

// Prizes 奖品展示（仅 Prizeable 游戏支持）。
func (c *EntertainmentChain) Prizes(ctx context.Context, siteID uint, gameID string) ([]entertainment.PrizeDef, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	p, ok := g.(entertainment.Prizeable)
	if !ok {
		return nil, errors.New("该游戏不支持奖品展示")
	}
	return p.Prizes(ctx, c.hc, base, cr)
}

// History 抽奖记录（仅 Historable 游戏支持）。
func (c *EntertainmentChain) History(ctx context.Context, siteID uint, gameID string) ([]entertainment.GameRecord, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	h, ok := g.(entertainment.Historable)
	if !ok {
		return nil, errors.New("该游戏不支持历史记录")
	}
	return h.History(ctx, c.hc, base, cr)
}

// Syntheses 卡牌合成目标列表（仅 Synthesizable 游戏支持）。
func (c *EntertainmentChain) Syntheses(ctx context.Context, siteID uint, gameID string) ([]entertainment.SynthesisTarget, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	s, ok := g.(entertainment.Synthesizable)
	if !ok {
		return nil, errors.New("该游戏不支持合成")
	}
	return s.Syntheses(ctx, c.hc, base, cr)
}

// Draw 抽碎片（synthesisID 目标，count 1/10/100/其它）。
func (c *EntertainmentChain) Draw(ctx context.Context, siteID uint, gameID string, synthesisID, count int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	s, ok := g.(entertainment.Synthesizable)
	if !ok {
		return nil, errors.New("该游戏不支持合成")
	}
	key := fmt.Sprintf("%d:draw:%s:%d:%d", siteID, gameID, synthesisID, count)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return s.Draw(ctx, c.hc, base, cr, synthesisID, count)
}

// Synthesize 合成（集齐碎片后）。
func (c *EntertainmentChain) Synthesize(ctx context.Context, siteID uint, gameID string, synthesisID int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	s, ok := g.(entertainment.Synthesizable)
	if !ok {
		return nil, errors.New("该游戏不支持合成")
	}
	key := fmt.Sprintf("%d:synth:%s:%d", siteID, gameID, synthesisID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return s.Synthesize(ctx, c.hc, base, cr, synthesisID)
}

// MedalTree 勋章金字塔（仅 MedalComposable 游戏支持，如 LongPT 六十四卦）。
func (c *EntertainmentChain) MedalTree(ctx context.Context, siteID uint, gameID string) ([]entertainment.MedalNode, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mc, ok := g.(entertainment.MedalComposable)
	if !ok {
		return nil, errors.New("该游戏不支持勋章合成")
	}
	return mc.MedalTree(ctx, c.hc, base, cr)
}

// MedalShards 阳爻/阴爻碎片余额。
func (c *EntertainmentChain) MedalShards(ctx context.Context, siteID uint, gameID string) (*entertainment.MedalShardInfo, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mc, ok := g.(entertainment.MedalComposable)
	if !ok {
		return nil, errors.New("该游戏不支持勋章合成")
	}
	return mc.MedalShards(ctx, c.hc, base, cr)
}

// ComposeMedal 碎片合成（太极/两仪/四象）。
func (c *EntertainmentChain) ComposeMedal(ctx context.Context, siteID uint, gameID, medalID string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mc, ok := g.(entertainment.MedalComposable)
	if !ok {
		return nil, errors.New("该游戏不支持勋章合成")
	}
	key := fmt.Sprintf("%d:compose:%s:%s", siteID, gameID, medalID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return mc.ComposeMedal(ctx, c.hc, base, cr, medalID)
}

// ComposeBigMedal 大勋章合成（八卦/六十四卦，耗魔力+子勋章）。
func (c *EntertainmentChain) ComposeBigMedal(ctx context.Context, siteID uint, gameID, medalID string) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mc, ok := g.(entertainment.MedalComposable)
	if !ok {
		return nil, errors.New("该游戏不支持勋章合成")
	}
	key := fmt.Sprintf("%d:composebig:%s:%s", siteID, gameID, medalID)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return mc.ComposeBigMedal(ctx, c.hc, base, cr, medalID)
}

// ConvertShard 阴阳转换（yiYang>0 阴→阳，<0 阳→阴）。
func (c *EntertainmentChain) ConvertShard(ctx context.Context, siteID uint, gameID string, yiYang int) (*entertainment.PlayResult, error) {
	g, base, cr, err := c.resolve(siteID, gameID)
	if err != nil {
		return nil, err
	}
	mc, ok := g.(entertainment.MedalComposable)
	if !ok {
		return nil, errors.New("该游戏不支持勋章合成")
	}
	key := fmt.Sprintf("%d:convert:%s:%d", siteID, gameID, yiYang)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return mc.ConvertShard(ctx, c.hc, base, cr, yiYang)
}

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
