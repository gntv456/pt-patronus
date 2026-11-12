// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/mmcdole/gofeed"
)

// RssChain 编排 RSS 订阅：抓取→解析→过滤→去重→(可选)下载→通知。cron 全局扫描按各订阅间隔到点调 Check。
// （dedup 靠 RssItem.item_key）。
type RssChain struct {
	Base
	op        *oper.RssOper
	itemOp    *oper.RssItemOper
	dlCh      *DownloadChain
	dlOp      *oper.DownloaderOper
	siteOp    *oper.SiteOper
	notice    *notice.Service
	hc        *httpclient.Client
	fp        *gofeed.Parser
	filterEng *filter.Engine // 可空（未装配时跳过规则过滤，按 feed 序）
}

// NewRssChain 构造。
func NewRssChain(b Base, op *oper.RssOper, itemOp *oper.RssItemOper, dlCh *DownloadChain,
	dlOp *oper.DownloaderOper, siteOp *oper.SiteOper, ns *notice.Service, hc *httpclient.Client, filterEng *filter.Engine) *RssChain {
	return &RssChain{Base: b, op: op, itemOp: itemOp, dlCh: dlCh, dlOp: dlOp, siteOp: siteOp, notice: ns, hc: hc, fp: gofeed.NewParser(), filterEng: filterEng}
}

// List 全部订阅。
func (c *RssChain) List() ([]models.RssSubscription, error) { return c.op.List() }

// Get 单条。
func (c *RssChain) Get(id uint) (*models.RssSubscription, error) { return c.op.Get(id) }

// Create 新建（建后不立即检查，由 cron 或手动触发）。
func (c *RssChain) Create(r *models.RssSubscription) (*models.RssSubscription, error) {
	if err := c.op.Create(r); err != nil {
		return nil, err
	}
	return r, nil
}

// Update 编辑。
func (c *RssChain) Update(r *models.RssSubscription) error { return c.op.Update(r) }

// Delete 删除。
// Delete 删 RSS 订阅源（连条目）。判定同 SubscribeChain.Delete：收在链层一处，
// 供 HTTP 面与将来的工具面共用，不在每个调用方重抄一遍。
func (c *RssChain) Delete(id uint) error {
	if _, err := c.op.Get(id); err != nil {
		return err
	}
	return c.op.Delete(id)
}

// ListItems 某订阅的条目。
func (c *RssChain) ListItems(subID uint, limit int) ([]models.RssItem, error) {
	return c.itemOp.ListBySubscription(subID, limit)
}

// CountItems 某订阅的条目总数（列表被上限截断时说「还有多少没显示」）。
func (c *RssChain) CountItems(subID uint) (int64, error) { return c.itemOp.CountBySubscription(subID) }

// RssCheckWindow 单轮检查的 feed 条目窗口。它挡的是**副作用突发**不是内存——gofeed 早已
// 把整个 feed 解析进内存；首次同步大源时若不设上限，会把上千条目一次性写库，开了自动下载
// 还会把上千种子推给下载器，而 cron 给一轮只有 2 分钟上下文，跑到一半被取消反而整轮作废。
const RssCheckWindow = 200

// RssCheckResult 一轮检查的账面。scanned/total/truncated 必须和新发现条数同行：
// 只报「新发现 N 条」时，「源里 512 条只看了窗口内的 200 条」和「整个源都看过了」在界面上同形。
type RssCheckResult struct {
	New       int
	Scanned   int
	Total     int
	Truncated bool
	// DownloadFailed 本轮「发现了但没能推进下载器」的条数（无下载器 / 推送失败）：
	// 这些条目已回滚、下轮会重试。必须和 New 一起回给界面——只报「新增 N 条」
	// 时，条目列表为空没有任何下文，用户不知道自动下载一直没成功。
	DownloadFailed int
	// DownloadErrors 前几条失败原因（人类可读，最多 maxRssDownloadErrors 条）。
	DownloadErrors []string
}

// maxRssDownloadErrors 回给界面的失败明细上限（计数不封顶）。
const maxRssDownloadErrors = 3

func noteRssDownloadErr(res *RssCheckResult, reason string) {
	res.DownloadFailed++
	if len(res.DownloadErrors) < maxRssDownloadErrors {
		res.DownloadErrors = append(res.DownloadErrors, reason)
	}
}

