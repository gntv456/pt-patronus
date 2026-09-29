package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// SubscribeChain 编排媒体订阅：跨站搜种子 → 分辨率过滤 → 去重 → 推送下载器 + 通知。
// 对标 MoviePilot 订阅。cron (SUBSCRIBE_CRON) 周期调 CheckAll；详情页订阅调 Create（建后即查一次）。
//
// 多评分低分保护：mediaCh.AggregatedRating 跨源聚合评分 + vote_count/上映天数闸门。
// 全局 MediaQualityConfig.Enabled 或订阅 EnableRatingGuard 任一为真即对该订阅启用闸门；
// 评分查询失败 fail-open（不阻断下载）。
type SubscribeChain struct {
	siteCh    *SiteChain
	dlCh      *DownloadChain
	dlOp      *oper.DownloaderOper
	op        *oper.SubscriptionOper
	notice    *notice.Service
	mediaCh   *MediaChain            // 可空（测试/未装配时跳过评分闸门与海报补全）
	filterEng *filter.Engine         // 可空（未装配时跳过规则过滤，回退 first-match）
	cfgOp     *oper.SystemConfigOper // 可空（测试/未装配时全局订阅默认取零值）
	// checkMu per-subscription (subID → *sync.Mutex) Check 互斥：Create 建后异步 Check 与
	// cron CheckAll 可能对同一订阅并发执行，虽 HitExists + 唯一索引兜底，仍存在双发竞态，
	// 故按 subID 串行 Check。零值 sync.Map 可用，无需构造。
	checkMu sync.Map
}

// NewSubscribeChain 构造。mediaCh 为多评分闸门用，可传 nil（评分保护置灰）；filterEng 为规则过滤用，可传 nil（回退 first-match）；
// cfgOp 为全局订阅默认用，可传 nil（回退零值默认）。
func NewSubscribeChain(siteCh *SiteChain, dlCh *DownloadChain, dlOp *oper.DownloaderOper, op *oper.SubscriptionOper, ns *notice.Service, mediaCh *MediaChain, filterEng *filter.Engine, cfgOp *oper.SystemConfigOper) *SubscribeChain {
	return &SubscribeChain{siteCh: siteCh, dlCh: dlCh, dlOp: dlOp, op: op, notice: ns, mediaCh: mediaCh, filterEng: filterEng, cfgOp: cfgOp}
}

// List 全部订阅（handler 用）。
func (c *SubscribeChain) List() ([]models.Subscription, error) { return c.op.List() }

// ListWithPosters 列出订阅并补齐历史记录或其他创建入口遗漏的海报。
// 精确来源详情优先，失败后才按标题跨源匹配；补图并发受限并写回数据库，后续列表无需重复请求。
func (c *SubscribeChain) ListWithPosters(ctx context.Context) ([]models.Subscription, error) {
	ss, err := c.op.List()
	if err != nil || c.mediaCh == nil {
		return ss, err
	}

	ctx, cancel := context.WithTimeout(ctx, mediaSearchBudget)
	defer cancel()
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range ss {
		if ss[i].PosterURL != "" {
			continue
		}
		wg.Add(1)
		safego.Go("subscribe:71", func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			posterURL := c.subscriptionPoster(ctx, &ss[i])
			if posterURL == "" {
				return
			}
			ss[i].PosterURL = posterURL
			if err := c.op.SetPosterURL(ss[i].ID, posterURL); err != nil {
				slog.Warn("subscribe poster backfill failed", "sub", ss[i].ID, "err", err)
			}
		})
	}
	wg.Wait()
	return ss, nil
}

func (c *SubscribeChain) subscriptionPoster(ctx context.Context, sub *models.Subscription) string {
	if detail, err := c.mediaCh.Detail(ctx, sub.Source, sub.SourceID); err == nil && detail != nil && detail.PosterURL != "" {
		return detail.PosterURL
	}
	posterURL, err := c.mediaCh.MatchPoster(ctx, sub.Title, sub.Year)
	if err != nil {
		return ""
	}
	return posterURL
}

