package chain

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
)

// RssChain 编排 RSS 订阅：抓取→解析→过滤→去重→(可选)下载→通知。cron 全局扫描按各订阅间隔到点调 Check。
// 对标 MoviePilot FetchRssAction + 订阅检查（dedup 靠 RssItem.item_key）。
type RssChain struct {
	Base
	op     *oper.RssOper
	itemOp *oper.RssItemOper
	dlCh   *DownloadChain
	dlOp   *oper.DownloaderOper
	siteOp *oper.SiteOper
	notice *notice.Service
	hc     *httpclient.Client
	fp     *gofeed.Parser
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
func (c *RssChain) Delete(id uint) error { return c.op.Delete(id) }

// ListItems 某订阅的条目。
func (c *RssChain) ListItems(subID uint, limit int) ([]models.RssItem, error) {
	return c.itemOp.ListBySubscription(subID, limit)
}

// Check 检查单个订阅：抓取→解析→过滤→去重→(可选)下载→通知。返回本轮新发现条数。
func (c *RssChain) Check(ctx context.Context, sub *models.RssSubscription) (int, error) {
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
		return 0, fmt.Errorf("抓取 RSS 失败: %w", err)
	}
	feed, err := c.fp.ParseString(string(resp.Body))
	if err != nil {
		slog.Warn("rss parse failed", "sub", sub.ID, "err", err)
		if e := c.op.MarkChecked(sub.ID); e != nil {
			slog.Warn("rss MarkChecked failed", "sub", sub.ID, "err", e)
		}
		return 0, fmt.Errorf("解析 RSS 失败: %w", err)
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
	cands := make([]rssCandidate, 0, len(feed.Items))
	for i, item := range feed.Items {
		if i >= 200 {
			break
		}
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

		// 自动下载
		if sub.AutoDownload && dlID != 0 {
			opts := downloader.AddOptions{
				URL: cd.torrURL, Category: sub.Category,
				Tags: parseCsv(sub.Tags), SavePath: sub.SavePath,
				SiteID: sub.SiteID,
			}
			if err := c.dlCh.Add(ctx, dlID, opts); err != nil {
				slog.Warn("rss auto download failed", "sub", sub.ID, "title", cd.title, "err", err)
			} else {
				_ = c.itemOp.MarkDownloaded(it.ID)
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
	if newCount > 0 {
		slog.Info("rss new items", "sub", sub.Name, "count", newCount)
	}
	return newCount, nil
}

// CheckAll cron 全局扫描入口：按各订阅 interval_min vs last_checked_at 决定是否到点。
func (c *RssChain) CheckAll(ctx context.Context) {
	subs, err := c.op.ListEnabled()
	if err != nil {
		slog.Warn("rss list enabled failed", "err", err)
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

// rssItemKey guid 的稳定哈希（去重键）。
func rssItemKey(guid string) string {
	sum := sha1.Sum([]byte(guid))
	return hex.EncodeToString(sum[:])[:16]
}

// rssItemDownload 从 gofeed.Item 取种子下载链接与大小（优先 enclosure）。
func rssItemDownload(item *gofeed.Item) (url string, size int64) {
	if len(item.Enclosures) > 0 {
		enc := item.Enclosures[0]
		url = enc.URL
		size = site.ParseBytes(enc.Length)
	}
	if url == "" {
		url = item.Link // 可能是磁力
	}
	return
}

func rssItemPubDate(item *gofeed.Item) *time.Time {
	if item.PublishedParsed != nil {
		return item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed
	}
	return nil
}

// rssMatchAny 标题命中 | 分隔任一关键词（空规则=放行）。
func rssMatchAny(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// rssMatchNone 标题命中 | 分隔任一关键词即排除（空规则=不排除）。
func rssMatchNone(title, rules string) bool {
	if strings.TrimSpace(rules) == "" {
		return true
	}
	t := strings.ToLower(title)
	for _, kw := range strings.Split(rules, "|") {
		if k := strings.TrimSpace(kw); k != "" && strings.Contains(t, strings.ToLower(k)) {
			return false
		}
	}
	return true
}

// parseCsv 逗号分隔 → 切片（去空）。
func parseCsv(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
