// Package medalwall 实现「勋章墙」抓取：各平台勋章商店 + 已拥有勋章聚合，支持购买/佩戴/摘下。
// per-platform 适配器 + Registry 分发（NexusPHP HTML 双布局 / MTeam 占位 / TNode / Unit3D JSON）。
// 独立子包（仿 site/assessment），避免膨胀 schemas；由 chain.MedalChain 调用。
package medalwall

import (
	"context"
	"sync"
)

// Status 勋章购买/库存状态（对齐旧版 Kotlin MedalPurchaseStatus）。
type Status string

const (
	StatusOwned        Status = "OWNED"         // 已拥有
	StatusAvailable    Status = "AVAILABLE"     // 可购买
	StatusOutOfStock   Status = "OUT_OF_STOCK"  // 缺货
	StatusNotOnSale    Status = "NOT_ON_SALE"   // 未开售（时间未到）
	StatusGrantOnly    Status = "GRANT_ONLY"    // 仅授予（站点授予，不可购买）
	StatusSynthesis    Status = "SYNTHESIS"     // 仅合成（LongPT 六十四卦等）
	StatusDiscontinued Status = "DISCONTINUED"  // 已下架/过期
	StatusUnknown      Status = "UNKNOWN"
)

// Item 商店勋章（抓取 + 合并 + 持久化用）。字段对齐 models.Medal。
type Item struct {
	MedalID          string  // 站点内业务主键
	Name             string
	Description      string
	ImageSmall       string  // 列表小图
	OriginalImageURL string  // 详情大图
	SaleBeginTime    *int64  // epoch ms（nil=未知）
	SaleEndTime      *int64
	Price            float64
	OriginalPrice    float64 // 原价（无溢价=Price；有溢价<Price，前端划线展示）
	Validity         string  // "30天"/"永久"
	BonusRate        string  // "+20%"
	PurchaseStatus   Status
	GiftStatus       string
	Stock            int // -1=无限/未知
	StockStatus      string
	Currency         string // 魔力/电力/金币/积分/BP
	Group            string
	WearStatus       bool
	BuyURL           string // 抠到的购买 href/onbuy 线索（best-effort）
}

// Owned 已拥有勋章（来自个人页 userdetails.php?id=<uid>）。
type Owned struct {
	MedalID string
	Name    string
	Image   string
	Wearing bool
}

// Def 勋章墙所需的最小站点定义（从 site.SiteMetadata.Medal 投影，避免反向依赖 site 包）。
type Def struct {
	Schema   string
	ShopPath string // 默认 medal.php
	OwnPath  string // 默认 userdetails.php
}

// ActionResult 写操作（购买/佩戴/摘下）结果。
type ActionResult struct {
	Success bool
	Message string
}

// HTTPDoer 勋章墙适配器需要的 HTTP 子接口（httpclient.Client 已实现，由 chain 层适配）。
type HTTPDoer interface {
	Get(ctx context.Context, rawURL, cookieStr string, ssrf bool) (*Response, error)
	Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string, cookie string, ssrf bool) (*Response, error)
}

// Response HTTP 响应最小子集（由调用方从 httpclient.Response 适配构造）。
type Response struct {
	StatusCode int
	Body       []byte
	FinalURL   string
}

// Adapter per-platform 勋章墙适配器契约。
type Adapter interface {
	Platform() string
	Supported() bool
	// FetchShop 抓商店勋章（分页在实现内处理，防伪分页）。
	FetchShop(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie string) ([]Item, error)
	// FetchOwned 抓已拥有勋章；uid 为空时返回空（chain 层负责发现 uid）。
	FetchOwned(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, uid string) ([]Owned, error)
	Buy(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error)
	Wear(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error)
	TakeOff(ctx context.Context, hc HTTPDoer, def Def, baseURL, cookie, medalID string) (ActionResult, error)
}

// Registry 按 platform（小写）分发勋章墙适配器；未注册返回 NoOp（Supported=false）。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry 构造空 Registry。
func NewRegistry() *Registry { return &Registry{adapters: map[string]Adapter{}} }

// Register 注册一个适配器（按 Platform() 小写键，同 key 覆盖）。
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Platform()] = a
}

// Get 取适配器；未注册返回 NoOp（Supported=false）。
func (r *Registry) Get(platform string) Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if a, ok := r.adapters[platform]; ok {
		return a
	}
	return NoOpAdapter{platform: platform}
}

// NoOpAdapter 未支持平台的占位。
type NoOpAdapter struct{ platform string }

func (n NoOpAdapter) Platform() string { return n.platform }
func (n NoOpAdapter) Supported() bool  { return false }

func (n NoOpAdapter) FetchShop(context.Context, HTTPDoer, Def, string, string) ([]Item, error) {
	return nil, nil
}
func (n NoOpAdapter) FetchOwned(context.Context, HTTPDoer, Def, string, string, string) ([]Owned, error) {
	return nil, nil
}
func (n NoOpAdapter) Buy(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "该平台不支持勋章购买"}, nil
}
func (n NoOpAdapter) Wear(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "该平台不支持勋章佩戴"}, nil
}
func (n NoOpAdapter) TakeOff(context.Context, HTTPDoer, Def, string, string, string) (ActionResult, error) {
	return ActionResult{Success: false, Message: "该平台不支持勋章摘下"}, nil
}
