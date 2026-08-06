package entertainment

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 猜大小（Big-or-Small）统一快照（好学 hxpt + 熊猫 pandapt 两站共用） ────
// 两站 API 差异由各自适配器在 BigSmallState() 内抹平：好学 games/bigsmall/ajax.php 多个 GET 聚合；
// 熊猫 /ajax.php reloadData 一次拿大头 + 任务/道具/红包/聊天轻取。时间靠客户端：本快照含权威
// countdown/seal_countdown/status，前端按 fetchedAt 本地递减（见 BigSmallGame.vue 注释）。

// BigSmallRound 当前一局信息。
type BigSmallRound struct {
	ID            int     `json:"id"`
	GameNo        string  `json:"game_no"`
	StartTime     string  `json:"start_time,omitempty"`
	EndTime       string  `json:"end_time,omitempty"`
	Status        string  `json:"status"`         // betting | sealed | finished
	Countdown     int     `json:"countdown"`      // 到本轮结束剩余秒（权威）
	SealCountdown int     `json:"seal_countdown"` // 到封盘剩余秒（≤0 禁止下注）
	PlayerCount   int     `json:"player_count"`
	TotalBet      float64 `json:"total_bet"`
}

// BigSmallBetSpot 当前局某方下注用户（押大/押小列表）。
type BigSmallBetSpot struct {
	Username string  `json:"username"`
	Amount   float64 `json:"amount"`
}

// BigSmallMyBet 我的下注 / 上局结算（Result/NetResult 仅 last_result 有意义）。
type BigSmallMyBet struct {
	GameNo    string  `json:"game_no,omitempty"`
	BetType   string  `json:"bet_type"` // big | small
	Amount    float64 `json:"amount"`
	Result    string  `json:"result,omitempty"`     // 上局开奖 big|small|house
	NetResult float64 `json:"net_result,omitempty"` // 上局输赢（正赢负输）
}

// BigSmallTrend 走势统计（40场 / 最近10场）。
type BigSmallTrend struct {
	BigCount   int `json:"big_count"`
	SmallCount int `json:"small_count"`
	HouseCount int `json:"house_count,omitempty"` // 通杀（好学有，熊猫无）
	BigPct     int `json:"big_pct"`
	SmallPct   int `json:"small_pct"`
}

// BigSmallRankEntry 排行榜单条。
type BigSmallRankEntry struct {
	Username string  `json:"username"`
	Amount   float64 `json:"amount"` // 赢/输/净值（按榜单语义）
	Count    int     `json:"count"`  // 赌王/慈善/参与次数
}

// BigSmallRank 一个榜单 tab（本月赌神/今日赌王/本月慈善/今日慈善…）。
type BigSmallRank struct {
	Key     string              `json:"key"` // monthly_king / daily_king / monthly_charity / daily_charity …
	Title   string              `json:"title"`
	Entries []BigSmallRankEntry `json:"entries"`
}

// BigSmallRecord 一局历史记录（我的/所有人）。
type BigSmallRecord struct {
	GameNo      string  `json:"game_no"`
	StartTime   string  `json:"start_time,omitempty"`
	EndTime     string  `json:"end_time,omitempty"`
	PlayerCount int     `json:"player_count,omitempty"`
	TotalBet    float64 `json:"total_bet,omitempty"`
	Result      string  `json:"result"` // big | small | house
	MyBetType   string  `json:"my_bet_type,omitempty"`
	MyBet       float64 `json:"my_bet,omitempty"`
	MyResult    float64 `json:"my_result,omitempty"` // 我的输赢（正赢负输）
	Bets        string  `json:"bets,omitempty"`      // 所有人记录：下注摘要 "大：1000"
}

// BigSmallTask 任务栏单项。
type BigSmallTask struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Desc         string `json:"desc,omitempty"`
	Reward       string `json:"reward,omitempty"`
	ProgressText string `json:"progress_text,omitempty"`
	Progress     int    `json:"progress"`
	Status       string `json:"status"` // idle | doing | rewardable | done
	MonthlyCount int    `json:"monthly_count,omitempty"`
	MonthlyLimit int    `json:"monthly_limit,omitempty"`
}

