package chain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"

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
		items []medalwall.Item
		err   string
	}
	results := make([]siteRes, len(sites))
	sem := make(chan struct{}, medalConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		go func(i int, s models.Site) {
			defer wg.Done()
			def := c.siteCh.defFor(&s)
			slog.Debug("medal site check", "site", s.Name, "enabled", def.Medal.Enabled, "schema", def.Schema, "shop", def.Medal.ShopPath)
			if !def.Medal.Enabled {
				return // 该站未开启勋章墙抓取
			}
			items, errMsg := c.medalSite(ctx, sem, &s, force)
			slog.Debug("medal site result", "site", s.Name, "items", len(items), "err", errMsg)
			results[i] = siteRes{items: items, err: errMsg}
		}(i, s)
	}
	wg.Wait()

	out := make([]MedalCard, 0)
	for i, s := range sites {
		r := results[i]
		if len(r.items) == 0 && r.err != "" {
			// 失败站点：放一张占位错误卡，前端据此提示「某站加载失败/未登录」
			out = append(out, MedalCard{SiteID: s.ID, SiteName: s.Name, Error: r.err})
			continue
		}
		for _, it := range r.items {
			out = append(out, itemToCard(s.ID, s.Name, it))
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
		go func(s models.Site) {
			defer wg.Done()
			def := c.siteCh.defFor(&s)
			if !def.Medal.Enabled {
				return // 该站未开启勋章墙抓取
			}
			items, errMsg := c.medalSite(ctx, sem, &s, force)
			// 失败站：一张占位错误卡；成功站：items 展开成多张卡
			if len(items) == 0 && errMsg != "" {
				emit([]MedalCard{{SiteID: s.ID, SiteName: s.Name, Error: errMsg}})
				return
			}
			cards := make([]MedalCard, 0, len(items))
			for _, it := range items {
				cards = append(cards, itemToCard(s.ID, s.Name, it))
			}
			if len(cards) > 0 {
				emit(cards)
			}
		}(s)
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
		return e.items, ""
	}
	// miss → singleflight 合并同 siteID 并发回源。
	res, waitErr := sfDoContext(ctx, &c.sf, strconv.Itoa(int(s.ID)), func() (medalSFResult, error) {
		// double-check 内存：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
		c.mu.Lock()
		ce, ok := c.cache[s.ID]
		c.mu.Unlock()
		if ok && time.Since(ce.at) <= medalMemTTL {
			return medalSFResult{items: ce.items}, nil
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
	shop, err := adapter.FetchShop(siteCtx, c.hc, medalDef(def), s.BaseURL, cookie)
	if err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			slog.Warn("medal site not logged in", "site", s.Name)
			return nil, "未登录"
		}
		slog.Warn("medal site fetch error", "site", s.Name, "err", err)
		return nil, err.Error()
	}
	uid := c.discoverUID(siteCtx, s, def, cookie)
	owned, _ := adapter.FetchOwned(siteCtx, c.hc, medalDef(def), s.BaseURL, cookie, uid)
	wearing := 0
	for _, o := range owned {
		if o.Wearing {
			wearing++
		}
	}
	slog.Debug("medal owned", "site", s.Name, "uid", uid, "owned", len(owned), "wearing", wearing)
	merged := mergeMedals(shop, owned)

	// 读旧（BatchUpsert 是删后插，旧数据会被清，必须先读）→ diff → 通知
	oldItems := medalsToItems(mustFindBySite(c.op, s.ID))
	if len(merged) > 0 {
		if err := c.op.BatchUpsert(s.ID, itemsToMedals(merged), time.Now()); err != nil {
			slog.Warn("medal BatchUpsert failed, results not persisted", "site", s.ID, "err", err)
		}
	}
	// 首次无旧记录跳过（防初始化刷屏）
	if c.notice != nil && len(oldItems) > 0 {
		c.dispatchMedalChanges(ctx, s, oldItems, merged)
	}
	c.mu.Lock()
	c.cache[s.ID] = medalCacheEntry{items: merged, at: time.Now()}
	c.mu.Unlock()
	return merged, ""
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
				c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalNew, Key: fmt.Sprintf("medal:%d:%s:new", s.ID, n.MedalID),
					Title: "新勋章：" + s.Name, Body: n.Name + " 可购买（" + fmtMedalPrice(n) + "）", Level: "info",
				}, true)
			}
			continue
		}
		// 价格变动
		if o.Price > 0 && n.Price > 0 && o.Price != n.Price {
			if n.Price < o.Price {
				c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceDrop, Key: fmt.Sprintf("medal:%d:%s:drop", s.ID, n.MedalID),
					Title: "勋章降价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "success",
				}, true)
			} else {
				c.notice.Notice(ctx, notice.Input{
					Source: notice.SourceMedalPriceRise, Key: fmt.Sprintf("medal:%d:%s:rise", s.ID, n.MedalID),
					Title: "勋章涨价：" + s.Name, Body: fmt.Sprintf("%s %s → %s", n.Name, fmtMedalPrice(o), fmtMedalPrice(n)), Level: "warning",
				}, true)
			}
		}
		// 上架可购买
		if o.PurchaseStatus != medalwall.StatusAvailable && n.PurchaseStatus == medalwall.StatusAvailable {
			c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalAvailable, Key: fmt.Sprintf("medal:%d:%s:avail", s.ID, n.MedalID),
				Title: "勋章上架：" + s.Name, Body: n.Name + " 现可购买", Level: "info",
			}, true)
		}
		// 售罄
		if !medalOOS(o) && medalOOS(n) {
			c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceMedalOutOfStock, Key: fmt.Sprintf("medal:%d:%s:oos", s.ID, n.MedalID),
				Title: "勋章售罄：" + s.Name, Body: n.Name + " 已缺货", Level: "danger",
			}, true)
		}
	}
}

