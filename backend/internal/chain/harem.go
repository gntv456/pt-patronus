package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// 后宫抓取并发与缓存参数（对齐原版：Semaphore(5) + per-site Mutex + 30min 缓存）。
const (
	haremConcurrency = 5
	haremCacheTTL    = 30 * time.Minute
	haremTimeout     = 30 * time.Second
)

// HaremResult 是后宫聚合结果。
type HaremResult struct {
	Overview      site.HaremOverview      `json:"overview"`
	Prescriptions []site.PrescriptionItem `json:"prescriptions"`
}

// HaremStatus 抓单站邀请库存（适配器需实现 site.HaremCapable）。
// 站点定义未启用后宫/适配器不支持时返回 ErrNotSupported——不能返回 (nil, nil)，
// 否则调用方把「不支持」渲染成「剩余 0、权限 ✗」的假数据。
func (c *SiteChain) HaremStatus(ctx context.Context, s *models.Site) (*site.InviteStatus, error) {
	def := c.defFor(s)
	if !def.Harem.Enabled {
		return nil, ErrNotSupported
	}
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out *site.InviteStatus
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		hc, ok := a.(site.HaremCapable)
		if !ok {
			return ErrNotSupported
		}
		st, e := hc.FetchInviteStatus(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out = st
		return nil
	})
	return out, err
}

// HaremInvitees 抓单站被邀请人花名册。
func (c *SiteChain) HaremInvitees(ctx context.Context, s *models.Site) ([]site.InviteeInfo, error) {
	def := c.defFor(s)
	if !def.Harem.Enabled {
		return nil, ErrNotSupported
	}
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, cerr
	}
	var out []site.InviteeInfo
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		hc, ok := a.(site.HaremCapable)
		if !ok {
			return ErrNotSupported
		}
		is, e := hc.FetchInvitees(ctx, def, s.BaseURL, token)
		if e != nil {
			return e
		}
		out = is
		return nil
	})
	return out, err
}

// haremSFResult 是后宫单次回源（邀请库存 + 被邀请人）的 singleflight 结果。
type haremSFResult struct {
	status   *site.InviteStatus
	invitees []site.InviteeInfo
	errMsg   string
}

// HaremAll 跨站并发聚合后宫（Semaphore(5) + per-site Mutex + 30min 缓存）。
// 非 force 时单站 miss 走 singleflight 合并同 siteID 并发抓取（防击穿，sem 仅 leader 占用）。
// 站点清单读不出时返回错误——不能返回空结果让前端渲染成「没有站点」。
func (c *SiteChain) HaremAll(ctx context.Context, force bool) (*HaremResult, error) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("harem list sites failed", "err", err)
		return nil, err
	}
	items := make([]site.PrescriptionItem, len(sites))
	sem := make(chan struct{}, haremConcurrency)
	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Add(1)
		safego.Go("harem:94", func() {
			defer wg.Done()
			// per-site 隔离：单站 panic 交出它自己的错误药单行（safego 只记日志，零值行被过滤）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("harem site panic", "site", s.Name, "err", r)
					items[i] = haremPanicItem(s, r)
				}
			}()
			// 快速路径：缓存命中
			if !force {
				c.haremMu.Lock()
				e, ok := c.haremCache[s.ID]
				c.haremMu.Unlock()
				if ok && time.Since(e.at) <= haremCacheTTL {
					items[i] = buildPrescription(s.ID, s.Name, e.status, e.invitees, "")
					return
				}
			}
			// miss：force 直接回源；否则 singleflight 合并同 siteID 并发回源。
			var res haremSFResult
			if force {
				res = c.fetchHaremOnce(ctx, sem, &s)
			} else {
				var waitErr error
				res, waitErr = sfDoContext(ctx, &c.haremSF, strconv.Itoa(int(s.ID)), func() (haremSFResult, error) {
					// double-check：leader 完成后紧随的 caller 重跑 fn，缓存可能已满。
					c.haremMu.Lock()
					ce, ok := c.haremCache[s.ID]
					c.haremMu.Unlock()
					if ok && time.Since(ce.at) <= haremCacheTTL {
						return haremSFResult{status: ce.status, invitees: ce.invitees}, nil
					}
					return c.fetchHaremOnce(ctx, sem, &s), nil
				})
				if waitErr != nil {
					res.errMsg = ctxErrText(ctx, waitErr)
				}
			}
			status := res.status
			if status == nil {
				status = &site.InviteStatus{}
			}
			items[i] = buildPrescription(s.ID, s.Name, status, res.invitees, res.errMsg)
		})
	}
	wg.Wait()

	// 排序：剩余降序、站名升序；失败站点排末尾
	sorted := make([]site.PrescriptionItem, 0, len(items))
	for _, it := range items {
		if it.SiteID != 0 {
			sorted = append(sorted, it)
		}
	}
	for i := 1; i < len(sorted); i++ { // 插入排序（站点数小）
		for j := i; j > 0; j-- {
			a, b := sorted[j-1], sorted[j]
			aFail, bFail := a.Error != "", b.Error != ""
			swap := false
			switch {
			case aFail && !bFail:
				swap = true
			case aFail == bFail:
				if a.Remain != b.Remain {
					swap = a.Remain < b.Remain
				} else {
					swap = a.SiteName > b.SiteName
				}
			}
			if swap {
				sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			} else {
				break
			}
		}
	}
	return &HaremResult{Overview: haremOverview(sorted), Prescriptions: sorted}, nil
}

