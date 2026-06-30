package automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/ws"
)

// runFreeleech 刷新所有站点全站免费/促销缓存（供前端秒读）；正在站免额外 WS 推送。
// 不做按次通知（站免期会持续多日，每次推送会刷屏）——通知由前端 WS 在线接收 + 列表展示。
func (s *Service) runFreeleech() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	results := s.siteCh.FreeleechAll(ctx, true)
	if s.hub != nil {
		var alerts []ws.FreeleechAlert
		for _, r := range results {
			for _, n := range r.Notices {
				if n.Status == site.FreeleechActive {
					alerts = append(alerts, ws.FreeleechAlert{
						Site: r.SiteName, Title: n.Title, Content: n.Content,
					})
					// 通知中心：每个站免站点当天去重一条
					if s.notice != nil {
						s.notice.Notice(ctx, notice.Input{
							Source: notice.SourceFreeleech,
							Key:    "freeleech:" + r.SiteName + ":" + time.Now().Format("2006-01-02"),
							Title:  "今日站免：" + r.SiteName,
							Body:   n.Title + " " + n.Content,
							Level:  "info",
						}, true)
					}
				}
			}
		}
		if len(alerts) > 0 {
			if payload, err := ws.EncodeFreeleechAlert(alerts); err == nil {
				s.hub.Broadcast(payload)
			}
		}
	}
	slog.Debug("freeleech cache refreshed", "sites", len(results))
}

// runAnnouncements 刷新所有站点公告缓存（供前端秒读）；公告正文 hash 变化 → 通知。
func (s *Service) runAnnouncements() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()
	results := s.siteCh.AnnouncementsAll(ctx, true)
	// 公告正文 hash diff：变化才通知；首次（last 为空）不通知，避免初始化刷屏。
	if s.notice != nil && s.cfgOp != nil {
		for _, r := range results {
			if r.Error != "" || r.RawText == "" {
				continue
			}
			h := shortHash(r.RawText)
			key := "announce:seen:" + r.SiteName
			last, _ := s.cfgOp.Get(key)
			if last != h {
				if last != "" {
					s.notice.Notice(ctx, notice.Input{
						Source: notice.SourceAnnounce,
						Key:    "announce:" + r.SiteName + ":" + h,
						Title:  "新公告：" + r.SiteName,
						Body:   truncate(r.RawText, 120),
						Level:  "info",
					}, true)
				}
				_ = s.cfgOp.Set(key, h)
			}
		}
	}
	slog.Debug("announcement cache refreshed", "sites", len(results))
}