// medalKey 勋章去重键：优先 medalId，无则归一名。
func medalKey(it medalwall.Item) string {
	if it.MedalID != "" {
		return "id:" + it.MedalID
	}
	return "name:" + medalNormName(it.Name)
}

// medalOOS 是否缺货（状态 OUT_OF_STOCK 或库存 0）。
func medalOOS(it medalwall.Item) bool {
	return it.PurchaseStatus == medalwall.StatusOutOfStock || it.Stock == 0
}

// fmtMedalPrice 价格 + 货币展示。
func fmtMedalPrice(it medalwall.Item) string {
	return fmt.Sprintf("%.0f %s", it.Price, it.Currency)
}

// mustFindBySite 读 DB（忽略错误，返回空切片）。
func mustFindBySite(op *oper.MedalOper, siteID uint) []models.Medal {
	ms, _ := op.FindBySite(siteID)
	if ms == nil {
		return []models.Medal{}
	}
	return ms
}

// discoverUID 从 usercp.php 抠当前用户 uid（site.ExtractUIDAndName 扫 userdetails.php?id= 链接）。
func (c *MedalChain) discoverUID(ctx context.Context, s *models.Site, def *site.SiteMetadata, cookie string) string {
	path := def.UserInfo.Path
	if path == "" {
		path = "usercp.php"
	}
	resp, err := c.hc.Get(ctx, medalJoinBase(s.BaseURL, path), cookie, true)
	if err != nil || resp == nil || len(resp.Body) < 200 {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return ""
	}
	uid, _ := site.ExtractUIDAndName(doc)
	return uid
}

