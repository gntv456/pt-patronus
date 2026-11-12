// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/schemas"
	"golang.org/x/sync/singleflight"
)

// SiteResult 是单站搜索结果（含可能的错误，用于聚合容错）。
type SiteResult struct {
	SiteID   uint           `json:"site_id"`
	SiteName string         `json:"site_name"`
	Torrents []site.Torrent `json:"torrents"`
	Error    string         `json:"error,omitempty"`
}

// SiteConnectionResult 是站点联通检查结果。
// Status 为 connected、disconnected 或 disabled；失败时 Error 保留具体原因，供前端提示。
// CheckedAt 为本次检测时刻，供前端展示「X 分钟前检测」并做节流判断。
type SiteConnectionResult struct {
	SiteID    uint      `json:"site_id"`
	SiteName  string    `json:"site_name"`
	Status    string    `json:"status"`
	Level     string    `json:"level,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// SiteChain 编排站点相关业务：搜索、连接测试、用户信息。
type SiteChain struct {
	Base
	sites        *oper.SiteOper
	registry     *site.Registry
	hc           *httpclient.Client      // 站点 HTTP 客户端（FetchTorrentFile 下载种子用；SetHTTPClient 注入）
	records      *oper.CheckinRecordOper // 签到历史落库（可选；未注入则不落库，测试用）
	checkinGuard *spendGuard             // 签到 in-flight 去重（防双击/重放/cron 重叠重复签到）
	piggo        *schemas.Piggo          // piggo 下载适配器（SetHTTPClient 时按需构造；dl 链路用）

	// 站免/公告 per-site 缓存（站点是全局共享配置，故缓存按 siteID 而非 uid 分桶）。
	flMu     sync.Mutex
	flCache  map[uint]flCacheEntry
	flSF     singleflight.Group // 同 siteID 站免并发抓取合并（防击穿）
	annMu    sync.Mutex
	annCache map[uint]annCacheEntry
	annSF    singleflight.Group // 同 siteID 公告并发抓取合并（防击穿）
	// 后宫 per-site 缓存（邀请库存 + 被邀请人）。
	haremMu    sync.Mutex
	haremCache map[uint]haremCacheEntry
	haremSF    singleflight.Group // 同 siteID 后宫并发抓取合并（防击穿）
	// 种子详情 per-(site|url) 缓存（详情页较重，短缓存减重复抓取）。
	detailMu    sync.Mutex
	detailCache map[string]detailCacheEntry
	detailSF    singleflight.Group // 同 key 详情并发抓取合并（防击穿）
	// 消息 per-(site|mailbox) 缓存（信箱 6 类，key=siteID|mailbox 防串数据）。
	msgMu    sync.Mutex
	msgCache map[string]msgCacheEntry
	msgSF    singleflight.Group // 同 site+mailbox 并发抓取合并（防击穿）

	// per-site 串行锁：仅保证 SnapshotSite 不与另一个 SnapshotSite 同站并发（runSnapshot 本就串行 +
	// SkipIfStillRunning 已防重叠，此锁为防御性）。前端 *All 走 singleflight + 跨站 sem，不持此锁。
	siteLocksMu sync.Mutex
	siteLocks   map[uint]*sync.Mutex
}

// flCacheEntry / annCacheEntry 是 per-site 缓存条目。
type (
	flCacheEntry struct {
		notices []site.SiteFreeleech
		at      time.Time
	}
	annCacheEntry struct {
		ann *site.SiteAnnouncement
		at  time.Time
	}
	haremCacheEntry struct {
		status   *site.InviteStatus
		invitees []site.InviteeInfo
		at       time.Time
	}
	detailCacheEntry struct {
		detail *site.TorrentDetail
		at     time.Time
	}
	msgCacheEntry struct {
		messages []site.Message
		errMsg   string
		at       time.Time
	}
)

// NewSiteChain 构造 SiteChain。
func NewSiteChain(b Base, sites *oper.SiteOper, registry *site.Registry) *SiteChain {
	return &SiteChain{
		Base:         b,
		sites:        sites,
		registry:     registry,
		checkinGuard: newSpendGuard(),
		flCache:      make(map[uint]flCacheEntry),
		annCache:     make(map[uint]annCacheEntry),
		haremCache:   make(map[uint]haremCacheEntry),
		detailCache:  make(map[string]detailCacheEntry),
		msgCache:     make(map[string]msgCacheEntry),
		siteLocks:    make(map[uint]*sync.Mutex),
	}
}

// siteLock 返回（并惰性创建）某站的串行 mutex，供 SnapshotSite 持有（防御性，见 siteLocks 注释）。
func (c *SiteChain) siteLock(siteID uint) *sync.Mutex {
	c.siteLocksMu.Lock()
	defer c.siteLocksMu.Unlock()
	lk, ok := c.siteLocks[siteID]
	if !ok {
		lk = &sync.Mutex{}
		c.siteLocks[siteID] = lk
	}
	return lk
}

// piggoAdapter 惰性取 piggo 下载适配器（与注册表里的适配器同源 hc，避免另起炉灶）。
func (c *SiteChain) piggoAdapter() *schemas.Piggo {
	if c.piggo != nil {
		return c.piggo
	}
	if c.hc == nil {
		return nil
	}
	c.piggo = schemas.NewPiggo(c.hc)
	return c.piggo
}

// SetHTTPClient 注入站点 HTTP 客户端（FetchTorrentFile 下载 .torrent 用）。app 装配时调用；
// 用 setter 而非改 NewSiteChain 签名，避免波及现有测试构造。
func (c *SiteChain) SetHTTPClient(hc *httpclient.Client) { c.hc = hc }

// SyncHostLimits 把每个站点的 LimitInterval（抓取限流秒数）同步到站点 HTTP 客户端的
// per-host 最小请求间隔（站点流控）。启动与站点增改后调用；client 为 nil（测试）跳过。
// host 取 BaseURL 的域名部分——同域多实例（主站/镜像）共享同一限速桶。
func (c *SiteChain) SyncHostLimits(sites []models.Site, hc *httpclient.Client) {
	if hc == nil {
		return
	}
	for _, s := range sites {
		host := hostOf(s.BaseURL)
		if host == "" {
			continue
		}
		if s.LimitInterval > 0 {
			hc.SetHostInterval(host, time.Duration(s.LimitInterval)*time.Second)
		} else {
			hc.SetHostInterval(host, 0)
		}
	}
}

// defFor 返回站点对应的定义；无则回退为仅含 schema 的最小定义。
func (c *SiteChain) defFor(s *models.Site) *site.SiteMetadata {
	// 统一经 EnsureDefaults：站点 YAML 省略的 capability（announcement/messages/freeleech/
	// harem/detail 等）补 NexusPHP 默认 Enabled=true + 默认路径，避免被各 chain 的「!Enabled」
	// 守卫误跳过（如好学 hxpt.yaml 未配 announcement/messages 段 → Enabled=false → 公告/消息恒空）。
	if s.Definition != "" {
		if def, ok := c.registry.Definition(s.Definition); ok {
			return site.EnsureDefaults(def)
		}
	}
	return site.EnsureDefaults(&site.SiteMetadata{ID: s.Definition, Name: s.Name, Schema: s.Platform, URLs: []string{s.BaseURL}})
}
