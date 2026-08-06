package chain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/iyuu"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// buildCandidateURL 拼候选下载 URL：
//   - MTeam：存哨兵 mteam:<id>（真 URL 由 Resolve 现取 genDlToken，避免入库后时效失效），
//     需本地已配 api_key（否则 false 跳过）。
//   - 其它站：passkey 直链（iyuu.BuildDownloadURL），需 passkey（否则 false 跳过）。
func (c *KeepSeedChain) buildCandidateURL(local *models.Site, iyuuSite iyuu.SiteInfo, torrentID int) (string, bool) {
	if local.Platform == "mteam" {
		apiKey, _ := c.siteOp.APIKey(local)
		if apiKey == "" {
			return "", false // MTeam 无 api_key 无法 genDlToken
		}
		return mteamURLPrefix + strconv.Itoa(torrentID), true
	}
	passkey := c.siteOp.DecryptedPasskey(local)
	if passkey == "" {
		return "", false
	}
	return iyuu.BuildDownloadURL(iyuuSite, torrentID, passkey), true
}

// resolveCandidateURL 取添加用的真实 URL：MTeam 哨兵 → 现取 genDlToken；其它原样返回。
func (c *KeepSeedChain) resolveCandidateURL(ctx context.Context, task *models.CrossSeedTask) (string, error) {
	if !strings.HasPrefix(task.CandidateURL, mteamURLPrefix) {
		return task.CandidateURL, nil
	}
	if c.mt == nil {
		return "", errors.New("MTeam 辅种不可用：未注入 mteam 客户端")
	}
	st, err := c.siteOp.Get(task.SiteID)
	if err != nil {
		return "", fmt.Errorf("load site %d: %w", task.SiteID, err)
	}
	apiKey, err := c.siteOp.APIKey(st)
	if err != nil {
		return "", err
	}
	if apiKey == "" {
		return "", errors.New("MTeam 站未配置 api_key")
	}
	return c.mt.GenDlToken(ctx, st.BaseURL, apiKey, strings.TrimPrefix(task.CandidateURL, mteamURLPrefix))
}

// Resolve 处理一个候选任务：add（添加辅种到下载器）/ skip（标记跳过）。
func (c *KeepSeedChain) Resolve(ctx context.Context, taskID uint, action string) error {
	var task models.CrossSeedTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		return err
	}
	now := time.Now()
	switch action {
	case "skip":
		task.Status = "skipped"
		task.ResolvedAt = &now
		return c.db.Save(&task).Error
	case "add":
		dl, err := c.dl.Get(ctx, task.DownloaderID)
		if err != nil {
			return err
		}
		url, err := c.resolveCandidateURL(ctx, &task)
		if err != nil {
			task.Status = "failed"
			task.ResolvedAt = &now
			if e := c.db.Save(&task).Error; e != nil {
				// 状态落库失败会让失败任务被当作未处理而反复重试。
				slog.Error("keepseed mark failed (resolve) persist error", "task_id", task.ID, "err", e)
			}
			return err
		}
		setting := c.loadSetting(task.DownloaderID)
		opts := downloader.AddOptions{
			URL:      url,
			SavePath: task.SavePath,
			Paused:   setting.PauseAfterAdd,
		}
		// 辅种 URL 多为 passkey/MTeam token 直链（FetchTorrentFileByID 按 schema 自动委托）；
		// 否则用站点 cookie 下载 .torrent 再以文件方式推（对标 DownloadChain.Add）。
		if task.SiteID != 0 && c.siteCh != nil {
			if content, _, ferr := c.siteCh.FetchTorrentFileByID(ctx, task.SiteID, url); ferr == nil {
				opts.FileContent, opts.URL = content, ""
			} else if !errors.Is(ferr, ErrDelegatedToDownloader) {
				task.Status = "failed"
				task.ResolvedAt = &now
				_ = c.db.Save(&task).Error
				return ferr
			}
		}
		if err := dl.AddTorrent(ctx, opts); err != nil {
			task.Status = "failed"
			task.ResolvedAt = &now
			if e := c.db.Save(&task).Error; e != nil {
				slog.Error("keepseed mark failed (add) persist error", "task_id", task.ID, "err", e)
			}
			return err
		}
		task.Status = "added"
		task.ResolvedAt = &now
		if c.notice != nil {
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceReseedDone,
				Title:  "辅种完成：" + task.SourceName,
				Body:   "已添加 " + task.CandidateSite + " 的辅种种子到下载器",
				Level:  "success",
			}, false)
		}
		return c.db.Save(&task).Error
	default:
		return errors.New("invalid action: " + action)
	}
}

// ListTasks 列出辅种候选任务（status 空则全部；siteID>0 时按候选站 site_id 过滤），按创建时间倒序。
func (c *KeepSeedChain) ListTasks(status string, siteID uint) ([]models.CrossSeedTask, error) {
	var out []models.CrossSeedTask
	q := c.db.Model(&models.CrossSeedTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if siteID > 0 {
		q = q.Where("site_id = ?", siteID)
	}
	if err := q.Order("created_at DESC").Limit(500).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// loadSetting 取该下载器的辅种设置（save_path 覆盖 / 添加后暂停）；无记录返默认值
