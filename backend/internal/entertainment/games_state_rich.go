package entertainment

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 富状态端点共用类型（批8）：好友买卖 / 钓鱼 ──────────────────
// 模式同 GobangState：游戏实现可选接口，chain/api 透传，前端富组件渲染。

// FriendTradeSlave 我持有的奴隶（my_assets 项）。
type FriendTradeSlave struct {
	UID      string `json:"uid"`
	Username string `json:"username"`
	Avatar   string `json:"avatar,omitempty"`
	// 价格与工作
	Price        string `json:"price,omitempty"`         // 当前身价（acquire_price）
	WorkLabel    string `json:"work_label,omitempty"`    // 今日安排（work_label）
	IncomeAmount string `json:"income_amount,omitempty"` // 今日产出
	HasWork      bool   `json:"has_work"`
	// 培养选项（cultivation_summary：安排工作/互动的下拉数据源，批17）
	WorkOptions        []FriendTradeOption `json:"work_options,omitempty"`
	InteractionOptions []FriendTradeOption `json:"interaction_options,omitempty"`
}

// FriendTradeOption 工作/互动选项（cultivation_summary.work_options / interaction_options 项）。
type FriendTradeOption struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Stage       string `json:"stage,omitempty"`      // 工作阶段（early/mid/late…）
	Income      string `json:"income,omitempty"`    // 预期产出
	Unlocked    bool   `json:"unlocked"`            // 是否解锁（属性门槛）
	UnlockText  string `json:"unlock_text,omitempty"` // 未解锁原因
}

// FriendTradeTask 新手/成就任务（task_status 项）。
type FriendTradeTask struct {
	Key          string `json:"key"`
	Label        string `json:"label,omitempty"` // 中文描述（站点未给则前端兜底）
	Progress     int    `json:"progress"`
	Target       int    `json:"target"`
	Status       string `json:"status"` // pending | completed | claimed
	RewardAmount string `json:"reward_amount,omitempty"`
}

// FriendTradeFeed 动态一条（activity_feed / my_feed 项）。
type FriendTradeFeed struct {
	Text      string `json:"text"`
	CreatedAt string `json:"created_at,omitempty"`
}

// FriendTradeState 好友买卖全景快照（PANDA friendTradeInitOrHome data 聚合）。
type FriendTradeState struct {
	MeUID     string `json:"me_uid"`
	MeName    string `json:"me_name"`
	Balance   string `json:"balance,omitempty"` // 站点魔力（页面 mybonus，非该接口字段则空）
	Currency  string `json:"currency,omitempty"`
	// 身价与归属
	ProfilePrice string `json:"profile_price"`
	OwnerName    string `json:"owner_name,omitempty"` // 空=自由身
	OwnerUID     string `json:"owner_uid,omitempty"`
	RedeemPrice  string `json:"redeem_price,omitempty"`
	CanRedeem    bool   `json:"can_redeem"`
	// 资产
	Assets          []FriendTradeSlave `json:"assets"`
	AssetTotalValue string             `json:"asset_total_value,omitempty"`
	ClaimableIncome string             `json:"claimable_income"`
	// 容量
	BaseCapacity  int    `json:"base_capacity,omitempty"`
	OwnedCount    int    `json:"owned_count"`
	NextSlotPrice string `json:"next_slot_price,omitempty"`
	// 收益概览
	TodayEarned string `json:"today_earned,omitempty"`
	TotalEarned string `json:"total_earned,omitempty"`
	// 任务与动态
	Tasks   []FriendTradeTask `json:"tasks,omitempty"`
	Feed    []FriendTradeFeed `json:"feed,omitempty"`
	Message string            `json:"message,omitempty"`
}

// FriendTradeable 好友买卖能力：State 只读全景；动作走 Bettable（claim/redeem/protect）。
type FriendTradeable interface {
	FriendTradeState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FriendTradeState, error)
}

// FishingFish 图鉴一条（codex 项）。
type FishingFish struct {
	Code         int    `json:"code"`
	Name         string `json:"name"`
	Rarity       string `json:"rarity"`        // common/uncommon/rare/...
	RarityLabel  string `json:"rarity_label"`  // 普通/不常见/…
	TimeSlot     int    `json:"time_slot"`     // 出没时段（5=全天?）
	Discovered   bool   `json:"discovered"`
	Quantity     int    `json:"quantity"`
}

// FishingLure 鱼饵明细（config.lures + inventory.lures 持有量）。
type FishingLure struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Icon        string  `json:"icon,omitempty"`
	Price       float64 `json:"price"`              // 单价（魔力）
	Description string  `json:"description,omitempty"`
	Owned       int     `json:"owned"`              // 持有数量
	Selected    bool    `json:"selected"`           // 当前是否挂饵
}

// FishingRod 鱼竿明细（config.rods + inventory.rods 解锁态）。
type FishingRod struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Icon        string `json:"icon,omitempty"`
	Description string `json:"description,omitempty"`
	Unlocked    bool   `json:"unlocked"`
	UnlockHint  string `json:"unlock_hint,omitempty"` // 解锁条件（task N / shop）
	Selected    bool   `json:"selected"`
}

// FishingState 钓鱼全景快照（思齐 FISHING_DATA 聚合）。
type FishingState struct {
	Balance float64 `json:"balance"` // 魔力
	// 钓次
	AttemptsUsed  int `json:"attempts_used"`
	AttemptsLimit int `json:"attempts_limit"`
	CaughtToday   int `json:"caught_today"`
	// 装备
	SelectedRod  string        `json:"selected_rod,omitempty"`
	SelectedLure string        `json:"selected_lure,omitempty"`
	Rods         []FishingRod  `json:"rods,omitempty"`  // 全部鱼竿（含未解锁）
	Lures        []FishingLure `json:"lures,omitempty"` // 商店鱼饵（含持有量）
	// 图鉴
	DiscoveredCount int           `json:"discovered_count"`
	TotalSpecies    int           `json:"total_species"`
	Codex           []FishingFish `json:"codex"`
	Message         string        `json:"message,omitempty"`
}

// Fishingable 钓鱼能力：State 只读；抛竿走 MultiPlayable（PlayMulti）。
type Fishingable interface {
	FishingState(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*FishingState, error)
}
