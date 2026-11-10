// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"context"
	"errors"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"log/slog"
	"math/rand"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
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
		// 静默 = 保活整轮跳过，短寿 cookie 会在快照间隙过期，读失败必须自己出声（当天去重）。
		if s.notice != nil {
			_ = s.notice.Notice(ctx, notice.Input{
				Source: notice.SourceSiteOffline,
				Key:    "keepalive-listfail:" + time.Now().Format("2006-01-02"),
				Title:  "Cookie 保活",
				Body:   "本轮保活未能开始：读取站点列表失败 — " + err.Error(),
				Level:  "warning",
			}, true)
		}
		return
	}

	var refreshed, loggedOut, failed int
	for i := range sites {
		st := &sites[i]
		res, err := s.siteCh.KeepAlive(ctx, st)
		if err != nil {
			// 不支持保活的站（mteam 等未实现 KeepAliveCapable）不算失败：否则 fail 指标
			// 每小时被非 NexusPHP 站刷高，Grafana 保活失败告警误报。
			if errors.Is(err, chain.ErrNotSupported) || errors.Is(err, chain.ErrNoAdapter) {
				slog.Debug("cookie-keepalive skipped: unsupported", "site", st.Name)
				continue
			}
			failed++
			bizmetrics.KeepAliveInc(bizmetrics.KeepAliveFail)
			slog.Debug("cookie-keepalive failed", "site", st.Name, "err", err)
		} else if res != nil {
			if !res.LoggedIn {
				loggedOut++
				bizmetrics.KeepAliveInc(bizmetrics.KeepAliveLoggedOut)
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
				bizmetrics.KeepAliveInc(bizmetrics.KeepAliveRefreshed)
				if err := s.siteOp.UpdateCookie(st.ID, res.RefreshedCookie); err != nil {
					failed++
					bizmetrics.KeepAliveInc(bizmetrics.KeepAliveFail)
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