// Check 检查单个订阅：抓取→解析→过滤→去重→(可选)下载→通知。返回本轮账面。
func (c *RssChain) Check(ctx context.Context, sub *models.RssSubscription) (*RssCheckResult, error) {
	cookie := ""
	if sub.SiteID != 0 {
		if s, err := c.siteOp.Get(sub.SiteID); err == nil && s != nil {
			if ck, err := c.siteOp.Cookie(s); err == nil {
				cookie = ck
			}
		}
	}
	// ssrf=true：RSS feed 应为公网地址，走 SSRF 防护拒私网/元数据，避免借 cron 持久探测内网。
	resp, err := c.hc.Get(ctx, sub.URL, cookie, true)
	if err != nil {
		slog.Warn("rss fetch failed", "sub", sub.ID, "url", sub.URL, "err", err)
		if e := c.op.MarkChecked(sub.ID); e != nil {
			slog.Warn("rss MarkChecked failed", "sub", sub.ID, "err", e)
		}
		return nil, fmt.Errorf("抓取 RSS 失败: %w", err)
	}
	feed, err := c.fp.ParseString(string(resp.Body))
	if err != nil {
		slog.Warn("rss parse failed", "sub", sub.ID, "err", err)
		if e := c.op.MarkChecked(sub.ID); e != nil {
			slog.Warn("rss MarkChecked failed", "sub", sub.ID, "err", e)
		}
		return nil, fmt.Errorf("解析 RSS 失败: %w", err)
	}

	// 解析下载器（自动下载时用）
	dlID := uint(0)
	if sub.AutoDownload {
		dlID = sub.DownloaderID
		if dlID == 0 {
			dlID = c.defaultDownloaderID()
		}
	}

	// 收集候选（前置过滤：include/exclude 关键词 + 分辨率 + 体积，始终生效）。
	type rssCandidate struct {
		item         *gofeed.Item
		key, torrURL string
		title        string
		size         int64
		pub          *time.Time
	}
	// 窗口取「哪 200 条」不能只按 feed 里的位置：RSS 不规定新旧顺序，不少站点/聚合器
	// 按「旧→新」升序输出，此时最新条目排在末尾，取前 RssCheckWindow 条会让它们永远
	// 落在窗口外——自动下载从此不再触发，而每轮界面都说「没有新条目」。
	res := &RssCheckResult{Total: len(feed.Items)}
	if res.Total > RssCheckWindow {
		res.Scanned = RssCheckWindow
		res.Truncated = true
		slog.Warn("rss scan window truncated", "sub", sub.ID, "name", sub.Name,
			"scanned", res.Scanned, "total", res.Total)
	} else {
		res.Scanned = res.Total
	}
	fromEnd := rssFeedAscending(feed.Items)
	cands := make([]rssCandidate, 0, res.Scanned)
	for i := 0; i < res.Scanned; i++ {
		idx := i
		if fromEnd {
			idx = len(feed.Items) - 1 - i
		}
		item := feed.Items[idx]
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		key := rssItemKey(guid)
		torrURL, size := rssItemDownload(item)
		title := item.Title
		if torrURL == "" {
			continue // 无可下载链接，跳过
		}
		if !rssMatchAny(title, sub.TitleInclude) || !rssMatchNone(title, sub.TitleExclude) {
			continue
		}
		if sub.Resolution != "" && !matchResolution(title, sub.Resolution) {
			continue
		}
		// 上下限共用一个判据：旧写法是两条独立比较，「没给出体积」在下限下被筛掉、
		// 在上限下被放行（方向相反，且设上限的用户恰恰是最不想拿到未知大小区间的那批）。
		if sub.MinSize > 0 && size < sub.MinSize {
			continue
		}
		if sub.MaxSize > 0 && size > sub.MaxSize {
			continue
		}
		cands = append(cands, rssCandidate{item: item, key: key, torrURL: torrURL, title: title, size: size, pub: rssItemPubDate(item)})
	}
	// 规则引擎：订阅 FilterGroups 优先，否则用全局 SubscribeFilterRuleGroups；空=不排序（按 feed 序）。
	if c.filterEng != nil {
		groups := parsePipeList(sub.FilterGroups)
		if len(groups) == 0 {
			if gnames, _ := c.filterEng.Store().GetBindings(filter.KeySubscribeFilterRuleGroups); len(gnames) > 0 {
				groups = gnames
			}
		}
		if len(groups) > 0 && len(cands) > 0 {
			torrents := make([]site.Torrent, 0, len(cands))
			for _, cd := range cands {
				var pub time.Time
				if cd.pub != nil {
					pub = *cd.pub
				}
				torrents = append(torrents, site.Torrent{Title: cd.title, DownloadURL: cd.torrURL, Size: bfFormatBytes(cd.size), PubDate: pub})
			}
			ranked, err := c.filterEng.Filter(ctx, groups, torrents, nil)
			if err == nil {
				byURL := make(map[string]rssCandidate, len(cands))
				for _, cd := range cands {
					byURL[cd.torrURL] = cd
				}
				filtered := make([]rssCandidate, 0, len(ranked))
				for _, mt := range ranked {
					if cd, ok := byURL[mt.Torrent.DownloadURL]; ok {
						filtered = append(filtered, cd)
					}
				}
				cands = filtered
			} else {
				slog.Warn("rss filter engine failed, keep feed order", "sub", sub.ID, "err", err)
			}
		}
	}
	// 去重 + 写库 + 自动下载。
	newCount := 0
	for _, cd := range cands {
		if seen, _ := c.itemOp.Exists(sub.ID, cd.key); seen {
			continue
		}
		it := &models.RssItem{
			SubscriptionID: sub.ID, ItemKey: cd.key, Title: cd.title, Link: cd.item.Link,
			TorrentURL: cd.torrURL, Size: cd.size, PubDate: cd.pub,
		}
		if err := c.itemOp.Create(it); err != nil {
			slog.Warn("rss item create failed", "sub", sub.ID, "err", err)
			continue
		}
		newCount++

		// 自动下载。失败/无下载器一律回滚去重行（Delete）：条目只有「成功入队」或
		// 「未处理待下轮重试」两种终态——否则下载器临时故障（重启/升级）即静默丢种，
		// 条目永久躺在「已见未下载」死行里，即使故障恢复也不再重试。
		if sub.AutoDownload {
			if dlID == 0 {
				slog.Warn("rss auto download skipped: no downloader configured", "sub", sub.ID, "title", cd.title)
				noteRssDownloadErr(res, "未配置下载器（自动下载已开启）：请到「RSS」页为该订阅选择一个下载器")
				if e := c.itemOp.Delete(it.ID); e != nil {
					slog.Warn("rss item rollback failed", "sub", sub.ID, "err", e)
				}
			} else if err := c.dlCh.Add(ctx, dlID, rssAutoDownloadOptions(sub, cd.torrURL, cd.key)); err != nil {
				slog.Warn("rss auto download failed", "sub", sub.ID, "title", cd.title, "err", err)
				noteRssDownloadErr(res, errtext.Text(err))
				if e := c.itemOp.Delete(it.ID); e != nil {
					slog.Warn("rss item rollback failed", "sub", sub.ID, "err", e)
				}
			} else {
				if e := c.itemOp.MarkDownloaded(it.ID); e != nil {
					slog.Warn("RSS 条目「已下载」标记写失败：种子已进下载器，条目却停在已见未下载态", "sub", sub.ID, "item", it.ID, "title", cd.title, "err", e)
				}
				if c.notice != nil {
					_ = c.notice.Notice(ctx, notice.Input{
						Source: notice.SourceRssHit,
						Key:    fmt.Sprintf("rss:%d:%s", sub.ID, cd.key),
						Title:  "RSS 命中：" + sub.Name, Body: cd.title, Level: "success",
					}, true)
				}
			}
		}
	}

	if e := c.op.MarkChecked(sub.ID); e != nil {
		slog.Warn("rss MarkChecked failed", "sub", sub.ID, "err", e)
	}
	res.New = newCount
	if newCount > 0 {
		slog.Info("rss new items", "sub", sub.Name, "count", newCount)
	}
	// 自动下载失败在 cron 里只有一行 Warn：条目已回滚、界面恒显示「已检查、新增 N 条」，
	// 而列表是空的——用户学到的是「RSS 就是没东西下来」。与抓取失败同理必须自曝
	// （按订阅+当天去重，失败每轮重试时不刷屏）。
	if res.DownloadFailed > 0 && c.notice != nil {
		_ = c.notice.Notice(ctx, rssDownloadFailNotice(sub, res), true)
	}
	return res, nil
}