// BigSmallProp 道具单项（好学随注 prop_id；熊猫独立 useItem）。
type BigSmallProp struct {
	ID       string `json:"id"`
	Tag      string `json:"tag"` // code/unique_tag（下单或使用用）
	Name     string `json:"name"`
	Desc     string `json:"desc,omitempty"`
	Image    string `json:"image,omitempty"`
	Quantity int    `json:"quantity"`
	Usable   bool   `json:"usable"` // 是否可随注/使用
}

// BigSmallChat 聊天一条。
type BigSmallChat struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Message  string `json:"message"`
	Time     string `json:"time,omitempty"`
}

// BigSmallRedpacket 红包（活跃列表/详情）。
type BigSmallRedpacket struct {
	ID       string  `json:"id"`
	Sender   string  `json:"sender,omitempty"`
	Greeting string  `json:"greeting,omitempty"`
	Total    float64 `json:"total"`
	Count    int     `json:"count"`
	Claimed  int     `json:"claimed,omitempty"`
	MyClaim  float64 `json:"my_claim,omitempty"`
}

// BigSmallState 猜大小全景快照（聚合当前局/走势/排行/记录/任务/道具/聊天/红包）。
type BigSmallState struct {
	Balance  float64 `json:"balance"`
	MinBet   float64 `json:"min_bet"`
	MaxBet   float64 `json:"max_bet,omitempty"`
	Currency string  `json:"currency,omitempty"` // 火花 / 魔力值
	Payout   string  `json:"payout,omitempty"`   // "1.7" / "1.95"
	HasHouse bool    `json:"has_house"`          // 通杀（好学 true / 熊猫 false）

	Round      BigSmallRound     `json:"round"`
	MyBet      *BigSmallMyBet    `json:"my_bet,omitempty"`
	LastResult *BigSmallMyBet    `json:"last_result,omitempty"`
	BigBets    []BigSmallBetSpot `json:"big_bets,omitempty"`
	SmallBets  []BigSmallBetSpot `json:"small_bets,omitempty"`

	Trend40       BigSmallTrend `json:"trend40"`
	Trend10       BigSmallTrend `json:"trend10"`
	TrendText     string        `json:"trend_text,omitempty"`
	RecentResults []string      `json:"recent_results,omitempty"` // 走势点阵：big|small|house

	Rankings   []BigSmallRank   `json:"rankings,omitempty"`
	MyRecords  []BigSmallRecord `json:"my_records,omitempty"`
	AllRecords []BigSmallRecord `json:"all_records,omitempty"`

	Tasks      []BigSmallTask      `json:"tasks,omitempty"`
	Props      []BigSmallProp      `json:"props,omitempty"`
	Chat       []BigSmallChat      `json:"chat,omitempty"`
	Redpackets []BigSmallRedpacket `json:"redpackets,omitempty"`
}

// BigSmallable 猜大小能力（好学 hxpt_bigsmall + 熊猫 panda_bigsmall）。
// BigSmallState 取全景快照（只读）；其余为社交/任务/道具/红包写操作。下注仍走 Bettable.Bet。
type BigSmallable interface {
	BigSmallState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*BigSmallState, error)
	BigSmallChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, since int) ([]BigSmallChat, error)
	BigSmallSendChat(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, message string) (*PlayResult, error)
	// BigSmallTaskAction 任务操作：claim（领取/开始）/ reward（领奖）/ refund（好学亏返卡，taskID 忽略）。
	BigSmallTaskAction(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, taskID int) (*PlayResult, error)
	BigSmallUseItem(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, tag string) (*PlayResult, error)
	// BigSmallRedpacket 红包：action=send|claim|info，extra 传 amount/count/greeting/id 等。
	BigSmallRedpacket(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, action string, extra map[string]string) (*PlayResult, error)
}
