// Package entertainment 实现站点娱乐/抽奖游戏：LotteryGame 抽象 + 按站点 host 注册。
// 游戏契约来自好学源码（jgg.php / magic_scratch.php 等），POST 带 cookie 调用站点端点、解析 JSON。
package entertainment

import (
	"context"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// PrizeOutcome 单次抽奖结果（连抽时 PlayResult.Prizes 放多份；LongPT 大转盘 luckyDraw 返回数组）。
type PrizeOutcome struct {
	PrizeID   int     `json:"prize_id"`
	PrizeName string  `json:"prize_name"`
	PrizeType string  `json:"prize_type"`            // LongPT 类型码：01魔力 02VIP 03上传 04彩虹ID … 99未中奖
	Amount    float64 `json:"amount"`                // prizeValue
	UnitName  string  `json:"unit_name,omitempty"`   // GB / 天 / 个 …
	IsWin     bool    `json:"is_win"`                // prizeType != "99"
}

// PlayResult 是一次游戏的结果。
type PlayResult struct {
	Success bool             `json:"success"`
	Message string           `json:"message,omitempty"` // 中奖信息 / 失败原因
	Prize   string           `json:"prize,omitempty"`   // 奖项名/描述
	PrizeID int              `json:"prize_id,omitempty"` // 奖品 id（九宫格/转盘动画定位用）
	Amount  float64          `json:"amount,omitempty"`  // 奖励数值（火花/魔力）
	Balance float64          `json:"balance,omitempty"` // 剩余火花
	Prizes  []PrizeOutcome   `json:"prizes,omitempty"`  // 多次抽奖结果（连抽）；单抽时含 1 项
	Raw     map[string]any   `json:"raw,omitempty"`     // 原始响应
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
	Balance     float64 `json:"balance"`      // 货币余额（火花/金元宝/魔力）
	CardNumber  int     `json:"card_number"`  // 剩余刮卡数（cspt）
	TodaySpins  int     `json:"today_spins"`  // 今日已抽
	MaxDaily    int     `json:"max_daily"`    // 每日上限
	CostPerPlay float64 `json:"cost_per_play"`
	// 以下为 LongPT 等站点特有可选字段（omitempty 向后兼容现有游戏）：
	Discount     int    `json:"discount,omitempty"`      // 连抽折扣（百分比，75 = 7.5 折）
	Participants int    `json:"participants,omitempty"`  // 每日抽奖当前参与人数
	StartTime    string `json:"start_time,omitempty"`    // 每日抽奖可参与开始时间
	EndTime      string `json:"end_time,omitempty"`      // 每日抽奖可参与结束时间
	OpenTime     string `json:"open_time,omitempty"`     // 每日抽奖开奖时间
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
	ID            int            `json:"id"`
	Name          string         `json:"name"`
	Image         string         `json:"image"`
	Got           int            `json:"got"`      // 已集碎片数（汇总）
	Need          int            `json:"need"`     // 所需碎片数（汇总）
	Fragments     []SynthFragment `json:"fragments"` // 各碎片持有/所需
	DrawPrice     int            `json:"draw_price"`
	TenPrice      int            `json:"ten_price"`
	HundredPrice  int            `json:"hundred_price"`
	CanSynthesize bool           `json:"can_synthesize"`
	Pity          string         `json:"pity"` // 保底进度文本
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

// MedalNode 六十四卦勋章树节点（LongPT：太极→两仪→四象→八卦→六十四卦 金字塔）。
type MedalNode struct {
	MedalID  string `json:"medal_id"`  // 二进制串（卦象："1"=阳爻 "0"=阴爻）；位数=该层爻数
	ParentID string `json:"parent_id"`
	Type     string `json:"type"`      // taiji/liangyi/sixiang/bagua/liushisi
	Name     string `json:"name"`
	Level    int    `json:"level"`     // 1-5
	ImageURL string `json:"image_url"`
	Owned    bool   `json:"owned"`
	NeedYang int    `json:"need_yang"` // 碎片合成所需阳爻数（由 medalId 二进制位数算）
	NeedYin  int    `json:"need_yin"`  // 碎片合成所需阴爻数
	BigCost  int    `json:"big_cost"`  // 大勋章合成魔力消耗（碎片合成为 0）
}

// MedalShardInfo 阳爻 / 阴爻碎片余额。
type MedalShardInfo struct {
	YangYao int `json:"yang_yao"`
	YinYao  int `json:"yin_yao"`
}

// MedalComposable 六十四卦勋章合成能力（LongPT 插件系统）：列勋章树 / 查碎片余额 /
// 碎片合成（太极·两仪·四象）/ 大勋章合成（八卦·六十四卦，耗魔力+子勋章）/ 阴阳互转。
type MedalComposable interface {
	MedalTree(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]MedalNode, error)
	MedalShards(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*MedalShardInfo, error)
	ComposeMedal(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, medalID string) (*PlayResult, error)
	ComposeBigMedal(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, medalID string) (*PlayResult, error)
	ConvertShard(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, yiYang int) (*PlayResult, error)
}

// Synthesizable 勋章合成能力：列目标 / 抽碎片（1/10/100）/ 合成（对齐 Kotlin MedalSynthesisGame）。
type Synthesizable interface {
	Syntheses(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]SynthesisTarget, error)
	Draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, synthesisID, count int) (*PlayResult, error)
	Synthesize(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, synthesisID int) (*PlayResult, error)
}

