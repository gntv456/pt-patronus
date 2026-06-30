package chain

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// SubscribeChain 编排媒体订阅：跨站搜种子 → 分辨率过滤 → 去重 → 推送下载器 + 通知。
// 对标 MoviePilot 订阅。cron (SUBSCRIBE_CRON) 周期调 CheckAll；详情页订阅调 Create（建后即查一次）。
type SubscribeChain struct {
	siteCh *SiteChain
	dlCh   *DownloadChain
	dlOp   *oper.DownloaderOper
	op     *oper.SubscriptionOper
	notice *notice.Service
}

// NewSubscribeChain 构造。
func NewSubscribeChain(siteCh *SiteChain, dlCh *DownloadChain, dlOp *oper.DownloaderOper, op *oper.SubscriptionOper, ns *notice.Service) *SubscribeChain {
	return &SubscribeChain{siteCh: siteCh, dlCh: dlCh, dlOp: dlOp, op: op, notice: ns}
}

// List 全部订阅（handler 用）。
func (c *SubscribeChain) List() ([]models.Subscription, error) { return c.op.List() }

// Delete 删除（handler 用）。
func (c *SubscribeChain) Delete(id uint) error { return c.op.Delete(id) }

// Update 编辑（resolution/downloader/status/save_path 等）。
func (c *SubscribeChain) Update(s *models.Subscription) error { return c.op.Update(s) }

// Get 单条。
func (c *SubscribeChain) Get(id uint) (*models.Subscription, error) { return c.op.Get(id) }

// Create 新建订阅（同源同 id 已订阅则幂等返回），并立即异步检查一次。
func (c *SubscribeChain) Create(ctx context.Context, s *models.Subscription) (*models.Subscription, error) {
	if s.Category == "" {
		s.Category = s.Type // movie/tv → SearchSites category
	}
	if s.Status == "" {
		s.Status = "subscribed"
	}
	if existing, err := c.op.FindActiveBySource(s.Source, s.SourceID); err == nil && existing != nil {
		return existing, nil // 已订阅，幂等
	}
	if err := c.op.Create(s); err != nil {
		return nil, err
	}
	// 立即异步检查一次（不等 cron），让订阅即时生效；不阻塞创建响应。
	sub := *s
	go func() {
		cctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, err := c.Check(cctx, &sub); err != nil {
			slog.Warn("subscribe initial check failed", "title", sub.Title, "err", err)
		}
	}()
	return s, nil
}

// Check 检查单个订阅：跨站搜 → 分辨率过滤 → 去重 → 推送 → 通知。返回本轮推送条数。
func (c *SubscribeChain) Check(ctx context.Context, sub *models.Subscription) (int, error) {
	dlID := sub.DownloaderID
	if dlID == 0 {
		dlID = c.defaultDownloaderID()
		if dlID == 0 {
			c.noticeOnce(ctx, sub, "订阅检查："+sub.Title, "未配置默认下载器，已跳过自动下载", "warning")
			_ = c.op.MarkChecked(sub.ID)
			return 0, fmt.Errorf("no default downloader")
		}
	}

	results := c.siteCh.SearchSites(ctx, sub.Title, sub.Category)
	pushed := 0
	for _, sr := range results {
		if sr.Error != "" {
			continue
		}
		for _, t := range sr.Torrents {
			if t.DownloadURL == "" {
				continue
			}
			if sub.Resolution != "" && !matchResolution(t.Title, sub.Resolution) {
				continue
			}
			key := torrentKey(t.DownloadURL)
			if hit, _ := c.op.HitExists(sub.ID, key); hit {
				continue // 已推送过，去重
			}
			opts := downloader.AddOptions{
				URL:      t.DownloadURL,
				Category: "subscribe",
				Tags:     []string{"subscribe", sub.Title},
			}
			if sub.SavePath != "" {
				opts.SavePath = sub.SavePath
			}
			if err := c.dlCh.Add(ctx, dlID, opts); err != nil {
				slog.Warn("subscribe push failed", "title", sub.Title, "site", sr.SiteName, "err", err)
				continue
			}
			_ = c.op.AddHit(&models.SubscriptionHit{
				SubscriptionID: sub.ID, TorrentKey: key, Title: t.Title, SiteName: sr.SiteName,
			})
			c.noticeOnce(ctx, sub, "订阅命中："+sub.Title,
				fmt.Sprintf("已从 %s 推送：%s", sr.SiteName, t.Title), "success")
			pushed++
		}
	}

	_ = c.op.MarkChecked(sub.ID)
	// 电影：推送过即视为完成（不再周期查）；剧集：持续追新集，保持 subscribed。
	if sub.Type == "movie" && pushed > 0 {
		_ = c.op.SetStatus(sub.ID, "completed")
	}
	return pushed, nil
}

// CheckAll cron 入口：遍历订阅中的订阅逐个检查（各自超时，互不影响）。
func (c *SubscribeChain) CheckAll(ctx context.Context) {
	subs, err := c.op.ListActive()
	if err != nil {
		slog.Warn("subscribe list active failed", "err", err)
		return
	}
	for i := range subs {
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		n, err := c.Check(sctx, &subs[i])
		cancel()
		if err != nil {
			slog.Warn("subscribe check failed", "title", subs[i].Title, "err", err)
		} else if n > 0 {
			slog.Info("subscribe hits pushed", "title", subs[i].Title, "count", n)
		}
	}
}

// defaultDownloaderID 取默认下载器；无默认取第一个；都没有返回 0。
func (c *SubscribeChain) defaultDownloaderID() uint {
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

// noticeOnce 发一条通知中心消息（dedup：同 sub+torrent 才去重，状态提示不去重以免漏）。
func (c *SubscribeChain) noticeOnce(ctx context.Context, sub *models.Subscription, title, body, level string) {
	if c.notice == nil {
		return
	}
	_ = c.notice.Notice(ctx, notice.Input{
		Source: notice.SourceSubscribeHit,
		Key:    fmt.Sprintf("sub:%d:%d", sub.ID, time.Now().UnixNano()), // 状态提示不去重；命中去重靠 HitExists
		Title:  title, Body: body, Level: level,
	}, false)
}

// matchResolution 种子标题是否含期望分辨率（大小写不敏感）。如 "2160p" 匹配含 "2160P"/"2160" 的标题。
func matchResolution(title, want string) bool {
	if want == "" {
		return true
	}
	w := strings.ToUpper(strings.TrimSpace(want))
	t := strings.ToUpper(title)
	if strings.Contains(t, w) {
		return true
	}
	// 2160p ↔ 4K、1080p ↔ 1080 互认
	num := strings.TrimRight(strings.TrimRight(w, "PI"), " ")
	switch {
	case strings.HasPrefix(w, "2160") && strings.Contains(t, "4K"):
		return true
	case strings.HasPrefix(w, "4K") && strings.Contains(t, "2160"):
		return true
	case num != "" && num != w && strings.Contains(t, num):
		return true
	}
	return false
}

// torrentKey 下载链接的稳定哈希（去重键）。
func torrentKey(url string) string {
	sum := sha1.Sum([]byte(url))
	return hex.EncodeToString(sum[:])[:16]
}
