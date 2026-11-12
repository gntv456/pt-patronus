// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
	"gorm.io/gorm"
)

// buildCandidateURL 拼候选下载 URL：
//   - MTeam：存哨兵 mteam:<id>（真 URL 由 Resolve 现取 genDlToken，避免入库后时效失效），
//     需本地已配 api_key（否则 false 跳过）。
//   - 其它站：passkey 直链（iyuu.BuildDownloadURL），需 passkey（否则 false 跳过）。
func (c *KeepSeedChain) buildCandidateURL(local *models.Site, iyuuSite iyuu.SiteInfo, torrentID int) (string, bool, error) {
	// #384 从前是 (string, bool) 且内部两处都是「错误丢掉 + 拿空串当没配」：
	//   apiKey, _ := c.siteOp.APIKey(local); if apiKey == "" { return "", false }
	//   passkey := c.siteOp.DecryptedPasskey(local); if passkey == "" { return "", false }
	// 而 reseed 的调用方（keepseed_reseed.go:88-91）拿到 !ok 就 `continue`——
	// 于是「用户压根没填凭据」与「填了但本地解不开（主密钥换过/密文损坏）」在界面上
	// 完全同形：辅种跑完只显示「没匹配到可辅种的种子」，没有任何地方说读不出凭据。
	// 两者下一步完全不同：前者去站点设置里补，后者得去重存凭据/找回旧密钥。
	// 现在把 error 交出去（与 #377/#383 同一哨兵），由上层决定是报出来还是继续。
	if local.Platform == "mteam" {
		apiKey, err := c.siteOp.APIKey(local)
		if err != nil {
			return "", false, siteCredUnreadable(local.Name, "APIKey", err)
		}
		if apiKey == "" {
			return "", false, nil // MTeam 无 api_key 无法 genDlToken（真·没配，正常跳过）
		}
		return mteamURLPrefix + strconv.Itoa(torrentID), true, nil
	}
	passkey, err := c.siteOp.Passkey(local)
	if err != nil {
		return "", false, siteCredUnreadable(local.Name, "Passkey", err)
	}
	if passkey == "" {
		return "", false, nil
	}
	return iyuu.BuildDownloadURL(iyuuSite, torrentID, passkey), true, nil
}

// #369 处理候选的三条「用户自己能修」的原因，各立一个哨兵：
// 从前它们是裸 errors.New / fmt.Errorf，接口层无法与「内部故障」区分，于是要么
// 全被压成通用话术（让用户去改 HTTP 表单），要么无差别透传（把内部原文也泄出去）。
// 有了哨兵，接口层就能按 ErrMissingNoToken 同构的方式精确放行这几条，
// 其余错误仍按原分类归 400 且不外泄内部原文（keepseed_errclass_test.go 钉着这条界）。
var (
	// ErrNoMTeamClient MTeam 客户端没注入（部署侧问题，重试无用）。
	ErrNoMTeamClient = errors.New("MTeam 辅种不可用：未注入 mteam 客户端")
	// ErrMTeamNoAPIKey 目标站没配 MTeam api_key（该去站点里补）。
	ErrMTeamNoAPIKey = errors.New("MTeam 站未配置 api_key：请在该站点设置里填入 MTeam API Key")
	// ErrBadAction action 不在 add/skip 之内（用户自己填错了）。
	ErrBadAction = errors.New("不支持的操作：只支持 add（添加辅种）或 skip（跳过）")
)