// rssDownloadFailNotice 组装「自动下载推种失败」的站内通知。单独成函数是为了能
// 不连通知服务直接断言载荷（与 rssCheckFailNotice 同一形状）。
func rssDownloadFailNotice(
	sub *models.RssSubscription, res *RssCheckResult,
) notice.Input {
	body := fmt.Sprintf("本轮发现 %d 条新条目，其中 %d 条未能推送到下载器：%s。条目已回滚，下轮会自动重试。",
		res.New, res.DownloadFailed, strings.Join(res.DownloadErrors, "；"))
	day := time.Now().Format("2006-01-02")
	return notice.Input{
		Source: notice.SourceRssDownloadFail,
		Key:    fmt.Sprintf("rss-dlfail:%d:%s", sub.ID, day),
		Title:  "RSS 自动下载失败：" + sub.Name,
		Body:   body,
		Level:  "warning",
	}
}

// rssCheckFailNotice 组装「某个订阅本轮抓取/解析失败」的站内通知。
// 单独成函数是因为这条路径在 cron 里没有别的读者：Check 的失败分支也调了 MarkChecked，
// last_checked_at 照旧推进，列表里这条订阅始终显示「已检查」、新条目恒 0，
// 只有日志一行 Warn ⇒ 用户学到的是「这个源就是没东西」，而不是「一直抓不到」。
func rssCheckFailNotice(sub *models.RssSubscription, err error) notice.Input {
	return notice.Input{
		Source: notice.SourceRssCheckFail,
		Key:    fmt.Sprintf("rss-checkfail:%d:%s", sub.ID, time.Now().Format("2006-01-02")),
		Title:  "RSS 抓取失败：" + sub.Name,
		Body:   errtext.Text(err), // 走文案单一来源（真机第一轮暴露：这里用 err.Error() 会把 SSRF/dial 原文贴进通知）
		Level:  "warning",
	}
}

