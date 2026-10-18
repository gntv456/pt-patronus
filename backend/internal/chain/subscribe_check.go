package chain

import (
	"context"
	"fmt"
	"log/slog"
	"sort"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/transfer"

	"strings"
)

// checkWithDefaults 检查单个订阅（用传入全局默认计算有效设置）：跨站搜 → 分辨率过滤 → 去重 → 推送 → 通知。
func (c *SubscribeChain) checkWithDefaults(ctx context.Context, sub *models.Subscription, gd GlobalDefaults) (int, error) {
	eff := resolveEffective(sub, gd)
	dlID := sub.DownloaderID
	if dlID == 0 {
		dlID = c.defaultDownloaderID()
		if dlID == 0 {
			// 持续型状态：稳定 Key 当天去重（此前每轮 cron 每订阅刷一条，淹没真实告警）。
			c.noticeStable(ctx, sub, "no-downloader", "订阅检查："+sub.Title, "未配置默认下载器，已跳过自动下载", "warning")
			if err := c.op.MarkCheckedWithResult(sub.ID, "未配置默认下载器，本轮没有搜索"); err != nil {
				slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
			}
			return 0, fmt.Errorf("no default downloader")
		}
	}

	// SiteIDs 有效值（订阅空→全局默认；都空→全部活跃站，SearchSites 已处理）。
	results, err := c.siteCh.SearchSites(ctx, sub.Title, sub.Category, eff.SiteIDs)
	if err != nil {
		if err := c.op.MarkCheckedWithResult(sub.ID, "跨站搜索失败："+errtext.Text(err)); err != nil {
			slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
		}
		return 0, fmt.Errorf("跨站搜索失败: %w", err)
	}

	// 多评分低分保护：闸门开启则命中种子前先查聚合评分；未通过则本轮整体跳过 + 通知。
	// 评分查询失败 fail-open（不阻断下载——外网评分服务故障不应让订阅停摆）。
	held, reason, gateSkip := c.ratingGate(ctx, sub, eff.EnableRatingGuard)
	if held {
		// 持续型状态（闸门未通过会连续多轮 held）：稳定 Key 当天去重。
		c.noticeStable(ctx, sub, "rating-guard", "订阅评分保护："+sub.Title,
			fmt.Sprintf("未通过质量闸门：%s（本轮跳过）", reason), "warning")
		if err := c.op.MarkCheckedWithResult(sub.ID, fmt.Sprintf("未通过质量闸门：%s（本轮跳过）", reason)); err != nil {
			slog.Warn("subscribe MarkChecked failed", "sub", sub.ID, "err", err)
		}
		return 0, nil
	}

	// 候选种子扁平化（带 SiteName），供规则引擎统一排序。
	// 逐站失败从前是 `if sr.Error != "" { continue }` 一声不响：10 个站里 3 个 cookie 失效时，
	// 界面上「上次检查 <时间>」照常刷新，用户读到的是「确实没有新命中」。
	// 现在把失败站点记进本轮账面（卡片 + 当天去重通知），并继续用可读的那些站的结果。
	var all []site.Torrent
	var siteFails []string
	for _, sr := range results {
		if sr.Error != "" {
			siteFails = append(siteFails, fmt.Sprintf("%s（%s）", sr.SiteName, clipRunes(sr.Error, 40)))
			continue
		}
		for _, t := range sr.Torrents {
			if t.DownloadURL == "" {
				continue
			}
			if t.SiteName == "" {
				t.SiteName = sr.SiteName // SearchSites 按站聚合，但单条 Torrent 无站名（types.go 注释）
			}
			t.SiteID = sr.SiteID
			all = append(all, t)
		}
	}
	checkErr := ""
	// 闸门开着但本轮没判（外网评分故障 / 评分模块未装配）：放行是设计，但「没被校验过」
	// 必须进本轮账面，否则低分片被推走时卡片与通知看起来跟「评分合格」一模一样。
	if gateSkip != "" {
		checkErr = gateSkip
		c.noticeStable(ctx, sub, "rating-guard-skipped", "订阅低分保护未生效："+sub.Title,
			gateSkip+"（本轮照常推送，等评分恢复后才会真正拦）", "warning")
	}
	if len(siteFails) > 0 {
		shown := siteFails
		if len(shown) > 3 {
			shown = append(append([]string{}, shown[:3]...), "…")
		}
		siteNote := fmt.Sprintf("%d/%d 个站本轮读不出来：%s", len(siteFails), len(results), strings.Join(shown, "、"))
		if checkErr != "" {
			checkErr += "；" + siteNote
		} else {
			checkErr = siteNote
		}
		// 持续型状态（站坏了会连续多轮都读不出来）：稳定 Key 当天去重，不刷屏。
		c.noticeStable(ctx, sub, "site-unavailable", "订阅检查："+sub.Title,
			checkErr+"，本轮只用上了其余站的结果", "warning")
	}
	// 标题匹配确认：对标 MP match_torrent。用订阅媒体别名集（Title+OriginalTitle+Names）精确匹配，
	// 接受集覆盖种子语言时收紧（过滤无关），无该语言别名时放行（不误杀跨语言合法命中）。
	all = filterByTitleMatch(all, sub, buildSubscribeAcceptSet(ctx, c.mediaCh, sub))
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
	// 仅官种：eff.OfficialOnly 为 true 时硬过滤（促销后/规则引擎前），只留 site.IsOfficial 命中者。
	if eff.OfficialOnly {
		all = filterByOfficial(all)
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

	// BestVersionFull（整季包优先）：剧集订阅开启时，候选稳定排序把整季包（Season Pack）置顶，
	// 同类内按 PriOrder 降序。配合下方洗版门控优先用整季包提升整季进度；若整季包未改善，
	// 单集仍有机会（回退）。字段定义见 models.Subscription.BestVersionFull。仅影响剧集订阅排序顺序，
	// 不改变去重/洗版门控语义，故 BestVersionFull=false 时行为完全不变。
	if sub.BestVersionFull && sub.Type == "tv" && len(ranked) > 1 {
		sort.SliceStable(ranked, func(i, j int) bool {
			iPack := isSeasonPackTitle(ranked[i].t.Title)
			jPack := isSeasonPackTitle(ranked[j].t.Title)
			if iPack != jPack {
				return iPack // 整季包排前
			}
			if ranked[i].pri != ranked[j].pri {
				return ranked[i].pri > ranked[j].pri // 同类内 PriOrder 降序
			}
			return ranked[i].t.SitePriority > ranked[j].t.SitePriority // 同层站优先级高者在前
		})
	} else if len(ranked) > 1 {
		// 非整季包路径：PriOrder 已由规则引擎按站优先级并列排序；此处兜底无规则场景
		// （ranked 全部 PriOrder=0）也能享受站点优先级择优。
		sort.SliceStable(ranked, func(i, j int) bool {
			return ranked[i].t.SitePriority > ranked[j].t.SitePriority
		})
	}

	// 推送（按 PriOrder 降序，去重 + 洗版门控 + 推送 + 通知）。
	pushed := 0
	epPri := loadEpisodePriority(sub.EpisodePriority) // 剧集洗版按集进度（非洗版/电影不使用）
	// 电影洗版的「是否已有基线」判据：未配规则组时候选 PriOrder 全为 0，没有画质信息可比，
	// 只能靠「推过没有」决定要不要放行首版。查询失败按「已推过」处理——宁可少推也不重复下载
	// （与下方 HitExists 出错即跳过同一策略）。
	movieHasBaseline := true
	if eff.BestVersion && sub.Type == "movie" {
		if ok, err := c.op.HasAnyHit(sub.ID); err == nil {
			movieHasBaseline = ok
		} else {
			slog.Warn("subscribe HasAnyHit failed, assume baseline present", "sub", sub.ID, "err", err)
		}
	}
	for _, cand := range ranked {
		t, pri := cand.t, cand.pri
		// 洗版门控：候选画质须严格优于已记录最优才推送（避免重复下同级/更低版）。
		if eff.BestVersion {
			if sub.Type == "movie" {
				if !interestedMovie(sub.CurrentPriority, pri, movieHasBaseline) {
					continue
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
			// 二轮 P3-3：订阅标题含逗号时 qb 按逗号拆出碎片脏标签——入列前清洗。
			Tags:   sanitizeTorrentTags("subscribe", sub.Title, transfer.EncodeMediaTag(sub.Source, sub.SourceID, sub.Type)),
			SiteID: t.SiteID,
		}
		if sub.SavePath != "" {
			opts.SavePath = sub.SavePath
		}
		if err := c.dlCh.Add(ctx, dlID, opts); err != nil {
			// 二轮 P3-4：TR 对已存在 hash 的 torrent-add 报 duplicate——种子实际
			// 已在下载器里（如用户从搜索页直推过），视作已推送记 AddHit，否则每轮
			// cron 重试+Warn 刷屏。qb add 幂等无此错误不受影响。
			if isDuplicateTorrentErr(err) {
				slog.Info("subscribe push duplicate (already in downloader), record hit", "title", t.Title)
				_ = c.op.AddHit(&models.SubscriptionHit{
					SubscriptionID: sub.ID, TorrentKey: key, Title: t.Title, SiteName: t.SiteName,
				})
			} else {
				slog.Warn("subscribe push failed", "title", sub.Title, "site", t.SiteName, "err", err)
			}
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
				// 首版落地后基线即成立：本轮及后续候选必须有画质信息且严格更优才再推
				// （否则无规则组时同一轮的多个站点会被一起放行，一次下好几份）。
				movieHasBaseline = true
			} else {
				keys, seasonPack := episodeKeys(t.Title)
				updateEpisodePriority(epPri, keys, seasonPack, pri)
			}
		}
	}

	if err := c.op.MarkCheckedWithResult(sub.ID, checkErr); err != nil {
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

// sanitizeTorrentTags 清洗进下载器的标签：去除逗号/首尾空白（qb 按逗号拆 tag，
// "Title, The" 会拆成两个脏标签；空串丢弃）。
func sanitizeTorrentTags(tags ...string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(strings.ReplaceAll(t, ",", " "))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// isDuplicateTorrentErr 判定下载器「种子已存在」类错误（TR duplicate torrent /
// qb 该 torrent already exists / Deluge already in session）。
func isDuplicateTorrentErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "already exists") ||
		strings.Contains(msg, "already in session")
}

// clipRunes 按 rune 截断（订阅卡片那一行要短；英文按字节截会切出半个字）。
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
