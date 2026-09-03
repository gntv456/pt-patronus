package entertainment

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// MedalNode 六十四卦勋章树节点（LongPT：太极→两仪→四象→八卦→六十四卦 金字塔）。
type MedalNode struct {
	MedalID  string `json:"medal_id"` // 二进制串（卦象："1"=阳爻 "0"=阴爻）；位数=该层爻数
	ParentID string `json:"parent_id"`
	Type     string `json:"type"` // taiji/liangyi/sixiang/bagua/liushisi
	Name     string `json:"name"`
	Level    int    `json:"level"` // 1-5
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
	Reward     float64 `json:"reward,omitempty"`      // 基础产出（思齐 base_reward）
	ExpireDays int     `json:"expire_days,omitempty"` // 农作物 2 天 / 动物 5 天
	Status     string  `json:"status"`                // idle 未种 | growing 生长中 | ready 可收获
	// Locked/UnlockText：思齐种子按累计收获解锁（unlock_harvest 门槛），未达显示还差多少。
	Locked     bool   `json:"locked,omitempty"`
	UnlockText string `json:"unlock_text,omitempty"`
	RemainTime string `json:"remain_time,omitempty"` // 生长中剩余时间文本
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
