package chain

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/transfer"
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
	mediaCh   *MediaChain    // 可空（测试/未装配时跳过评分闸门与海报补全）
	filterEng *filter.Engine // 可空（未装配时跳过规则过滤，回退 first-match）
	cfgOp     *oper.SystemConfigOper // 可空（测试/未装配时全局订阅默认取零值）
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
		go func(i int) {
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
		}(i)
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

// Check 检查单个订阅：读一次全局默认后委托 checkWithDefaults。返回本轮推送条数。
func (c *SubscribeChain) Check(ctx context.Context, sub *models.Subscription) (int, error) {
	return c.checkWithDefaults(ctx, sub, loadGlobalDefaults(c.cfgOp))
}

// checkWithDefaults 检查单个订阅（用传入全局默认计算有效设置）：跨站搜 → 分辨率过滤 → 去重 → 推送 → 通知。
func (c *SubscribeChain) checkWithDefaults(ctx context.Context, sub *models.Subscription, gd GlobalDefaults) (int, error) {
	eff := resolveEffective(sub, gd)
	dlID := sub.DownloaderID
	if dlID == 0 {
		dlID = c.defaultDownloaderID()
		if dlID == 0 {
			c.noticeOnce(ctx, sub, "订阅检查："+sub.Title, "未配置默认下载器，已跳过自动下载", "warning")
			if err := c.op.MarkChecked(sub.ID); err != nil {
				slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
			}
			return 0, fmt.Errorf("no default downloader")
		}
	}

	// SiteIDs 有效值（订阅空→全局默认；都空→全部活跃站，SearchSites 已处理）。
	results := c.siteCh.SearchSites(ctx, sub.Title, sub.Category, eff.SiteIDs)

	// 多评分低分保护：闸门开启则命中种子前先查聚合评分；未通过则本轮整体跳过 + 通知。
	// 评分查询失败 fail-open（不阻断下载——外网评分服务故障不应让订阅停摆）。
	if held, reason := c.ratingGate(ctx, sub, eff.EnableRatingGuard); held {
		c.noticeOnce(ctx, sub, "订阅评分保护："+sub.Title,
			fmt.Sprintf("未通过质量闸门：%s（本轮跳过）", reason), "warning")
		if err := c.op.MarkChecked(sub.ID); err != nil {
			slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
		}
		return 0, nil
	}

	// 候选种子扁平化（带 SiteName），供规则引擎统一排序。
	var all []site.Torrent
	for _, sr := range results {
		if sr.Error != "" {
			continue
		}
		for _, t := range sr.Torrents {
			if t.DownloadURL == "" {
				continue
			}
			if t.SiteName == "" {
				t.SiteName = sr.SiteName // SearchSites 按站聚合，但单条 Torrent 无站名（types.go 注释）
			}
			all = append(all, t)
		}
	}
	// 简单参数前置过滤：分辨率有效值（订阅空→全局默认；空=不过滤；对齐 MP-2 TorrentHelper 两阶段前置过滤）。
	if eff.Resolution != "" {
		all = filterByResolution(all, eff.Resolution)
	}
	// 促销类型多选：有效值非空则只保留促销类型 ∈ eff.PromoTypes 的种（位于分辨率过滤后、规则引擎前）。
	// 空=不过滤（任何促销都接受，含非促销）。
	if len(eff.PromoTypes) > 0 {
		want := make(map[site.PromoType]struct{}, len(eff.PromoTypes))
		for _, p := range eff.PromoTypes {
			want[site.PromoType(p)] = struct{}{}
		}
		kept := make([]site.Torrent, 0, len(all))
		for _, t := range all {
			if _, ok := want[site.PromoTypeOf(t)]; ok {
				kept = append(kept, t)
			}
		}
		all = kept
	}
	// 规则引擎排序（保留 PriOrder 供洗版门控）：
	//   普通订阅 → FilterGroups 优先，否则全局 SubscribeFilterRuleGroups；
	//   洗版订阅 → BestVersionFilterRuleGroups 优先（允许更严格的画质门槛），空则回退普通绑定。
	type candidate struct {
		t   site.Torrent
		pri int
	}
	var ranked []candidate
	if c.filterEng != nil {
		groups := resolveFilterGroups(c.filterEng, sub, eff.BestVersion)
		if len(groups) > 0 {
			media := &filter.MediaInfo{Type: sub.Type, Category: sub.Category}
			matched, err := c.filterEng.Filter(ctx, groups, all, media)
			if err != nil {
				slog.Warn("subscribe filter engine failed, fall back to unfiltered", "sub", sub.ID, "err", err)
			} else {
				for _, mt := range matched {
					ranked = append(ranked, candidate{t: mt.Torrent, pri: mt.PriOrder})
				}
			}
		}
	}
	if len(ranked) == 0 {
		// 无规则或过滤失败 → 全部保留（PriOrder=0，洗版门控此时仅放行 CurrentPriority==0 的首版）。
		for _, t := range all {
			ranked = append(ranked, candidate{t: t})
		}
	}

	// 推送（按 PriOrder 降序，去重 + 洗版门控 + 推送 + 通知）。
	pushed := 0
	epPri := loadEpisodePriority(sub.EpisodePriority) // 剧集洗版按集进度（非洗版/电影不使用）
	for _, cand := range ranked {
		t, pri := cand.t, cand.pri
		// 洗版门控：候选画质须严格优于已记录最优才推送（避免重复下同级/更低版）。
		if eff.BestVersion {
			if sub.Type == "movie" {
				if pri <= sub.CurrentPriority {
					continue // 未更优
				}
			} else {
				keys, seasonPack := episodeKeys(t.Title)
				if !interestedEpisode(epPri, keys, seasonPack, pri) {
					continue
				}
			}
		}
		key := torrentKey(t.DownloadURL)
		hit, hitErr := c.op.HitExists(sub.ID, key)
		if hitErr != nil {
			// 去重判断失败：宁可漏推不可重复下载，跳过该种子（下次重试）。
			slog.Warn("subscribe HitExists failed, skip torrent to avoid duplicate", "sub", sub.ID, "err", hitErr)
			continue
		}
		if hit {
			continue // 已推送过，去重
		}
		opts := downloader.AddOptions{
			URL:      t.DownloadURL,
			Category: "subscribe",
			Tags:     []string{"subscribe", sub.Title, transfer.EncodeMediaTag(sub.Source, sub.SourceID, sub.Type)},
		}
		if sub.SavePath != "" {
			opts.SavePath = sub.SavePath
		}
		if err := c.dlCh.Add(ctx, dlID, opts); err != nil {
			slog.Warn("subscribe push failed", "title", sub.Title, "site", t.SiteName, "err", err)
			continue
		}
		if err := c.op.AddHit(&models.SubscriptionHit{
			SubscriptionID: sub.ID, TorrentKey: key, Title: t.Title, SiteName: t.SiteName,
		}); err != nil {
			slog.Warn("subscribe AddHit failed, may re-push next cycle", "sub", sub.ID, "key", key, "err", err)
		}
		c.noticeOnce(ctx, sub, "订阅命中："+sub.Title,
			fmt.Sprintf("已从 %s 推送：%s", t.SiteName, t.Title), "success")
		pushed++
		// 洗版进度回填（内存，循环外统一持久化）。
		if eff.BestVersion {
			if sub.Type == "movie" {
				sub.CurrentPriority = pri
			} else {
				keys, seasonPack := episodeKeys(t.Title)
				updateEpisodePriority(epPri, keys, seasonPack, pri)
			}
		}
	}

	if err := c.op.MarkChecked(sub.ID); err != nil {
		slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
	}
	// 洗版进度持久化 + 完成判定。
	if eff.BestVersion && pushed > 0 {
		if err := c.op.SetBestVersionProgress(sub.ID, sub.CurrentPriority, saveEpisodePriority(epPri)); err != nil {
			slog.Warn("subscribe SetBestVersionProgress failed", "sub", sub.ID, "err", err)
		}
		// 电影达最高画质（PriOrder≥100）视为洗版完成；剧集持续升级，不自动完成。
		if sub.Type == "movie" && sub.CurrentPriority >= 100 {
			if err := c.op.SetStatus(sub.ID, "completed"); err != nil {
				slog.Warn("subscribe SetStatus completed failed", "sub", sub.ID, "err", err)
			}
			return pushed, nil
		}
	}
	// 非洗版电影：推送过即视为完成（不再周期查）；剧集持续追新集，保持 subscribed。
	if !eff.BestVersion && sub.Type == "movie" && pushed > 0 {
		if err := c.op.SetStatus(sub.ID, "completed"); err != nil {
			slog.Warn("subscribe SetStatus completed failed", "sub", sub.ID, "err", err)
		}
	}
	return pushed, nil
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
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		n, err := c.checkWithDefaults(sctx, &subs[i], gd)
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

// ratingGate 多评分低分保护判定：返回 (是否拦截本轮推送, 拦截原因)。
//   - 未装配 mediaCh / 闸门未启用 → (false, "") 放行。
//   - 聚合查询失败 → (false, "") fail-open 放行（外网评分故障不应阻断下载）。
//   - 查到但 summary.Passed==false → (true, reason) 拦截。
//
// 启用条件：全局 MediaQualityConfig.Enabled 或本订阅有效 EnableRatingGuard（已回退全局订阅默认）任一为真。
func (c *SubscribeChain) ratingGate(ctx context.Context, sub *models.Subscription, enableGuard bool) (bool, string) {
	if c.mediaCh == nil {
		return false, ""
	}
	cfg := c.mediaCh.LoadQuality()
	if !cfg.Enabled && !enableGuard {
		return false, ""
	}
	summary, err := c.mediaCh.AggregatedRating(ctx, sub.Title, sub.Year, sub.Type)
	if err != nil || summary == nil {
		return false, "" // fail-open
	}
	if summary.Passed {
		return false, ""
	}
	return true, summary.Reason
}

// filterByResolution 按分辨率子串过滤（复用 matchResolution，对齐 MP-2 两阶段前置过滤）。
func filterByResolution(torrents []site.Torrent, want string) []site.Torrent {
	out := torrents[:0]
	for _, t := range torrents {
		if matchResolution(t.Title, want) {
			out = append(out, t)
		}
	}
	return out
}

// parsePipeList 按管道符拆分（trim + 去空）；FilterGroups 字段格式 "组A|组B"。
func parsePipeList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveFilterGroups 解析订阅应使用的规则组：
//   - 洗版订阅优先 BestVersionFilterRuleGroups（允许更严格画质门槛），空则回退普通绑定；
//   - 普通订阅用 FilterGroups，空则全局 SubscribeFilterRuleGroups。
// 空切片 = 不过滤（全部保留 PriOrder=0）。
func resolveFilterGroups(eng *filter.Engine, sub *models.Subscription, bestVersion bool) []string {
	if bestVersion {
		if names, _ := eng.Store().GetBindings(filter.KeyBestVersionFilterRuleGroups); len(names) > 0 {
			return names
		}
		// 洗版绑定未配 → 回退普通绑定（洗版门控仍按 PriOrder 拦截同级/更低版）。
	}
	if groups := parsePipeList(sub.FilterGroups); len(groups) > 0 {
		return groups
	}
	if names, _ := eng.Store().GetBindings(filter.KeySubscribeFilterRuleGroups); len(names) > 0 {
		return names
	}
	return nil
}

// loadEpisodePriority 反序列化按集 PriOrder（{"S01E02":80}）；脏数据/空回退空 map。
func loadEpisodePriority(s string) map[string]int {
	m := map[string]int{}
	if strings.TrimSpace(s) == "" {
		return m
	}
	_ = json.Unmarshal([]byte(s), &m)
	return m
}

// saveEpisodePriority 序列化按集 PriOrder；空 map 返回 ""。
func saveEpisodePriority(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// episodeKeys 从种子标题解析其覆盖的集（SxxExx 形式）。
// 返回 (keys, seasonPack)：单集→keys=["S01E02"]；整季包（S01 或仅有季号）→seasonPack=true；
// 电影/不可解析→keys=nil, seasonPack=false。
func episodeKeys(title string) (keys []string, seasonPack bool) {
	m := transfer.ParseMeta(title)
	se := m.SeasonEpisode
	if se == "" {
		if m.Season > 0 {
			return nil, true
		}
		return nil, false
	}
	// 纯季号（"S01" 无 E 部分）= 整季包
	if !strings.Contains(se, "E") {
		return nil, true
	}
	return []string{se}, false
}

// interestedEpisode 报告候选（画质=pri）是否对该订阅的剧集进度有改善。
//   - 单集 keys：任一集当前 PriOrder < pri（或该集首次出现）→ 感兴趣；
//   - 整季包 seasonPack：从未见过任何集 OR pri 高于已见最弱集 → 感兴趣。
func interestedEpisode(epPri map[string]int, keys []string, seasonPack bool, pri int) bool {
	if len(keys) > 0 {
		for _, k := range keys {
			if cur, ok := epPri[k]; !ok || pri > cur {
				return true
			}
		}
		return false
	}
	if seasonPack {
		if len(epPri) == 0 {
			return true // 首次：整季包建立基线
		}
		for _, cur := range epPri {
			if pri > cur {
				return true
			}
		}
		return false
	}
	return false
}

// updateEpisodePriority 按候选覆盖的集回填进度（取 max，单调不降）。
func updateEpisodePriority(epPri map[string]int, keys []string, seasonPack bool, pri int) {
	if len(keys) > 0 {
		for _, k := range keys {
			if cur, ok := epPri[k]; !ok || pri > cur {
				epPri[k] = pri
			}
		}
		return
	}
	if seasonPack {
		if len(epPri) == 0 {
			return // 未知集数，留待单集资源补全
		}
		for k, cur := range epPri {
			if pri > cur {
				epPri[k] = pri
			}
		}
	}
}