// CheckAll cron 全局扫描入口：按各订阅 interval_min vs last_checked_at 决定是否到点。
func (c *RssChain) CheckAll(ctx context.Context) {
	subs, err := c.op.ListEnabled()
	if err != nil {
		slog.Warn("rss list enabled failed", "err", err)
		// 自曝：启用订阅读不出=整轮扫描不跑、RSS 命中漏推，渲染成「没有订阅」是假绿。
		// 当天去重防 cron 刷屏；notice 可空（测试未装配时跳过）。
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceRssListFail,
				Key:    "rss-listfail:" + time.Now().Format("2006-01-02"),
				Title:  "RSS 订阅",
				Body:   "本轮 RSS 扫描未能开始：读取订阅列表失败 — " + err.Error(),
				Level:  "warning",
			}, true)
		}
		return
	}
	for i := range subs {
		sub := &subs[i]
		interval := time.Duration(sub.IntervalMin) * time.Minute
		if interval <= 0 {
			interval = 30 * time.Minute
		}
		if sub.LastCheckedAt != nil && time.Since(*sub.LastCheckedAt) < interval {
			continue // 未到点
		}
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		// per-sub recover：单个坏源 panic（解析/过滤/下载链 bug）不得拖累同 tick 其它订阅的检查。
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("rss check panic recovered", "sub", sub.Name, "panic", r)
				}
			}()
			if _, err := c.Check(sctx, sub); err != nil {
				slog.Warn("rss check failed", "sub", sub.Name, "err", err)
				// 只留日志＝没吭声：失败路径也推进了 last_checked_at，界面上这条订阅永远「健康」。
				// 自曝一条按订阅+当天去重的通知（口径同本文件的 rss_list_fail）。
				if c.notice != nil {
					_ = c.notice.Notice(sctx, rssCheckFailNotice(sub, err), true)
				}
			}
		}()
		cancel()
	}
}

// defaultDownloaderID 取默认下载器；无默认取第一个；都没有返回 0。
func (c *RssChain) defaultDownloaderID() uint {
	list, err := c.dlOp.List()
	if err != nil || len(list) == 0 {
		return 0
	}
	for _, d := range list {
		if d.IsDefault {
			return d.ID
		}
	}
	return list[0].ID
}
