package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"golang.org/x/sync/singleflight"
)

// 勋章墙缓存参数（对齐旧版：内存 24h、DB 48h）。
const (
	medalMemTTL      = 24 * time.Hour
	medalDBTTL       = 48 * time.Hour
	medalTimeout     = 30 * time.Second
	medalConcurrency = 5
)

// medalHTTP 把 httpclient.Client 适配为 medalwall.HTTPDoer。
type medalHTTP struct{ c *httpclient.Client }

func (w medalHTTP) Get(ctx context.Context, rawURL, cookie string, ssrf bool) (*medalwall.Response, error) {
	r, err := w.c.Get(ctx, rawURL, cookie, ssrf)
	if err != nil {
		return nil, err
	}
	return &medalwall.Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}

func (w medalHTTP) Do(ctx context.Context, method, rawURL string, body []byte, headers map[string]string, cookie string, ssrf bool) (*medalwall.Response, error) {
	r, err := w.c.Do(ctx, method, rawURL, body, headers, cookie, ssrf)
	if err != nil {
		return nil, err
	}
	return &medalwall.Response{StatusCode: r.StatusCode, Body: r.Body, FinalURL: r.FinalURL}, nil
}

// medalCacheEntry 内存缓存条目。
type medalCacheEntry struct {
	items []medalwall.Item
	at    time.Time
	err   string
}

// MedalCard 勋章墙 UI 卡片（API 返回）。
type MedalCard struct {
	SiteID           uint    `json:"site_id"`
	SiteName         string  `json:"site_name"`
	MedalID          string  `json:"medal_id"`
	Name             string  `json:"name"`
	Description      string  `json:"description,omitempty"`
	ImageSmall       string  `json:"image_small"`
	OriginalImageURL string  `json:"original_image_url,omitempty"`
	SaleBeginTime    *int64  `json:"sale_begin_time"` // epoch ms（nil=未知）
	SaleEndTime      *int64  `json:"sale_end_time"`
	Price            float64 `json:"price"`
	OriginalPrice    float64 `json:"original_price"`       // 原价（溢价时<Price，前端划线展示）
	Validity         string  `json:"validity,omitempty"`   // "30天"/"永久"
	BonusRate        string  `json:"bonus_rate,omitempty"` // "+20%"
	PurchaseStatus   string  `json:"purchase_status"`      // OWNED/AVAILABLE/OUT_OF_STOCK/NOT_ON_SALE/DISCONTINUED/UNKNOWN
	Stock            int     `json:"stock"`                // -1=无限/未知
	Currency         string  `json:"currency"`
	Group            string  `json:"group,omitempty"`
	WearStatus       bool    `json:"wear_status"`
	IsOwned          bool    `json:"is_owned"`
	Actionable       bool    `json:"actionable"`
	Error            string  `json:"error,omitempty"`
}

// MedalChain 编排站点勋章墙（3 级缓存 内存→DB→网络 + per-platform 适配 + 商店/已拥有合并）。
type MedalChain struct {
	siteCh *SiteChain
	reg    *medalwall.Registry
	op     *oper.MedalOper
	hc     medalwall.HTTPDoer
	notice *notice.Service
	mu     sync.Mutex
	cache  map[uint]medalCacheEntry
	sf     singleflight.Group // 同 siteID 勋章墙并发抓取合并（防击穿）
	guard  *spendGuard        // 买/佩戴 in-flight 去重（防双击重复扣站点魔力）
}

// NewMedalChain 构造。ns 为站内通知中心（价格提醒 P6 用；nil 跳过）。
func NewMedalChain(siteCh *SiteChain, reg *medalwall.Registry, op *oper.MedalOper, hc *httpclient.Client, ns *notice.Service) *MedalChain {
	return &MedalChain{siteCh: siteCh, reg: reg, op: op, hc: medalHTTP{c: hc}, notice: ns, cache: map[uint]medalCacheEntry{}, guard: newSpendGuard()}
}