// mergeMedals 商店勋章与已拥有勋章合并：medalId 精确 → 名称精确 → 名称模糊（contains）三级匹配。
// 命中则置 PurchaseStatus=OWNED + 回填 WearStatus；未命中的 owned（商店无售但用户拥有/佩戴，
// 如等级勋章、成就勋章）追加到结果标 OWNED——这样「佩戴中」筛选能看到非商店来源的勋章。
func mergeMedals(shop []medalwall.Item, owned []medalwall.Owned) []medalwall.Item {
	if len(owned) == 0 {
		return shop
	}
	byID := map[string]int{}
	byName := map[string]int{}
	ownedNorm := make([]string, 0, len(owned))
	for i, o := range owned {
		if o.MedalID != "" {
			byID[o.MedalID] = i
		}
		if o.Name != "" {
			n := medalNormName(o.Name)
			byName[n] = i
			ownedNorm = append(ownedNorm, n)
		}
	}
	matched := make([]bool, len(owned))
	for i := range shop {
		it := &shop[i]
		oi := -1
		if it.MedalID != "" {
			if idx, ok := byID[it.MedalID]; ok {
				oi = idx
			}
		}
		if oi < 0 && it.Name != "" {
			if idx, ok := byName[medalNormName(it.Name)]; ok {
				oi = idx
			}
		}
		if oi < 0 && it.Name != "" {
			n := medalNormName(it.Name)
			for _, on := range ownedNorm {
				if on != "" && (strings.Contains(on, n) || strings.Contains(n, on)) {
					oi = byName[on]
					break
				}
			}
		}
		if oi >= 0 {
			it.PurchaseStatus = medalwall.StatusOwned
			it.WearStatus = owned[oi].Wearing
			matched[oi] = true
		}
	}
	// 未匹配的 owned（商店无售但拥有/佩戴）追加，标 OWNED
	for i, o := range owned {
		if matched[i] || o.Name == "" {
			continue
		}
		shop = append(shop, medalwall.Item{
			MedalID:          "owned:" + medalNormName(o.Name), // 商店无售的拥有勋章无真实 id，用 name 派生避 UNIQUE(site_id,medal_id) 冲突
			Name:             o.Name,
			ImageSmall:       o.Image,
			OriginalImageURL: o.Image,
			PurchaseStatus:   medalwall.StatusOwned,
			WearStatus:       o.Wearing,
			Currency:         "魔力",
		})
	}
	return shop
}

// medalNormName 归一名（trim + lower + 去空白），合并匹配用。
func medalNormName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.Join(strings.Fields(s), "")
}

// medalDef 把 site.SiteMetadata 投影为 medalwall.Def。
func medalDef(def *site.SiteMetadata) medalwall.Def {
	return medalwall.Def{Schema: def.Schema, ShopPath: def.Medal.ShopPath, OwnPath: def.Medal.OwnPath}
}