// fetchHaremOnce 抓单站邀请库存 + 被邀请人（占一个 sem 槽、独立超时），成功时写缓存。
func (c *SiteChain) fetchHaremOnce(ctx context.Context, sem chan struct{}, s *models.Site) haremSFResult {
	if err := acquireSemaphore(ctx, sem); err != nil {
		return haremSFResult{errMsg: ctxErrText(ctx, err)}
	}
	if sem != nil {
		defer func() { <-sem }()
	}
	siteCtx, cancel := context.WithTimeout(ctx, haremTimeout)
	defer cancel()
	status, e1 := c.HaremStatus(siteCtx, s)
	invitees, e2 := c.HaremInvitees(siteCtx, s)
	errMsg := ""
	if errors.Is(e1, site.ErrNotLoggedIn) {
		errMsg = "未登录或 cookie 失效"
	} else if errors.Is(e1, ErrNotSupported) || errors.Is(e2, ErrNotSupported) {
		errMsg = "该站不支持邀请页抓取"
	} else if e1 != nil {
		errMsg = ctxErrText(siteCtx, e1)
	} else if e2 != nil {
		errMsg = ctxErrText(siteCtx, e2)
	}
	if errMsg == "" {
		c.haremMu.Lock()
		c.haremCache[s.ID] = haremCacheEntry{status: status, invitees: invitees, at: time.Now()}
		c.haremMu.Unlock()
	}
	return haremSFResult{status: status, invitees: invitees, errMsg: errMsg}
}

// buildPrescription 构造单站药单行。
func buildPrescription(id uint, name string, st *site.InviteStatus, invitees []site.InviteeInfo, errMsg string) site.PrescriptionItem {
	return BuildPrescription(id, name, st, invitees, errMsg)
}

// haremPanicItem 崩站的那行药单（JSON 面与流式面共用，防两处口径漂移）：零值行会被聚合出口的
// `it.SiteID != 0` 整行丢掉 ⇒ 后宫页安静地少一站，与「这站没有成员」同形（#127 同族）。
func haremPanicItem(s models.Site, r any) site.PrescriptionItem {
	return buildPrescription(s.ID, s.Name, nil, nil, fmt.Sprintf("后宫抓取异常: %v", r))
}

// BuildPrescription 构造单站药单行（导出，供 handler 单站刷新用）。
func BuildPrescription(id uint, name string, st *site.InviteStatus, invitees []site.InviteeInfo, errMsg string) site.PrescriptionItem {
	if st == nil {
		st = &site.InviteStatus{}
	}
	// invitees 归一化为非 nil 切片：PrescriptionItem.Invitees 无 omitempty，
	// nil 会序列化成 JSON null，前端 row.invitees.length 取属性会崩（MTeam/NexusPHP
	// 在 cookie 无 uid、解析失败等路径返回 nil）。统一成 [] 让前端拿到空数组。
	if invitees == nil {
		invitees = []site.InviteeInfo{}
	}
	dead := 0
	for _, iv := range invitees {
		if iv.IsDead() {
			dead++
		}
	}
	return site.PrescriptionItem{
		SiteID: id, SiteName: name,
		Remain:         st.TotalRemain(),
		PermanentCount: st.PermanentCount,
		TemporaryCount: st.TemporaryCount,
		CanBuy:         st.TotalBuyable(),
		CanInvite:      st.CanInvite,
		HaremCount:     len(invitees),
		DeadCount:      dead,
		Invitees:       invitees,
		Error:          errMsg,
	}
}