// FarmItem 农场单项（NovaHD 魔力农场：农作物 / 动物）。基础配置源码写死，status 由页面渲染推断。
type FarmItem struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Icon       string  `json:"icon,omitempty"`
	Section    string  `json:"section"`               // crop | animal
	GrowHours  int     `json:"grow_hours"`            // 成长时间（小时）
	Price      float64 `json:"price"`                 // 买入价（魔力值）
	ExpireDays int     `json:"expire_days,omitempty"` // 农作物 2 天 / 动物 5 天
	Status     string  `json:"status"`                // idle 未种 | growing 生长中 | ready 可收获
	RemainTime string  `json:"remain_time,omitempty"` // 生长中剩余时间文本
}

// FarmState 农场全景：余额 + 种植区 + 养殖区 + 仓库 + 菜市场（magic_fram.php 同页渲染）。
type FarmState struct {
	Balance   float64       `json:"balance"`
	FarmName  string        `json:"farm_name,omitempty"`
	Crops     []FarmItem    `json:"crops"`
	Animals   []FarmItem    `json:"animals"`
	Warehouse []StorageItem `json:"warehouse,omitempty"`
	Market    []MarketItem  `json:"market,omitempty"`
}

// StorageItem 仓库物品（含出售 key 与市场价）。
type StorageItem struct {
	Key         string  `json:"key"` // 出售用 ?action=sell&key=
	Name        string  `json:"name"`
	Icon        string  `json:"icon,omitempty"`
	Count       int     `json:"count"`
	MarketPrice float64 `json:"market_price,omitempty"`
}

// MarketItem 菜市场行情（24h ±20% 波动）。
type MarketItem struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Section     string  `json:"section"` // crop | animal
	MarketPrice float64 `json:"market_price"`
}

// Farmable 魔力农场能力（NovaHD magic_fram.php，GET + 整页 HTML，无 JSON API）。
// section 决定种植动作：crop→action=plant，animal→action=breed。
type Farmable interface {
	Farm(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FarmState, error)
	Plant(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error)
	Harvest(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, section string, itemID int) (*PlayResult, error)
	HarvestAll(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error)
	Sell(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, key string) (*PlayResult, error)
}

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

// Registry 按站点 host 注册游戏。
type Registry struct {
	games map[string]map[string]LotteryGame // host -> gameID -> game
}

// NewRegistry 构造。
func NewRegistry() *Registry {
	return &Registry{games: map[string]map[string]LotteryGame{}}
}

// Register 为某 host 注册一个游戏。
func (r *Registry) Register(host string, g LotteryGame) {
	h := normalizeHost(host)
	if r.games[h] == nil {
		r.games[h] = map[string]LotteryGame{}
	}
	r.games[h][g.ID()] = g
}

// Games 返回某 host 的全部游戏。
// host 匹配放宽：站点 base_url 常带 www（www.hxpt.org）而注册用裸域（hxpt.org），
// 按后缀匹配视为同一站点，避免「娱乐页空白」。
func (r *Registry) Games(host string) []LotteryGame {
	site := normalizeHost(host)
	out := make([]LotteryGame, 0)
	for regHost, games := range r.games {
		if hostMatch(site, regHost) {
			for _, g := range games {
				out = append(out, g)
			}
		}
	}
	return out
}

// Game 取某 host 的指定游戏（同样按后缀匹配）。
func (r *Registry) Game(host, gameID string) (LotteryGame, bool) {
	site := normalizeHost(host)
	for regHost, games := range r.games {
		if hostMatch(site, regHost) {
			if g, ok := games[gameID]; ok {
				return g, true
			}
		}
	}
	return nil, false
}

// hostMatch 报告 siteHost 是否对应 regHost：精确相等，或 siteHost 是 regHost 的子域
// （如 www.hxpt.org ↔ hxpt.org）。
func hostMatch(siteHost, regHost string) bool {
	if siteHost == regHost {
		return true
	}
	return strings.HasSuffix(siteHost, "."+regHost)
}

// Hosts 返回已注册的全部 host。
func (r *Registry) Hosts() []string {
	out := make([]string, 0, len(r.games))
	for h := range r.games {
		out = append(out, h)
	}
	return out
}

// normalizeHost：去 scheme/path，小写，去端口。
func normalizeHost(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, ':'); i >= 0 {
		s = s[:i] // 去端口
	}
	return strings.ToLower(s)
}
