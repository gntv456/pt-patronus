package automation

import (
	"context"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"log/slog"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// runSnapshot 聚合站点快照任务：把原 stats / hr / freeleech / announcement / messages /
// cookie-keepalive 6 个任务对同站的分散请求，收敛为 SiteChain.SnapshotSite 一次串行抓取，
// 降低站点感知的请求频率（对齐 MoviePilot 6h 串行哲学）。每站抓取后：
//   - 用其 uid 跑一次 assessment（AssessSiteWithUID 复用 uid，访问 userdetails.php?id=<uid>，
//     不再像原 assessment-refresh 那样每整小时裸访 userdetails.php——52movie 警告根因）
//   - dispatchSnapshotSite 分派 stats 落库 / cookie 轮换 / H&R / 站免 / 公告 / 新邮件 通知
func (s *Service) runSnapshot() {
	s.jitterSleep()                                               // 随机抖动 0-180s，打散整点齐发
	ctx, cancel := context.WithTimeout(s.rootCtx, 30*time.Minute) // 4h 周期可宽松：串行 N 站每站多步 + 站间抖动，10min 在站点多时会截断
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("snapshot list sites failed", "err", err)
		// 静默返回 = 本轮什么都没采集，而界面只会显示「上次检测时间」越来越旧，
		// 看起来像站点全挂了。读库失败必须自己出声。
		if s.notif != nil {
			s.notif.Notify(ctx, notification.Message{
				Title: "站点快照", Body: "本轮聚合快照未能开始：读取站点列表失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	var agg snapshotAgg
	for i := range sites {
		st := &sites[i]
		res := s.siteCh.SnapshotSite(ctx, st)
		if s.assessCh != nil && res.UID != "" {
			s.assessCh.AssessSiteWithUID(ctx, st, res.UID)
		}
		s.dispatchSnapshotSite(ctx, st, &res, &agg)
		if i < len(sites)-1 {
			t := time.NewTimer(time.Duration(2+rand.Intn(7)) * time.Second) // 站间 2-8s，更像人类浏览
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				agg.broadcast(s, ctx)
				return
			}
		}
	}
	agg.broadcast(s, ctx)
	// 全站扫完后推送守护神名片（开启且密钥已配置时；失败不影响快照本身）
	if s.hxcardPush != nil {
		s.hxcardPush(ctx)
	}
	slog.Info("site-snapshot done", "sites", len(sites),
		"loggedOut", len(agg.loggedOut), "freeleech", len(agg.flAlerts))
}

// jitterSleep 聚合任务启动前随机 sleep 0-180s，避免多站点在整点齐发（robfig/cron 不支持随机表达式）。
func (s *Service) jitterSleep() {
	t := time.NewTimer(time.Duration(rand.Intn(180)) * time.Second)
	defer t.Stop()
	select {
	case <-t.C:
	case <-s.rootCtx.Done():
	}
}

// snapshotAgg 汇总一轮聚合跨站的广播项，延迟到全站扫完一次性推送（Cookie 掉线 / 站免）。
// H&R 由独立 hr-check cron 告警，不在此聚合。
type snapshotAgg struct {
	loggedOut []string
	flAlerts  []ws.FreeleechAlert
}

// dispatchSnapshotSite 消费单站快照结果，分派 stats 落库 + cookie 轮换 + per-site 通知
// （站免/公告/新邮件），并把汇总类项（H&R 危险 / 掉线 / 站免 WS）累积到 agg，由 broadcast 统一推送。
// 通知去重 key 与 SystemConfig baseline 完全复用原 6 个 run*，不引入新 key。
func (s *Service) dispatchSnapshotSite(ctx context.Context, st *models.Site, res *chain.SnapshotResult, agg *snapshotAgg) {
	// 0. 连通状态缓存：与 stats 落库互补——disconnected 也写，供站点运营矩阵/Dashboard 进页面
	//    瞬时回显「上次检测结果」。后台 4h 聚合快照顺带刷新缓存，保证页面不打开也有定期更新。
	if res.UserInfo != nil && res.UserErr == "" {
		_ = s.siteOp.UpsertConnectivity(st.ID, "connected", res.UserInfo.Level, "", 0)
	} else {
		_ = s.siteOp.UpsertConnectivity(st.ID, "disconnected", "", res.UserErr, 0)
	}

	// 1. stats 落库（UserInfo 成功且无错误才记录，避免部分结果写脏统计行）
	if res.UserInfo != nil && res.UserErr == "" {
		bizmetrics.SiteSyncInc(bizmetrics.SyncSuccess)
		s.recordStat(st, res.UserInfo)
	} else {
		bizmetrics.SiteSyncInc(bizmetrics.SyncFail)
	}

	// 2. cookie 掉线告警（per-site notice 当天去重）+ 轮换回写
	if res.LoggedOut {
		agg.loggedOut = append(agg.loggedOut, st.Name)
		if s.notice != nil {
			_ = s.notice.Notice(ctx, notice.Input{
				Source: notice.SourceSiteOffline,
				Key:    "cookie:" + st.Name,
				Title:  "站点掉线：" + st.Name,
				Body:   st.Name + " cookie 已失效，请尽快重新登录以免自动化停摆",
				Level:  "danger",
			}, true)
		}
	} else if res.RefreshedCookie != "" {
		if cur, _ := s.siteOp.Cookie(st); cur != res.RefreshedCookie {
			if err := s.siteOp.UpdateCookie(st.ID, res.RefreshedCookie); err != nil {
				slog.Warn("persist cookie failed", "site", st.Name, "err", err)
			}
		}
	}

	// H&R 由独立 hr-check cron 每小时检查 + 告警，聚合不处理（避免重复）。

	// 4. 站免（WS 汇总 + per-site 当天去重 notice）
	for _, n := range res.Freeleech {
		if n.Status == site.FreeleechActive {
			if s.hub != nil {
				agg.flAlerts = append(agg.flAlerts, ws.FreeleechAlert{Site: st.Name, Title: n.Title, Content: n.Content})
			}
			if s.notice != nil {
				_ = s.notice.Notice(ctx, notice.Input{
					Source: notice.SourceFreeleech,
					Key:    "freeleech:" + st.Name + ":" + time.Now().Format("2006-01-02"),
					Title:  "今日站免：" + st.Name,
					Body:   n.Title + " " + n.Content,
					Level:  "info",
				}, true)
			}
		}
	}

	// 5. 公告 hash diff（首次不通知，避免初始化刷屏）
	if s.notice != nil && s.cfgOp != nil && res.Announcement != nil && res.Announcement.RawText != "" {
		h := shortHash(res.Announcement.RawText)
		key := "announce:seen:" + st.Name
		last, _ := s.cfgOp.Get(key)
		if last != h {
			if last != "" {
				_ = s.notice.Notice(ctx, notice.Input{
					Source: notice.SourceAnnounce,
					Key:    "announce:" + st.Name + ":" + h,
					Title:  "新公告：" + st.Name,
					Body:   truncate(res.Announcement.RawText, 120),
					Level:  "info",
				}, true)
			}
			_ = s.cfgOp.Set(key, h)
		}
	}

	// 6. 新邮件 seen diff
	if s.notice != nil && s.cfgOp != nil && len(res.Messages) > 0 {
		seen := loadSeenIDs(s.cfgOp, st.Name)
		cur := make(map[uint64]struct{}, len(res.Messages))
		fresh := 0
		for i := range res.Messages {
			m := &res.Messages[i]
			cur[m.ID] = struct{}{}
			if _, ok := seen[m.ID]; !ok {
				fresh++
			}
		}
		if fresh > 0 {
			_ = s.notice.Notice(ctx, notice.Input{
				Source: notice.SourceNewMail,
				Key:    "mail:" + st.Name + ":" + time.Now().Format("2006-01-02"),
				Title:  "新邮件：" + st.Name,
				Body:   "收到 " + strconv.Itoa(fresh) + " 条新私信",
				Level:  "info",
			}, true)
		}
		saveSeenIDs(s.cfgOp, st.Name, cur)
	}
}

// broadcast 汇总推送跨站告警：Cookie 掉线 / 站免横幅（H&R 由独立 hr-check cron 告警）。
func (a *snapshotAgg) broadcast(s *Service, ctx context.Context) {
	// Cookie 掉线汇总告警（notif + WS）
	if len(a.loggedOut) > 0 {
		s.notif.Notify(ctx, notification.Message{
			Title: "Cookie 失效告警",
			Body: fmt.Sprintf("以下 %d 个站点 cookie 已失效，请尽快重新登录以免自动化停摆：%s",
				len(a.loggedOut), strings.Join(a.loggedOut, "、")),
			Level: "danger",
		})
		if s.hub != nil {
			if payload, err := ws.EncodeCookieAlert(a.loggedOut); err == nil {
				s.hub.Broadcast(payload)
			}
		}
	}

	// 站免 WS 推送（在线连接秒收）
	if s.hub != nil && len(a.flAlerts) > 0 {
		if payload, err := ws.EncodeFreeleechAlert(a.flAlerts); err == nil {
			s.hub.Broadcast(payload)
		}
	}
}

// SnapshotCronSpec 返回当前聚合快照 cron（启动时 DB 覆盖后的值），供 API 展示。
func (s *Service) SnapshotCronSpec() string { return s.cfg.SnapshotCron }

// ReloadSnapshot 运行时更新聚合快照 cron：sched.AddFunc 同名替换（robfig/cron 原子替换旧条目）
// + 持久化到 SystemConfig（schedule.site-snapshot，重启后仍生效）。spec 合法性由调用方预校验。
func (s *Service) ReloadSnapshot(spec string) error {
	if err := s.sched.AddFunc("site-snapshot", spec, s.runSnapshot); err != nil {
		return err
	}
	s.cfg.SnapshotCron = spec
	if s.cfgOp != nil {
		_ = s.cfgOp.Set("schedule.site-snapshot", spec)
	}
	slog.Info("site-snapshot cron reloaded", "spec", spec)
	return nil
}

// KeepAliveCronSpec 返回当前 cookie 保活 cron。
func (s *Service) KeepAliveCronSpec() string { return s.cfg.CookieKeepAliveCron }

// ReloadKeepAlive 运行时更新 cookie-keepalive cron + 持久化 schedule.cookie-keepalive。
func (s *Service) ReloadKeepAlive(spec string) error {
	if err := s.sched.AddFunc("cookie-keepalive", spec, s.runCookieKeepAlive); err != nil {
		return err
	}
	s.cfg.CookieKeepAliveCron = spec
	if s.cfgOp != nil {
		_ = s.cfgOp.Set("schedule.cookie-keepalive", spec)
	}
	slog.Info("cookie-keepalive cron reloaded", "spec", spec)
	return nil
}
