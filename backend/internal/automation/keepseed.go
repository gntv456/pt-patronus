// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// keepSeedRunner 是定时辅种所需的最小 KeepSeedChain 能力（解耦具体类型，便于测试注入 fake）。
// *chain.KeepSeedChain 天然满足。
type keepSeedRunner interface {
	ListAutoReseedSettings() ([]models.CrossSeedSetting, error)
	Reseed(ctx context.Context, downloaderID uint) ([]models.CrossSeedTask, error)
	Resolve(ctx context.Context, taskID uint, action string) error
}

// runKeepSeed 定时自动辅种：遍历启用 AutoReseed 的下载器重跑 IYUU 精确匹配（Reseed），
// 刷新 pending 候选；该下载器开启 AutoReseedAdd 则自动把新候选添加辅种（Resolve "add"）。
//
// 默认非破坏：无下载器启用 AutoReseed 即零动作；auto-add 默认关。单下载器失败不致命（continue）。
func (s *Service) runKeepSeed() {
	ctx, cancel := context.WithTimeout(s.rootCtx, 10*time.Minute)
	defer cancel()

	settings, err := s.keepSeed.ListAutoReseedSettings()
	if err != nil {
		slog.Warn("auto-reseed list settings failed", "err", err)
		// 静默 = 自动辅种整轮不跑、pending 候选停更，读失败必须出声。
		if s.notif != nil {
			s.notif.Notify(ctx, notification.Message{
				Title: "自动辅种", Body: "本轮辅种未能开始：读取辅种设置失败 — " + err.Error(), Level: "warning",
			})
		}
		return
	}
	if len(settings) == 0 {
		return // 无下载器启用，零动作
	}

	for _, st := range settings {
		created, err := s.keepSeed.Reseed(ctx, st.DownloaderID)
		if err != nil {
			slog.Warn("auto-reseed failed", "downloader", st.DownloaderID, "err", err)
			// #352 与上面「读设置失败」同一口径：静默 = 这台下载器整轮不跑、pending 候选停更，
			// 用户在界面上看不到任何东西。逐台 continue 不代表可以不说话。
			s.notifyKeepSeed(ctx, "本轮自动辅种未能完成：下载器（编号 "+strconv.FormatUint(uint64(st.DownloaderID), 10)+"）辅种失败 — "+err.Error())
			continue
		}
		added := 0
		if st.AutoReseedAdd {
			var addFail int
			var addWhy string
			for _, t := range created {
				if err := s.keepSeed.Resolve(ctx, t.ID, "add"); err != nil {
					slog.Warn("auto-reseed add failed", "task", t.ID, "err", err)
					addFail++
					if addWhy == "" {
						addWhy = err.Error()
					}
					continue
				}
				added++
			}
			// 逐条 add 失败同样要出声，否则「自动辅种开着却什么都没加」无从排查。
			if addFail > 0 {
				s.notifyKeepSeed(ctx, "自动辅种未能添加 "+strconv.Itoa(addFail)+" 个候选（成功 "+strconv.Itoa(added)+" 个）— "+addWhy)
			}
		}
		slog.Info("auto-reseed done", "downloader", st.DownloaderID, "new", len(created), "autoAdded", added)
	}
}

// notifyKeepSeed 统一出口：自动辅种的失败要出声（#352）。与上面「读设置失败」那处同口径。
// 注：与 keepalive 的 notice 通道不同，这里不做当天去重——辅种轮次本身是低频的，
// 逐台失败逐台报反而能看出是哪台在坏；要防刷屏得在通知中心侧做，不在本函数。
func (s *Service) notifyKeepSeed(ctx context.Context, body string) {
	if s.notif == nil {
		return
	}
	_ = s.notif.Notify(ctx, notification.Message{
		Title: "自动辅种", Body: body, Level: "warning",
	})
}