// MedalAll 跨站并发聚合勋章墙（按站点顺序输出）。
func (c *MedalChain) MedalAll(ctx context.Context, force bool) []MedalCard {
	sites, err := c.siteCh.sites.ListActive()
	if err != nil {
		slog.Warn("medal list sites failed", "err", err)
		return []MedalCard{}
	}
	type siteRes struct {
		items     []medalwall.Item
		err       string
		supported bool
	}
	results := make([]siteRes, len(sites))
	sem := make(chan struct{}, medalConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("medal:121", func() {
			defer wg.Done()
			def := c.siteCh.defFor(&s)
			slog.Debug("medal site check", "site", s.Name, "enabled", def.Medal.Enabled, "schema", def.Schema, "shop", def.Medal.ShopPath)
			if !def.Medal.Enabled {
				return // 该站未开启勋章墙抓取
			}
			if !c.reg.Get(def.Schema).Supported() {
				return
			}
			items, errMsg := c.medalSite(ctx, sem, &s, force)
			slog.Debug("medal site result", "site", s.Name, "items", len(items), "err", errMsg)
			if len(items) == 0 && errMsg == "" {
				errMsg = "未发现勋章数据（站点可能未开放勋章功能或页面结构待适配）"
			}
			results[i] = siteRes{items: items, err: errMsg, supported: true}
		})
	}
	wg.Wait()

	out := make([]MedalCard, 0)
	for i, s := range sites {
		r := results[i]
		for _, it := range r.items {
			out = append(out, itemToCard(s.ID, s.Name, it))
		}
		if r.supported && r.err != "" {
			out = append(out, MedalCard{SiteID: s.ID, SiteName: s.Name, Error: r.err})
		}
	}
	return out
}

// MedalAllStream 流式推送跨站勋章墙：每站完成即 emit（成功站多张卡，失败站一张错误占位卡）。
// 供 SSE 端点「加载一个显示一个」。与 MedalAll 不同：不按站序输出（按到达顺序）。
// medalSite 内部已有 3 级缓存 + per-site singleflight（24h/48h），缓存命中时逐站瞬时返回。
// 注意：emit 会被多个站点 goroutine 并发调用，调用方（handler）须自行串行化写入。
func (c *MedalChain) MedalAllStream(ctx context.Context, force bool, emit func(cards []MedalCard)) {
	sites, err := c.siteCh.sites.ListActive()
	if err != nil {
		slog.Warn("medal list sites failed", "err", err)
		return
	}
	sem := make(chan struct{}, medalConcurrency)
	var wg sync.WaitGroup
	for _, s := range sites {
		wg.Add(1)
		safego.Go("medal:168", func() {
			defer wg.Done()
			def := c.siteCh.defFor(&s)
			if !def.Medal.Enabled {
				return // 该站未开启勋章墙抓取
			}
			if !c.reg.Get(def.Schema).Supported() {
				return
			}
			items, errMsg := c.medalSite(ctx, sem, &s, force)
			if len(items) == 0 && errMsg == "" {
				errMsg = "未发现勋章数据（站点可能未开放勋章功能或页面结构待适配）"
			}
			cards := make([]MedalCard, 0, len(items))
			for _, it := range items {
				cards = append(cards, itemToCard(s.ID, s.Name, it))
			}
			if errMsg != "" {
				cards = append(cards, MedalCard{SiteID: s.ID, SiteName: s.Name, Error: errMsg})
			}
			if len(cards) > 0 {
				emit(cards)
			}
		})
	}
	wg.Wait()
}

// MedalOne 单站强制网络刷新（写操作后调用；单站不限并发，sem=nil）。
func (c *MedalChain) MedalOne(ctx context.Context, s *models.Site) []MedalCard {
	items, _ := c.medalSite(ctx, nil, s, true)
	out := make([]MedalCard, 0, len(items))
	for _, it := range items {
		out = append(out, itemToCard(s.ID, s.Name, it))
	}
	return out
}

// medalSFResult 是勋章墙单次回源的 singleflight 结果（合并后勋章 + 展示用 errMsg）。
type medalSFResult struct {
	items  []medalwall.Item
	errMsg string
}

