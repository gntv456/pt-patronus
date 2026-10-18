package automation

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/notification"
)

// runRegistration 探测全部注册监控目标的 signup 页。
// 仅在状态由「关闭/从未检测」转为「开放」时通知——靠 last_open 去重，避免每轮重复打扰。
func (s *Service) runRegistration() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 5*time.Minute)
	defer cancel()

	opened, err := s.regCh.CheckAllAndUpdate(ctx)
	if err != nil {
		// 静默 = 开放提醒永远不再响，用户只会以为没有站开放。读失败必须自己出声。
		slog.Warn("registration check failed", "err", err)
		if s.notif != nil {
			s.notif.Notify(ctx, notification.Message{
				Title: "注册开放检测", Body: "本轮检测未能开始：读取监控目标失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	if len(opened) == 0 {
		slog.Debug("registration check done, no newly opened sites")
		return
	}
	s.notif.Notify(s.rootCtx, notification.Message{
		Title: "注册开放提醒",
		Body:  "以下站点已开放注册，抓紧抢号：\n" + strings.Join(opened, "\n"),
		Level: "celebration",
	})
}
