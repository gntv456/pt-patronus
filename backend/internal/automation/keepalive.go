package automation

import (
	"context"
	"log/slog"
	"math/rand"
	"time"

	"github.com/hxpt/ptpatronus/internal/notice"
)

// runCookieKeepAlive 每小时对启用站点做一次 KeepAlive：
//   - 刷新服务端 session（根治 Laravel qw_session 等短寿 cookie 在 4h 快照间隙过期）
//   - 合并并回写轮换后的 cookie
//   - 掉线时写站内通知（当天去重）
//
// 与 site-snapshot 内嵌的 KeepAlive 互补：本任务频率更高、职责更单一。
func (s *Service) runCookieKeepAlive() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 20*time.Minute)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("cookie-keepalive list sites failed", "err", err)
		return
	}

	var refreshed, loggedOut, failed int
	for i := range sites {
		st := &sites[i]
		res, err := s.siteCh.KeepAlive(ctx, st)
		if err != nil {
			failed++
			slog.Debug("cookie-keepalive failed", "site", st.Name, "err", err)
		} else if res != nil {
			if !res.LoggedIn {
				loggedOut++
				if s.notice != nil {
					s.notice.Notice(ctx, notice.Input{
						Source: notice.SourceSiteOffline,
						Key:    "cookie:" + st.Name,
						Title:  "站点掉线：" + st.Name,
						Body:   st.Name + " cookie 已失效，请尽快重新登录以免自动化停摆",
						Level:  "danger",
					}, true)
				}
			} else if res.RefreshedCookie != "" {
				if err := s.siteOp.UpdateCookie(st.ID, res.RefreshedCookie); err != nil {
					failed++
					slog.Warn("cookie-keepalive persist failed", "site", st.Name, "err", err)
				} else {
					refreshed++
				}
			}
		}

		if i < len(sites)-1 {
			t := time.NewTimer(time.Duration(1+rand.Intn(3)) * time.Second)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				slog.Info("cookie-keepalive interrupted", "done", i+1, "total", len(sites),
					"refreshed", refreshed, "loggedOut", loggedOut, "failed", failed)
				return
			}
		}
	}
	slog.Info("cookie-keepalive done", "sites", len(sites),
		"refreshed", refreshed, "loggedOut", loggedOut, "failed", failed)
}
