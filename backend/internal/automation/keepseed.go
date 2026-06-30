package automation

import (
	"context"
	"log/slog"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
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
		return
	}
	if len(settings) == 0 {
		return // 无下载器启用，零动作
	}

	for _, st := range settings {
		created, err := s.keepSeed.Reseed(ctx, st.DownloaderID)
		if err != nil {
			slog.Warn("auto-reseed failed", "downloader", st.DownloaderID, "err", err)
			continue
		}
		added := 0
		if st.AutoReseedAdd {
			for _, t := range created {
				if err := s.keepSeed.Resolve(ctx, t.ID, "add"); err != nil {
					slog.Warn("auto-reseed add failed", "task", t.ID, "err", err)
					continue
				}
				added++
			}
		}
		slog.Info("auto-reseed done", "downloader", st.DownloaderID, "new", len(created), "autoAdded", added)
	}
}