// medalSite 单站 3 级缓存：内存→DB→网络。返回（合并后勋章, 错误消息）。sem 为聚合级并发闸
// （nil 表示不限，单站用）。force=true 绕过缓存与 singleflight 直接网络回源；否则 miss 时
// singleflight 合并同 siteID 并发回源（防击穿，sem 仅 leader 占用）。包整个回源——避免 waiter
// 各自 BatchUpsert + 重复发通知（dispatchMedalChanges）。
func (c *MedalChain) medalSite(ctx context.Context, sem chan struct{}, s *models.Site, force bool) ([]medalwall.Item, string) {
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return nil, "" // 该平台不支持勋章墙（如 MTeam 占位），静默
	}
	if force {
		return c.fetchMedalSite(ctx, sem, s, def, adapter)
	}
	// 快速路径：内存命中直接返回（不经 singleflight）。
	c.mu.Lock()
	e, ok := c.cache[s.ID]
	c.mu.Unlock()
	if ok && time.Since(e.at) <= medalMemTTL {
		return e.items, e.err
	}
	// miss → singleflight 合并同 siteID 并发回源。
	res, waitErr := sfDoContext(ctx, &c.sf, strconv.Itoa(int(s.ID)), func() (medalSFResult, error) {
		// double-check 内存：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
		c.mu.Lock()
		ce, ok := c.cache[s.ID]
		c.mu.Unlock()
		if ok && time.Since(ce.at) <= medalMemTTL {
			return medalSFResult{items: ce.items, errMsg: ce.err}, nil
		}
		// double-check DB（48h）：另一调用可能在 leader 期间刚写 DB。
		if latest, err := c.op.LatestFetchedAt(s.ID); err == nil && !latest.IsZero() && time.Since(latest) <= medalDBTTL {
			items := medalsToItems(mustFindBySite(c.op, s.ID))
			c.mu.Lock()
			c.cache[s.ID] = medalCacheEntry{items: items, at: time.Now()}
			c.mu.Unlock()
			return medalSFResult{items: items}, nil
		}
		items, errMsg := c.fetchMedalSite(ctx, sem, s, def, adapter)
		return medalSFResult{items: items, errMsg: errMsg}, nil
	})
	if waitErr != nil {
		return nil, waitErr.Error()
	}
	return res.items, res.errMsg
}

// fetchMedalSite 网络抓取勋章墙（FetchShop + discoverUID + FetchOwned）→ 合并 → BatchUpsert →
// 价格变动通知 → 内存缓存。sem 非空时占一个槽（聚合级并发闸；nil 表示不限，单站用）。
func (c *MedalChain) fetchMedalSite(ctx context.Context, sem chan struct{}, s *models.Site, def *site.SiteMetadata, adapter medalwall.Adapter) ([]medalwall.Item, string) {
	if err := acquireSemaphore(ctx, sem); err != nil {
		return nil, err.Error()
	}
	if sem != nil {
		defer func() { <-sem }()
	}
	siteCtx, cancel := context.WithTimeout(ctx, medalTimeout)
	defer cancel()
	cookie := c.siteCh.credential(s, def)
	shop, shopErr := adapter.FetchShop(siteCtx, c.hc, medalDef(def), s.BaseURL, cookie)
	if shopErr != nil {
		if errors.Is(shopErr, site.ErrNotLoggedIn) {
			slog.Warn("medal site not logged in", "site", s.Name)
		} else {
			slog.Warn("medal site shop fetch error", "site", s.Name, "err", shopErr)
		}
	}
	uid := c.discoverUID(siteCtx, s, def, cookie)
	owned, ownedErr := adapter.FetchOwned(siteCtx, c.hc, medalDef(def), s.BaseURL, cookie, uid)
	if ownedErr != nil {
		slog.Warn("medal site owned fetch error", "site", s.Name, "err", ownedErr)
	}
	wearing := 0
	for _, o := range owned {
		if o.Wearing {
			wearing++
		}
	}
	slog.Debug("medal owned", "site", s.Name, "uid", uid, "owned", len(owned), "wearing", wearing)
	merged := mergeMedals(shop, owned)
	merged = ensureMedalIDs(merged)
	errMsg := ""
	if shopErr != nil {
		errMsg = shopErr.Error()
	} else if len(merged) == 0 {
		errMsg = "未发现勋章数据（站点可能未开放勋章功能或页面结构待适配）"
	}

	// 读旧（BatchUpsert 是删后插，旧数据会被清，必须先读）→ diff → 通知
	oldItems := medalsToItems(mustFindBySite(c.op, s.ID))
	// 商店失败时保留完整商店快照，避免一次网络/结构故障清空可用数据。
	// 但旧版本可能把个人页误扫结果以 owned:<name> 写成“仅拥有快照”；若本次仍只能拿到
	// owned-only 数据，应允许用 UID 限定后的结果替换它，避免错误佩戴状态在 DB TTL 内复活。
	ownedOnlyRepair := shopErr != nil && len(merged) > 0 && ownedOnlyMedalSnapshot(oldItems) && ownedOnlyMedalSnapshot(merged)
	if (shopErr == nil && len(merged) > 0) || ownedOnlyRepair {
		if err := c.op.BatchUpsert(s.ID, itemsToMedals(merged), time.Now()); err != nil {
			slog.Warn("medal BatchUpsert failed, results not persisted", "site", s.ID, "err", err)
		}
	}
	// 首次无旧记录跳过（防初始化刷屏）
	if shopErr == nil && c.notice != nil && len(oldItems) > 0 {
		c.dispatchMedalChanges(ctx, s, oldItems, merged)
	}
	if shopErr == nil && len(merged) > 0 {
		c.mu.Lock()
		c.cache[s.ID] = medalCacheEntry{items: merged, at: time.Now(), err: errMsg}
		c.mu.Unlock()
	}
	return merged, errMsg
}

