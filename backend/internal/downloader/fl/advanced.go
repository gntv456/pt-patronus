// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package fl

// advanced.go 实现 Flood (jesec/flood) 的高级操作，接口面同 qb/tr。
// API 参考：https://github.com/jesec/flood （/server/routes/api/torrents.ts、client.ts）。
//
// jesec 路径与 legacy Flood-UI 不同：本文件面向 jesec（主流分叉），
// 遇 404 回退 downloader.ErrNotImplemented（兼容 legacy 探测）。
// Flood 基于 rTorrent，部分 qBittorrent 概念无对应端点（见各方法注释）。

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 生命周期 ——

// Recheck 强制重新校验 → POST /api/torrents/check-hash {hashes}。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	return c.postHashesJesec(ctx, "/api/torrents/check-hash", hashes)
}

// Reannounce 重新汇报 → POST /api/torrents/reannounce {hashes}。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	return c.postHashesJesec(ctx, "/api/torrents/reannounce", hashes)
}

// SetLocation 移动保存路径 → POST /api/torrents/move。
// jesec moveTorrentsSchema 要求 moveFiles/isBasePath/isCheckHash 三字段（strictObject 必填）。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	payload := map[string]any{
		"hashes":      upperAll(hashes),
		"destination": location,
		"moveFiles":   true,  // 移动数据而非复制
		"isBasePath":  false, // destination 为目标目录（非 base path）
		"isCheckHash": false, // 移动后不强制重新校验
	}
	status, _, err := c.raw(ctx, http.MethodPost, "/api/torrents/move",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("move status %d", status)
	}
	return nil
}

// SetForceStart 强制开始/取消强制。
// Flood（rTorrent）无"强制开始"概念：rTorrent 只有 start/stop，无视队列的
// force-start 开关未通过 Flood REST 暴露。保持未实现。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	return downloader.ErrNotImplemented
}

// —— 分类 / 标签 ——

// ListCategories Flood 无分类实体，按首个 tag 聚合近似（tags[0] 视为分类）。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		if t.Category != "" && !seen[t.Category] {
			seen[t.Category] = true
			out = append(out, t.Category)
		}
	}
	return out, nil
}

// ListTags 从任务列表聚合所有 tag。
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, t := range list {
		for _, tag := range t.Tags {
			if tag != "" && !seen[tag] {
				seen[tag] = true
				out = append(out, tag)
			}
		}
	}
	return out, nil
}

// SetTags 覆盖式设置标签 → jesec PATCH /api/torrents/tags {hashes, tags}。
// jesec 该端点为 SET 语义（整体覆盖），正好匹配 SetTags 契约。
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	payload := map[string]any{"hashes": upperAll(hashes), "tags": tags}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/tags",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set tags status %d", status)
	}
	return nil
}

// AddTags 追加标签。jesec 无原生 add 端点（只有覆盖式 SET），按每任务 diff 实现：
// 读当前 tags → 并入新 tags → SetTags 覆盖。不同任务原 tag 不同故逐个设置。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	cur, err := c.tagsByHashes(ctx, hashes)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		merged := unionStrings(cur[strings.ToLower(h)], tags)
		if err := c.SetTags(ctx, []string{h}, merged); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTags 移除标签。同样按每任务 diff：读当前 → 减去要移除的 → SetTags 剩余。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	cur, err := c.tagsByHashes(ctx, hashes)
	if err != nil {
		return err
	}
	for _, h := range hashes {
		remain := subtractStrings(cur[strings.ToLower(h)], tags)
		if err := c.SetTags(ctx, []string{h}, remain); err != nil {
			return err
		}
	}
	return nil
}

// CreateCategory / EditCategory / DeleteCategories：Flood 无分类实体。
// 分类即 tags；Flood 的 torrentDestinations 设置仅是 tag→目录的弱映射，
// 非 qB 意义上的分类 CRUD（无法仅凭标签新建/编辑/删除带保存路径的分类）。保持未实现。
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	return downloader.ErrNotImplemented
}

func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	return downloader.ErrNotImplemented
}

// —— 做种限制 ——

// SetShareLimits 设置分享比率/做种时长上限。
// Flood REST 未暴露分享比率/做种时长限制端点；rTorrent 底层经 XML-RPC（views/group）
// 支持但 Flood REST 不开放。保持未实现（ 亦未对 Flood 实现）。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	return downloader.ErrNotImplemented
}

// —— 队列优先级 ——

// SetQueuePriority 调整优先级 → jesec PATCH /api/torrents/priority {hashes, priority}。
// jesec TorrentPriority 取值 0-3（0=不下载 1=低 2=正常 3=高）。
// 接口 action∈top|up|down|bottom 映射为绝对优先级；Flood 无相对位移概念，
// 故 up/top 同为高、down/bottom 区分正常/低（caps.queueFull=false 反映此限制）。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	var prio int
	switch action {
	case "top", "up":
		prio = 3
	case "down":
		prio = 2
	case "bottom":
		prio = 1
	default:
		return fmt.Errorf("invalid queue action: %s", action)
	}
	payload := map[string]any{"hashes": upperAll(hashes), "priority": prio}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/priority",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("priority status %d", status)
	}
	return nil
}

// —— 顺序下载 / 首尾优先 ——

// ToggleSequentialDownload 切换顺序下载。
// jesec 端点为 SET（非 toggle）：PATCH /api/torrents/sequential {hashes, isSequential}。
// 实现按每任务当前状态翻转：当前开→关、当前关→开，分组各发一次。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return err
	}
	state := make(map[string]bool, len(list))
	for _, t := range list {
		state[t.Hash] = t.Sequential
	}
	var on, off []string
	for _, h := range hashes {
		lh := strings.ToLower(h)
		if state[lh] {
			on = append(on, lh) // 当前顺序开 → 翻转为关
		} else {
			off = append(off, lh) // 当前关 → 翻转为开
		}
	}
	if len(off) > 0 {
		if err := c.setSequential(ctx, off, true); err != nil {
			return err
		}
	}
	if len(on) > 0 {
		if err := c.setSequential(ctx, on, false); err != nil {
			return err
		}
	}
	return nil
}

// setSequential jesec PATCH /api/torrents/sequential {hashes, isSequential}。
func (c *Client) setSequential(ctx context.Context, hashes []string, enable bool) error {
	payload := map[string]any{"hashes": upperAll(hashes), "isSequential": enable}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/sequential",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("sequential status %d", status)
	}
	return nil
}

// ToggleFirstLastPiece 切换首尾优先。
// Flood REST 未暴露首尾优先端点（rTorrent 内部支持但 Flood REST 不开放）。保持未实现。
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return downloader.ErrNotImplemented
}

// —— 限速（bytes/s，0=不限）——

// SetLimits 设置单/多个任务上下行限速。
// Flood REST 无单种子限速端点（仅全局 throttleGlobal* 经 /api/client/settings）。
// 保持未实现。
func (c *Client) SetLimits(ctx context.Context, hashes []string, limits downloader.Limits) error {
	return downloader.ErrNotImplemented
}