// haremOverview 仅从成功站点汇总 7 格统计。
func haremOverview(items []site.PrescriptionItem) site.HaremOverview {
	var o site.HaremOverview
	for _, it := range items {
		if it.Error != "" {
			continue
		}
		o.TotalSites++
		o.TotalInvitees += it.HaremCount
		o.TotalPermInvites += it.PermanentCount
		o.TotalTempInvites += it.TemporaryCount
		o.TotalBanned += it.DeadCount
		for _, iv := range it.Invitees {
			if iv.IsLowRatio() {
				o.TotalLowRatio++
			}
			if iv.HasNoTrafficData() {
				o.TotalNoData++
			}
		}
	}
	return o
}

// ExportPrescription 生成药单文本（CRLF，ASCII 标点，权限 ✓/✗）。
func ExportPrescription(items []site.PrescriptionItem) (totalRemain, totalCanBuy, failed int, text string) {
	var sb []byte
	for _, it := range items {
		if it.Error != "" {
			failed++
			sb = append(sb, []byte("站点["+it.SiteName+"]: 获取失败["+it.Error+"]\r\n")...)
			continue
		}
		totalRemain += it.Remain
		totalCanBuy += it.CanBuy
		perm := "✗"
		if it.CanInvite {
			perm = "✓"
		}
		sb = append(sb, []byte("站点["+it.SiteName+"]: 剩余["+itoa(it.Remain)+"]个. 可购买["+itoa(it.CanBuy)+"]个. 权限["+perm+"]\r\n")...)
	}
	return totalRemain, totalCanBuy, failed, string(sb)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// HaremAllStream 流式推送跨站后宫：每站完成即 emit(prescription)，并在全部完成后返回 overview
// （供 handler 在 done 帧一次性下发准确汇总）。供 SSE 端点「加载一个显示一个」——卡片渐进显示，
// 顶部 7 格汇总数字在全部完成时给出（汇总本就需全量数据）。与 HaremAll 不同：不排序（按到达顺序显示，
// 渐进加载下排序反而使卡片跳动）。非 force 单站 miss 仍走 singleflight + 30min 缓存；force 各站强制回源。
// 返回的第二值是站点清单读失败的错误文本（空=正常）——清单读不出时以前静默返回空 overview，
// done 帧与「全部站点都空闲」在界面上同形；现在随 done 帧交回，调用方必须原样透出。
// 注意：emit 会被多个站点 goroutine 并发调用，调用方（handler）须自行串行化实际写入。
func (c *SiteChain) HaremAllStream(ctx context.Context, force bool, emit func(site.PrescriptionItem)) (site.HaremOverview, string) {
	sites, err := c.sites.ListActive()
	if err != nil {
		slog.Warn("harem list sites failed", "err", err)
		return site.HaremOverview{}, err.Error()
	}
	sem := make(chan struct{}, haremConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []site.PrescriptionItem
	// push 累积到 all（用于最终 overview）并转发给调用方 emit。
	push := func(item site.PrescriptionItem) {
		mu.Lock()
		all = append(all, item)
		mu.Unlock()
		emit(item)
	}
	for _, s := range sites {
		wg.Add(1)
		safego.Go("harem:321", func() {
			defer wg.Done()
			// per-site 隔离：崩站也必须 push 一行（否则前端只看到「少一站」，账面数字对不上）。
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("harem stream site panic", "site", s.Name, "err", r)
					push(haremPanicItem(s, r))
				}
			}()
			// 快速路径：缓存命中
			if !force {
				c.haremMu.Lock()
				e, ok := c.haremCache[s.ID]
				c.haremMu.Unlock()
				if ok && time.Since(e.at) <= haremCacheTTL {
					push(buildPrescription(s.ID, s.Name, e.status, e.invitees, ""))
					return
				}
			}
			// miss：force 直接回源；否则 singleflight 合并同 siteID 并发回源。
			var res haremSFResult
			if force {
				res = c.fetchHaremOnce(ctx, sem, &s)
			} else {
				var waitErr error
				res, waitErr = sfDoContext(ctx, &c.haremSF, strconv.Itoa(int(s.ID)), func() (haremSFResult, error) {
					c.haremMu.Lock()
					ce, ok := c.haremCache[s.ID]
					c.haremMu.Unlock()
					if ok && time.Since(ce.at) <= haremCacheTTL {
						return haremSFResult{status: ce.status, invitees: ce.invitees}, nil
					}
					return c.fetchHaremOnce(ctx, sem, &s), nil
				})
				if waitErr != nil {
					res.errMsg = ctxErrText(ctx, waitErr)
				}
			}
			status := res.status
			if status == nil {
				status = &site.InviteStatus{}
			}
			push(buildPrescription(s.ID, s.Name, status, res.invitees, res.errMsg))
		})
	}
	wg.Wait()
	return haremOverview(all), ""
}