func ownedOnlyMedalSnapshot(items []medalwall.Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !strings.HasPrefix(strings.TrimSpace(item.MedalID), "owned:") || item.PurchaseStatus != medalwall.StatusOwned {
			return false
		}
	}
	return true
}

// dispatchMedalChanges 对比新旧勋章，对新勋章/上架/降价/涨价/售罄变动发通知（当天去重）。
func (c *MedalChain) dispatchMedalChanges(ctx context.Context, s *models.Site, old, cur []medalwall.Item) {
	oldByID := map[string]medalwall.Item{}
	for _, o := range old {
		oldByID[medalKey(o)] = o
	}
	for _, n := range cur {
		o, hadOld := oldByID[medalKey(n)]
		if !hadOld {
			if n.PurchaseStatus == medalwall.StatusAvailable {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalNew, Key: fmt.Sprintf("medal:%d:%s:new", s.ID, n.MedalID),
					Title: "新勋章：" + s.Name, Body: n.Name + " 可购买（" + fmtMedalPrice(n) + "）", Level: "info",
				}, true)
			}
			continue
		}
		// 价格变动
		if o.Price > 0 && n.Price > 0 && o.Price != n.Price {
			if n.Price < o.Price {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceDrop, Key: fmt.Sprintf("medal:%d:%s:drop", s.ID, n.MedalID),
					Title: "勋章降价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "success",
				}, true)
			} else {
				_ = c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceRise, Key: fmt.Sprintf("medal:%d:%s:rise", s.ID, n.MedalID),
					Title: "勋章涨价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "warning",
				}, true)
			}
		}
		// 上架可购买
		if o.PurchaseStatus != medalwall.StatusAvailable && n.PurchaseStatus == medalwall.StatusAvailable {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalAvailable, Key: fmt.Sprintf("medal:%d:%s:avail", s.ID, n.MedalID),
				Title: "勋章上架：" + s.Name, Body: n.Name + " 现可购买", Level: "info",
			}, true)
		}
		// 售罄
		if !medalOOS(o) && medalOOS(n) {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalOutOfStock, Key: fmt.Sprintf("medal:%d:%s:oos", s.ID, n.MedalID),
				Title: "勋章售罄：" + s.Name, Body: n.Name + " 已缺货", Level: "danger",
			}, true)
		}
	}
}

// medalKey 勋章去重键：优先 medalId，无则归一名。
