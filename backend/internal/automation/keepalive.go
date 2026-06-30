package automation

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// runCookieKeepAlive 遍历启用站点做认证 GET：
//   - 刷新服务端 session（防 cookie 因长期不活动过期）
//   - 捕获响应 Set-Cookie 轮换后的最新 cookie，回写 DB
//   - 检测失效（重定向到登录页等）→ danger 通知，提示重新登录
//
// cookie 过期会使签到/统计/H&R 全部自动化失效，故这是最高优先级保命任务。
func (s *Service) runCookieKeepAlive() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	sites, err := s.siteOp.ListActive()
	if err != nil {
		slog.Warn("keepalive list sites failed", "err", err)
		return
	}
	var loggedOut []string
	refreshed := 0
	for i := range sites {
		st := &sites[i]
		res, err := s.siteCh.KeepAlive(ctx, st)
		if err != nil {
			// 网络/适配器不支持：非登录态问题，debug 记录，不告警
			slog.Debug("keepalive skipped", "site", st.Name, "err", err)
			continue
		}
		if !res.LoggedIn {
			loggedOut = append(loggedOut, st.Name)
			slog.Warn("cookie expired", "site", st.Name)
			// 通知中心：每站当天去重一条「站点掉线」
			if s.notice != nil {
				s.notice.Notice(ctx, notice.Input{
					Source: notice.SourceSiteOffline,
					Key:    "cookie:" + st.Name,
					Title:  "站点掉线：" + st.Name,
					Body:   st.Name + " cookie 已失效，请尽快重新登录以免自动化停摆",
					Level:  "danger",
				}, true)
			}
			continue
		}
		// 服务端轮换了 cookie → 回写最新值
		if res.RefreshedCookie != "" {
			if cur, _ := s.siteOp.Cookie(st); cur != res.RefreshedCookie {
				if err := s.siteOp.UpdateCookie(st.ID, res.RefreshedCookie); err != nil {
					slog.Warn("persist cookie failed", "site", st.Name, "err", err)
				} else {
					refreshed++
				}
			}
		}
	}

	if len(loggedOut) > 0 {
		s.notif.Notify(s.rootCtx, notification.Message{
			Title: "Cookie 失效告警",
			Body: fmt.Sprintf("以下 %d 个站点 cookie 已失效，请尽快重新登录以免自动化停摆：%s",
				len(loggedOut), strings.Join(loggedOut, "、")),
			Level: "danger",
		})
		// 实时推送：在线 WS 连接秒收，移动端首屏即可见。
		if s.hub != nil {
			if payload, err := ws.EncodeCookieAlert(loggedOut); err == nil {
				s.hub.Broadcast(payload)
			}
		}
	}
	slog.Info("cookie keepalive done", "checked", len(sites), "refreshed", refreshed, "loggedOut", len(loggedOut))
}
