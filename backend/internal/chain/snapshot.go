package chain

import (
	"context"
	"errors"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
)

// SnapshotResult 是单站聚合快照结果（site-snapshot 任务抓取一次，供 automation 层消费 + 分派通知）。
// 把原 stats / hr / freeleech / announcement / messages / cookie-keepalive 6 个任务对同站的分散请求，
// 收敛为一次串行抓取，降低站点感知的请求频率与多端点密集访问（对齐 MoviePilot 6h 串行哲学）。
type SnapshotResult struct {
	SiteID   uint
	SiteName string

	// 步骤 1：UserInfo（stats + uid + 登录态）
	UserInfo  *site.UserInfo // nil=抓取失败；含 uid 供步骤 2 assessment 复用
	UID       string         // 站点用户 ID（聚合步骤 2 传给 assessment，避免裸访 userdetails.php）
	UserErr   string
	LoggedOut bool // cookie 失效（UserInfo 返回 ErrNotLoggedIn）；为 true 时后续步骤跳过

	// 步骤 2：KeepAlive（cookie 轮换）
	RefreshedCookie string // 非空且 ≠ DB 现值时 automation 回写

	// 步骤 3：H&R
	HRTorrents []site.HRTorrent
	HRErr      string

	// 步骤 4：indexInfo（站免 + 公告合并）
	Freeleech    []site.SiteFreeleech
	FLErr        string // indexInfo 合并返回，FL/Ann 共享错误串
	Announcement *site.SiteAnnouncement

	// 步骤 5：Messages
	Messages []site.Message
	MsgErr   string
}

// SnapshotSite 单站聚合抓取（per-site 锁内执行），供 automation.runSnapshot 逐站编排——
// 每站抓取后由 automation 插入 assessment（复用 uid）+ 通知分派。掉线站仅打 1 次 usercp。
func (c *SiteChain) SnapshotSite(ctx context.Context, s *models.Site) SnapshotResult {
	lk := c.siteLock(s.ID)
	lk.Lock()
	defer lk.Unlock()
	return c.snapshotSite(ctx, s)
}

// snapshotSite 单站串行抓取核心（调用方负责 per-site 锁）。掉线站仅打 1 次 usercp（UserInfo）。
func (c *SiteChain) snapshotSite(ctx context.Context, s *models.Site) SnapshotResult {
	res := SnapshotResult{SiteID: s.ID, SiteName: s.Name}
	def := c.defFor(s)

	// 1. UserInfo：基础统计 + uid + 登录态判定（usercp.php 未重定向登录页=已登录）。
	ui, err := c.UserInfo(ctx, s)
	if err != nil {
		if errors.Is(err, site.ErrNotLoggedIn) {
			res.LoggedOut = true
		}
		res.UserErr = err.Error()
	}
	if ui != nil {
		res.UserInfo = ui
		res.UID = ui.ID
	}
	if res.LoggedOut {
		return res // 掉线：后续请求无意义，省请求
	}

	// 2. KeepAlive：刷新 session + 捕获 cookie 轮换 + 补充掉线检测（端点敏感度与 UserInfo 不同，
	// 任一判掉线即视为掉线；KeepAlive 判掉线则跳过后续 HR/indexInfo/Messages）。
	if ka, e := c.keepAliveLocked(ctx, s); e == nil && ka != nil {
		if !ka.LoggedIn {
			res.LoggedOut = true
		}
		res.RefreshedCookie = ka.RefreshedCookie
	}
	if res.LoggedOut {
		return res
	}

	// H&R 由独立 hr-check cron 每小时抓取（封号命脉，时效优先于 4h 聚合），此处不抓。

	// 3. indexInfo：合并站免 + 公告（一次 GET index.php，两 path 一致时）+ 写缓存供前端秒读。
	fl, ann, e := c.indexInfo(ctx, s)
	if e != nil {
		res.FLErr = e.Error()
	}
	res.Freeleech = fl
	res.Announcement = ann
	c.writeInfoCache(s, fl, ann)

	// 5. Messages（仅 def.Messages.Enabled）+ 写缓存（成功/失败均写，负缓存与 fetchMessages 一致）。
	if def.Messages.Enabled {
		msgs, e := c.Messages(ctx, s, site.MailboxInbox)
		if e != nil {
			res.MsgErr = e.Error()
		}
		res.Messages = msgs
		c.writeMsgCache(s, site.MailboxInbox, msgs, res.MsgErr, e)
	}
	return res
}