// medalJoinBase 拼 baseURL + 相对路径。
func medalJoinBase(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

// itemToCard 把勋章条目转为 UI 卡片（IsOwned 由 PurchaseStatus 派生）。
func itemToCard(siteID uint, siteName string, it medalwall.Item) MedalCard {
	return MedalCard{
		SiteID:           siteID,
		SiteName:         siteName,
		MedalID:          it.MedalID,
		Name:             it.Name,
		Description:      it.Description,
		ImageSmall:       it.ImageSmall,
		OriginalImageURL: it.OriginalImageURL,
		SaleBeginTime:    it.SaleBeginTime,
		SaleEndTime:      it.SaleEndTime,
		Price:            it.Price,
		OriginalPrice:    it.OriginalPrice,
		Validity:         it.Validity,
		BonusRate:        it.BonusRate,
		PurchaseStatus:   string(it.PurchaseStatus),
		Stock:            it.Stock,
		Currency:         it.Currency,
		Group:            it.Group,
		WearStatus:       it.WearStatus,
		IsOwned:          it.PurchaseStatus == medalwall.StatusOwned,
	}
}

// itemsToMedals / medalsToItems：medalwall.Item ↔ models.Medal 互转。
func itemsToMedals(items []medalwall.Item) []models.Medal {
	out := make([]models.Medal, 0, len(items))
	for _, it := range items {
		out = append(out, models.Medal{
			MedalID:          it.MedalID,
			Name:             it.Name,
			Description:      it.Description,
			ImageSmall:       it.ImageSmall,
			OriginalImageURL: it.OriginalImageURL,
			SaleBeginTime:    it.SaleBeginTime,
			SaleEndTime:      it.SaleEndTime,
			Price:            it.Price,
			OriginalPrice:    it.OriginalPrice,
			Validity:         it.Validity,
			BonusRate:        it.BonusRate,
			PurchaseStatus:   string(it.PurchaseStatus),
			GiftStatus:       it.GiftStatus,
			Stock:            it.Stock,
			StockStatus:      it.StockStatus,
			Currency:         it.Currency,
			GroupName:        it.Group,
			WearStatus:       it.WearStatus,
		})
	}
	return out
}

func medalsToItems(ms []models.Medal) []medalwall.Item {
	out := make([]medalwall.Item, 0, len(ms))
	for _, m := range ms {
		out = append(out, medalwall.Item{
			MedalID:          m.MedalID,
			Name:             m.Name,
			Description:      m.Description,
			ImageSmall:       m.ImageSmall,
			OriginalImageURL: m.OriginalImageURL,
			SaleBeginTime:    m.SaleBeginTime,
			SaleEndTime:      m.SaleEndTime,
			Price:            m.Price,
			OriginalPrice:    m.OriginalPrice,
			Validity:         m.Validity,
			BonusRate:        m.BonusRate,
			PurchaseStatus:   medalwall.Status(m.PurchaseStatus),
			GiftStatus:       m.GiftStatus,
			Stock:            m.Stock,
			StockStatus:      m.StockStatus,
			Currency:         m.Currency,
			Group:            m.GroupName,
			WearStatus:       m.WearStatus,
		})
	}
	return out
}

// medalToItem 单条 model→item。
func medalToItem(m models.Medal) medalwall.Item {
	return medalsToItems([]models.Medal{m})[0]
}

// evict 清单站内存缓存（写操作后强制重抓用）。
func (c *MedalChain) evict(siteID uint) {
	c.mu.Lock()
	delete(c.cache, siteID)
	c.mu.Unlock()
}

// validatePurchaseCandidate 购买前校验（best-effort，从 DB 读该站勋章的状态）。
func validatePurchaseCandidate(it *medalwall.Item) error {
	if it == nil {
		return nil // DB 无记录，交站点校验
	}
	switch it.PurchaseStatus {
	case medalwall.StatusOwned:
		return errors.New("已拥有该勋章")
	case medalwall.StatusNotOnSale:
		return errors.New("勋章未开售")
	case medalwall.StatusDiscontinued:
		return errors.New("勋章已下架")
	}
	if it.Stock == 0 {
		return errors.New("勋章库存为 0")
	}
	return nil
}

// Buy 购买勋章：校验 → adapter.Buy → 成功后清缓存重抓确认 isOwned。
func (c *MedalChain) Buy(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	// in-flight 去重：防双击/重放重复购买扣站点魔力。
	guardKey := fmt.Sprintf("buy:%d:%s", s.ID, medalID)
	if !c.guard.acquire(guardKey) {
		return medalwall.ActionResult{Success: false, Message: "购买正在进行中，请勿重复触发"}, nil
	}
	defer c.guard.release(guardKey)
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return medalwall.ActionResult{Success: false, Message: "该平台不支持勋章购买"}, nil
	}
	// 校验（best-effort）
	for _, m := range mustFindBySite(c.op, s.ID) {
		if m.MedalID == medalID {
			it := medalToItem(m)
			if err := validatePurchaseCandidate(&it); err != nil {
				return medalwall.ActionResult{Success: false, Message: err.Error()}, nil
			}
			break
		}
	}
	cookie := c.siteCh.credential(s, def)
	res, err := adapter.Buy(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	if err != nil {
		return medalwall.ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if res.Success {
		c.evict(s.ID)
		_, _ = c.medalSite(ctx, nil, s, true) // 重抓确认
	}
	return res, nil
}

// Wear 佩戴勋章。
func (c *MedalChain) Wear(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	return c.formAction(ctx, s, medalID, "wear")
}

// TakeOff 摘下勋章。
func (c *MedalChain) TakeOff(ctx context.Context, s *models.Site, medalID string) (medalwall.ActionResult, error) {
	return c.formAction(ctx, s, medalID, "takeoff")
}

func (c *MedalChain) formAction(ctx context.Context, s *models.Site, medalID, action string) (medalwall.ActionResult, error) {
	def := c.siteCh.defFor(s)
	adapter := c.reg.Get(def.Schema)
	if !adapter.Supported() {
		return medalwall.ActionResult{Success: false, Message: "该平台不支持勋章操作"}, nil
	}
	cookie := c.siteCh.credential(s, def)
	var res medalwall.ActionResult
	var err error
	switch action {
	case "wear":
		res, err = adapter.Wear(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	case "takeoff":
		res, err = adapter.TakeOff(ctx, c.hc, medalDef(def), s.BaseURL, cookie, medalID)
	}
	if err != nil {
		return medalwall.ActionResult{Success: false, Message: "请求失败：" + err.Error()}, nil
	}
	if res.Success {
		c.evict(s.ID)
		_, _ = c.medalSite(ctx, nil, s, true)
	}
	return res, nil
}