// Delete 删除（handler 用）。
func (c *SubscribeChain) Delete(id uint) error { return c.op.Delete(id) }

// Update 编辑（resolution/downloader/status/save_path 等）。
// 用列级 Updates 仅写用户可编辑字段，不覆盖 cron 维护的 last_checked_at/current_priority/episode_priority。
func (c *SubscribeChain) Update(s *models.Subscription) error { return c.op.UpdateEditableFields(s) }

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
		// paused 复用复活：避免同源双订阅（恢复后同 torrent 双推送、进度各一份）。
		if existing.Status == "paused" {
			existing.Status = "subscribed"
			if err := c.op.Update(existing); err != nil {
				return nil, err
			}
		}
		return existing, nil // 已订阅/复活，幂等
	}
	if err := c.op.Create(s); err != nil {
		return nil, err
	}
	// 立即异步检查一次（不等 cron），让订阅即时生效；不阻塞创建响应。
	sub := *s
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("subscribe initial check panic", "title", sub.Title, "panic", r)
			}
		}()
		cctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if _, err := c.Check(cctx, &sub); err != nil {
			slog.Warn("subscribe initial check failed", "title", sub.Title, "err", err)
		}
	}()
	return s, nil
}

// Check 检查单个订阅：读一次全局默认后委托 checkWithDefaults。返回本轮推送条数。
// 经 per-subscription 互斥串行：Create 建后异步 Check 与 cron CheckAll 对同一订阅并发时互斥，
// 避免重复推送（HitExists + 唯一索引兜底之外的竞态保护）。
func (c *SubscribeChain) Check(ctx context.Context, sub *models.Subscription) (int, error) {
	unlock := c.lockCheck(sub.ID)
	defer unlock()
	return c.checkWithDefaults(ctx, sub, loadGlobalDefaults(c.cfgOp))
}

// lockCheck 取/建 per-subscription 互斥并加锁，返回解锁函数。
func (c *SubscribeChain) lockCheck(subID uint) func() {
	muAny, _ := c.checkMu.LoadOrStore(subID, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// CheckAll cron 入口：遍历订阅中的订阅逐个检查（各自超时，互不影响）。全局默认只读一次。
func (c *SubscribeChain) CheckAll(ctx context.Context) {
	subs, err := c.op.ListActive()
	if err != nil {
		slog.Warn("subscribe list active failed", "err", err)
		return
	}
	gd := loadGlobalDefaults(c.cfgOp)
	for i := range subs {
		// per-subscription 互斥：与 Create 建后异步 Check 串行，防同订阅并发重复推送。
		unlock := c.lockCheck(subs[i].ID)
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		n, err := c.checkWithDefaults(sctx, &subs[i], gd)
		cancel()
		unlock()
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

// noticeOnce 发一次性事件通知（命中推送等）：Key 掺时间戳不去重——命中去重靠
// HitExists，通知层再去重会漏报。
func (c *SubscribeChain) noticeOnce(ctx context.Context, sub *models.Subscription, title, body, level string) {
	c.noticeKeyed(ctx, sub, fmt.Sprintf("sub:%d:%d", sub.ID, time.Now().UnixNano()), title, body, level, false)
}

// noticeStable 持续型状态提示（稳定 Key + 当天去重）：同订阅同主题当天只发一条。
// 修复「未配置默认下载器」这类持续状态每轮 cron × 每订阅刷一条通知、淹没真实告警。
func (c *SubscribeChain) noticeStable(ctx context.Context, sub *models.Subscription, topic, title, body, level string) {
	c.noticeKeyed(ctx, sub, fmt.Sprintf("sub:%d:%s", sub.ID, topic), title, body, level, true)
}

func (c *SubscribeChain) noticeKeyed(ctx context.Context, sub *models.Subscription, key, title, body, level string, dedup bool) {
	if c.notice == nil {
		return
	}
	_ = c.notice.Notice(ctx, notice.Input{
		Source: notice.SourceSubscribeHit,
		Key:    key,
		Title:  title, Body: body, Level: level,
	}, dedup)
}
