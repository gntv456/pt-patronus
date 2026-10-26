package chain

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ErrGameNotFound 该站点未注册此游戏。
var ErrGameNotFound = errors.New("该站点没有这个游戏：请在娱乐页改选已支持的游戏/站点，或先为该站点登记凭据")

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

// SiteGameCount 站点 + 已适配娱乐项目数。嵌入 models.Site 令 JSON 字段平铺
// （除站点原有字段外多一个 game_count），供娱乐页站点筛选展示「N 个项目」。
type SiteGameCount struct {
	models.Site
	GameCount int `json:"game_count"`
}

// Sites 列出全部站点及其已适配的娱乐项目数。计数与 Games() 同一 hostMatch 规则
// （含 www 子域放宽），按 base_url 的 host 在 registry 匹配——不落库，纯运行时计算。
func (c *EntertainmentChain) Sites() ([]SiteGameCount, error) {
	sites, err := c.siteOp.List()
	if err != nil {
		return nil, err
	}
	out := make([]SiteGameCount, len(sites))
	for i, s := range sites {
		out[i] = SiteGameCount{Site: s, GameCount: len(c.reg.Games(s.BaseURL))}
	}
	// 按已适配游戏数降序：游戏多的站靠前，方便在站点筛选里快速找到适配了游戏的站。
	// 游戏数相同则保持 siteOp.List 的原顺序（priority desc, id asc）——
	// SliceStable + 恒 false 即原样保留相对次序。
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].GameCount != out[j].GameCount {
			return out[i].GameCount > out[j].GameCount
		}
		return false
	})
	return out, nil
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
		return nil, fmt.Errorf("%w连抽", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w统计", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w购卡", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w购卡", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w奖品展示", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w历史记录", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w勋章合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w勋章合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w勋章合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w勋章合成", ErrGameUnsupported)
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
		return nil, fmt.Errorf("%w勋章合成", ErrGameUnsupported)
	}
	key := fmt.Sprintf("%d:convert:%s:%d", siteID, gameID, yiYang)
	if !c.guard.acquire(key) {
		return nil, ErrDuplicateSpend
	}
	defer c.guard.release(key)
	return mc.ConvertShard(ctx, c.hc, base, cr, yiYang)
}

// OverviewGame 总览条目：游戏 + 分类（前端矩阵分组用）。
type OverviewGame struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Category 玩法分类（前端总览矩阵按此分组）：
	// wheel 转盘/老虎机 | bet 下注/选号 | stateful 状态机(21点/HiLo/猜大小) |
	// farm 农场/钓鱼 | table 棋牌(五子棋) | other 其他
	Category string `json:"category"`
}

// OverviewSite 总览单站：站点信息 + 游戏清单。
type OverviewSite struct {
	ID       int            `json:"id"`
	Name     string         `json:"name"`
	BaseURL  string         `json:"base_url"`
	IsActive bool           `json:"is_active"`
	Games    []OverviewGame `json:"games"`
}

// OverviewResp 总览响应：统计 + 站点清单（有游戏的站在前）。
type OverviewResp struct {
	TotalSites int            `json:"total_sites"` // 站点总数
	GameSites  int            `json:"game_sites"`  // 有已适配游戏的站点数
	TotalGames int            `json:"total_games"` // 已适配游戏总数
	Categories map[string]int `json:"categories"`  // 分类 → 游戏数
	Sites      []OverviewSite `json:"sites"`
}

// classifyGame 按游戏 id 归类（与前端组件路由对齐）。
func classifyGame(id string) string {
	switch {
	case id == "pting_gobang":
		return "table"
	}
	switch {
	case len(id) > 6 && id[len(id)-6:] == "_wheel", id == "cbg_slot", id == "tangpt_slot",
		id == "hdvideo_slots", id == "hhan_lucky_wheel", id == "march_wheel",
		id == "luckydraw_wheel", id == "ub_wheel", id == "tangpt_lottery":
		return "wheel"
	case id == "magic_farm", id == "siqi_plant", id == "siqi_fishing",
		id == "dubhe_stardust", id == "hdvideo_chest":
		return "farm"
	case id == "hdvideo_blackjack", id == "nxp_blackjack", id == "opencd_blackjack",
		id == "audiences_blackjack", id == "hdvideo_hilo", id == "hdvideo_bigsmall",
		id == "hxpt_bigsmall", id == "panda_bigsmall", id == "tangpt_bigsmall",
		id == "panda_flip", id == "panda_friendtrade":
		return "stateful"
	case len(id) > 4 && id[len(id)-4:] == "_bet":
		return "other"
	default:
		return "bet"
	}
}

// Overview 娱乐总览：一次返回全部站点与各自已适配游戏（带分类），供总览页矩阵渲染。
func (c *EntertainmentChain) Overview() (*OverviewResp, error) {
	sites, err := c.siteOp.List()
	if err != nil {
		return nil, err
	}
	resp := &OverviewResp{Categories: map[string]int{}, Sites: []OverviewSite{}}
	var withGames, noGames []OverviewSite
	for _, s := range sites {
		os := OverviewSite{ID: int(s.ID), Name: s.Name, BaseURL: s.BaseURL, IsActive: s.IsActive, Games: []OverviewGame{}}
		for _, g := range c.reg.Games(s.BaseURL) {
			cat := classifyGame(g.ID())
			os.Games = append(os.Games, OverviewGame{ID: g.ID(), Name: g.Name(), Category: cat})
			resp.Categories[cat]++
			resp.TotalGames++
		}
		if len(os.Games) > 0 {
			withGames = append(withGames, os)
		} else {
			noGames = append(noGames, os)
		}
	}
	resp.TotalSites = len(sites)
	resp.GameSites = len(withGames)
	resp.Sites = append(withGames, noGames...)
	return resp, nil
}