// indexInfo 抓单站首页同时解析站免 + 公告：两 path 一致（默认 index.php）且 adapter 实现
// IndexInfoCapable 时一次 GET 双解析（省一次 index.php）；否则回退分别调用 FetchFreeleech + FetchAnnouncement
// （两 path 不同，或 adapter 未实现合并，如 MTeam 走不同 JSON 端点）。
func (c *SiteChain) indexInfo(ctx context.Context, s *models.Site) ([]site.SiteFreeleech, *site.SiteAnnouncement, error) {
	def := c.defFor(s)
	flEnabled := def.Freeleech.Enabled
	annEnabled := def.Announcement.Enabled
	if !flEnabled && !annEnabled {
		return nil, nil, nil
	}
	token, cerr := c.credentialWithErr(s, def)
	if cerr != nil {
		return nil, nil, cerr
	}
	flPath := def.Freeleech.Path
	if flPath == "" {
		flPath = "index.php"
	}
	annPath := def.Announcement.Path
	if annPath == "" {
		annPath = "index.php"
	}
	samePath := flEnabled && annEnabled && flPath == annPath

	var fl []site.SiteFreeleech
	var ann *site.SiteAnnouncement
	err := c.dispatchAdapter(ctx, def.Schema, func(a site.Adapter) error {
		// 优先合并：两 path 一致 + adapter 实现 IndexInfoCapable。
		if samePath {
			if ic, ok := a.(site.IndexInfoCapable); ok {
				info, e := ic.FetchIndexInfo(ctx, def, s.BaseURL, token)
				if e != nil {
					return e
				}
				fl = info.Freeleech
				ann = info.Announcement
				return nil
			}
		}
		// 回退：分别调用。
		if flEnabled {
			if fc, ok := a.(site.FreeleechCapable); ok {
				ns, e := fc.FetchFreeleech(ctx, def, s.BaseURL, token)
				if e != nil {
					return e
				}
				fl = ns
			}
		}
		if annEnabled {
			if ac, ok := a.(site.AnnouncementCapable); ok {
				an, e := ac.FetchAnnouncement(ctx, def, s.BaseURL, token)
				if e != nil {
					return e
				}
				ann = an
			}
		}
		return nil
	})
	for i := range fl { // 盖章站点身份（适配器解析时不知 siteID/name）
		fl[i].SiteID = s.ID
		fl[i].SiteName = s.Name
	}
	if ann != nil {
		ann.SiteID = s.ID
		ann.SiteName = s.Name
		if ann.Items == nil {
			ann.Items = []site.AnnouncementItem{}
		}
	}
	return fl, ann, err
}

// writeInfoCache 把聚合抓取的站免/公告写入 per-site 缓存，供前端/API 手动刷新时秒读（命中缓存）。
func (c *SiteChain) writeInfoCache(s *models.Site, fl []site.SiteFreeleech, ann *site.SiteAnnouncement) {
	if fl != nil {
		c.flMu.Lock()
		c.flCache[s.ID] = flCacheEntry{notices: fl, at: time.Now()}
		c.flMu.Unlock()
	}
	if ann != nil && ann.Error == "" {
		c.annMu.Lock()
		c.annCache[s.ID] = annCacheEntry{ann: ann, at: time.Now()}
		c.annMu.Unlock()
	}
}

// writeMsgCache 把聚合抓取的消息写入 per-(site|mailbox) 缓存。
// cause 是本次抓取的原始错误：调用方中断（context.Canceled）不是站点故障，不缓存——
// 否则一次切页/断连就把「context canceled」伪装成站点故障钉在缓存里，直到 TTL 过期。
func (c *SiteChain) writeMsgCache(s *models.Site, mailbox site.MailboxType, msgs []site.Message, errMsg string, cause error) {
	if errors.Is(cause, context.Canceled) {
		return
	}
	if msgs == nil {
		msgs = []site.Message{}
	}
	c.msgMu.Lock()
	c.msgCache[msgCacheKey(s.ID, mailbox)] = msgCacheEntry{messages: msgs, errMsg: errMsg, at: time.Now()}
	c.msgMu.Unlock()
}