// resolveCandidateURL 取添加用的真实 URL：MTeam 哨兵 → 现取 genDlToken；其它原样返回。
func (c *KeepSeedChain) resolveCandidateURL(ctx context.Context, task *models.CrossSeedTask) (string, error) {
	if !strings.HasPrefix(task.CandidateURL, mteamURLPrefix) {
		return task.CandidateURL, nil
	}
	if c.mt == nil {
		return "", ErrNoMTeamClient
	}
	st, err := c.siteOp.Get(task.SiteID)
	if err != nil {
		// #369 从前是 `load site %d: %w`：纯英文句式。用户看到的是
		// 「load site 7: sql: database is closed」——既不知道是哪个站，也不知道该做什么。
		return "", fmt.Errorf("取目标站点（编号 %d）失败：%w", task.SiteID, err)
	}
	apiKey, err := c.siteOp.APIKey(st)
	if err != nil {
		return "", err
	}
	if apiKey == "" {
		return "", ErrMTeamNoAPIKey
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
		// 否则用站点 cookie 下载 .torrent 再以文件方式推（同 DownloadChain.Add）。
		if task.SiteID != 0 && c.siteCh != nil {
			if content, _, ferr := c.siteCh.FetchTorrentFileByID(ctx, task.SiteID, url); ferr == nil {
				opts.FileContent, opts.URL = content, ""
			} else if !errors.Is(ferr, ErrDelegatedToDownloader) {
				task.Status = "failed"
				task.ResolvedAt = &now
				if e := c.db.Save(&task).Error; e != nil {
					slog.Error("keepseed mark failed (fetch) persist error", "task_id", task.ID, "err", e)
				}
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
		// 先落库后通知：若 Save 失败而先发了「辅种完成」，用户会收到成功通知但任务状态
		// 仍是 pending（下轮 Reseed 视为进行中跳过），通知与真实状态矛盾。
		if err := c.db.Save(&task).Error; err != nil {
			slog.Error("keepseed mark added persist error", "task_id", task.ID, "err", err)
			return err
		}
		if c.notice != nil {
			body := "已添加 " + task.CandidateSite + " 的辅种种子到下载器"
			// PauseAfterAdd（默认开）：种子以暂停态加入，不恢复做种就没有上传——
			// 不提示会让用户误以为辅种已生效。
			if opts.Paused {
				body += "（暂停中，需在下载器恢复做种）"
			}
			_ = c.notice.Notice(ctx, notice.Input{
				Source: notice.SourceReseedDone,
				Title:  "辅种完成：" + task.SourceName,
				Body:   body,
				Level:  "success",
			}, false)
		}
		return nil
	default:
		// #369 从前是 `invalid action: ` + action（纯英文）。action 由请求体进来，
		// 用户只会填 add/skip；填错时该告诉他可填什么，而不是甩一句英文。
		return fmt.Errorf("%w（收到 %q）", ErrBadAction, action)
	}
}

// KeepseedTaskLimit 候选任务列表的单次返回上限（按创建时间倒序取最近若干条）。
// 导出给 API 层判定 truncated，避免上限在两处各写一份而漂移。
const KeepseedTaskLimit = 500

// taskQuery 组装候选任务的筛选条件。Count 与 List 共用一份，避免两处口径漂移。
func (c *KeepSeedChain) taskQuery(status string, siteID uint) *gorm.DB {
	q := c.db.Model(&models.CrossSeedTask{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if siteID > 0 {
		q = q.Where("site_id = ?", siteID)
	}
	return q
}

// CountTasks 同一筛选条件下的真实条数。列表有上限，界面要说得出「一共多少条」，
// 否则 1300 条候选只显示「候选任务（500）」，清空这 500 条后还会谎报「没有待处理候选」。
func (c *KeepSeedChain) CountTasks(status string, siteID uint) (int64, error) {
	var n int64
	if err := c.taskQuery(status, siteID).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// ListTasks 列出辅种候选任务（status 空则全部；siteID>0 时按候选站 site_id 过滤），按创建时间倒序。
// 最多 KeepseedTaskLimit 条——真实总数用 CountTasks 取。
func (c *KeepSeedChain) ListTasks(status string, siteID uint) ([]models.CrossSeedTask, error) {
	var out []models.CrossSeedTask
	if err := c.taskQuery(status, siteID).Order("created_at DESC").Limit(KeepseedTaskLimit).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// loadSetting 取该下载器的辅种设置（save_path 覆盖 / 添加后暂停）；无记录返默认值
