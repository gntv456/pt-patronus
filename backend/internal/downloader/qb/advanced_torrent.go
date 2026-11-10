// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package qb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// —— 文件优先级 ——

// ListFiles 列出某任务内文件。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/files?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var fs []struct {
		Index    int     `json:"index"`
		Name     string  `json:"name"`
		Size     int64   `json:"size"`
		Progress float64 `json:"progress"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &fs); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(fs))
	for _, f := range fs {
		out = append(out, downloader.TorrentFile{
			Index: f.Index, Name: f.Name, Size: f.Size, Progress: f.Progress, Priority: f.Priority,
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级。qB 字段名为 id（值为逗号分隔 file index），priority ∈ {0,1,2,7}。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	ids := make([]string, 0, len(indices))
	for _, i := range indices {
		ids = append(ids, strconv.Itoa(i))
	}
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("id", strings.Join(ids, ","))
	form.Set("priority", strconv.Itoa(priority))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/filePrio",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("filePrio status %d", status)
	}
	return nil
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker（过滤 qB 返回的 ** [DHT]/** [PEX]/** [LSD] 虚拟项）。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/torrents/trackers?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var raw []struct {
		URL    string `json:"url"`
		Tier   int    `json:"tier"`
		Status int    `json:"status"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	out := make([]downloader.Tracker, 0, len(raw))
	for _, t := range raw {
		if strings.HasPrefix(t.URL, "**") {
			continue
		}
		out = append(out, downloader.Tracker{URL: t.URL, Tier: t.Tier, Status: trackerStatus(t.Status)})
	}
	return out, nil
}

func trackerStatus(s int) string {
	switch s {
	case 0:
		return "disabled"
	case 1:
		return "not_contacted"
	case 2:
		return "working"
	case 3:
		return "updating"
	case 4:
		return "not_working"
	default:
		return "unknown"
	}
}

// AddTrackers 追加 tracker（urls 换行分隔）。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("urls", strings.Join(urls, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/addTrackers",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("addTrackers status %d", status)
	}
	return nil
}

// RemoveTrackers 移除 tracker（urls 换行分隔）。
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("urls", strings.Join(urls, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeTrackers",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeTrackers status %d", status)
	}
	return nil
}

// —— 做种限制 ——

// SetShareLimits 设置分享比率/做种时长上限。
// qB setShareLimits：ratioLimit/seedingTimeLimit 均支持 -2=无限 -1=全局默认 >=0=具体值。
func (c *Client) SetShareLimits(ctx context.Context, hashes []string, limits downloader.ShareLimits) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("ratioLimit", strconv.FormatFloat(limits.RatioLimit, 'f', -1, 64))
	form.Set("seedingTimeLimit", strconv.Itoa(limits.SeedingTimeLimit))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setShareLimits",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setShareLimits status %d", status)
	}
	return nil
}

// —— 队列优先级 ——

// SetQueuePriority 调整队列优先级（top=置顶 up=上移 down=下移 bottom=置底）。
func (c *Client) SetQueuePriority(ctx context.Context, hashes []string, action string) error {
	path, ok := map[string]string{
		"top":    "/api/v2/torrents/topPriority",
		"up":     "/api/v2/torrents/increasePrio",
		"down":   "/api/v2/torrents/decreasePrio",
		"bottom": "/api/v2/torrents/bottomPriority",
	}[action]
	if !ok {
		return fmt.Errorf("invalid queue action: %s", action)
	}
	return c.postHashes(ctx, path, hashes)
}

// —— 强制开始 ——

// SetForceStart 强制开始/取消强制。
func (c *Client) SetForceStart(ctx context.Context, hashes []string, enable bool) error {
	form := url.Values{}
	form.Set("hashes", joinHashes(hashes))
	form.Set("value", strconv.FormatBool(enable))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/setForceStart",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("setForceStart status %d", status)
	}
	return nil
}

// —— 顺序下载 / 首尾优先（toggle）——

// ToggleSequentialDownload 切换顺序下载。
func (c *Client) ToggleSequentialDownload(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/toggleSequentialDownload", hashes)
}

// ToggleFirstLastPiece 切换首尾优先。
func (c *Client) ToggleFirstLastPiece(ctx context.Context, hashes []string) error {
	return c.postHashes(ctx, "/api/v2/torrents/toggleFirstLastPiecePrio", hashes)
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体（/sync/torrentPeers）。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.do(ctx, http.MethodGet, "/api/v2/sync/torrentPeers?hash="+url.QueryEscape(hash), "", "")
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("torrentPeers status %d", status)
	}
	var resp struct {
		Peers map[string]struct {
			Client   string  `json:"client"`
			Progress float64 `json:"progress"`
			DLSpeed  int64   `json:"dl_speed"`
			UPSpeed  int64   `json:"up_speed"`
			Flags    string  `json:"flags"`
			Country  string  `json:"country"`
		} `json:"peers"`
	}
	if err := json.Unmarshal(b, &resp); err != nil {
		return nil, fmt.Errorf("parse torrentPeers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(resp.Peers))
	for ip, p := range resp.Peers {
		out = append(out, downloader.Peer{
			IP:       ip,
			Client:   p.Client,
			Progress: p.Progress,
			DLRate:   p.DLSpeed,
			UPRate:   p.UPSpeed,
			Flags:    p.Flags,
		})
	}
	return out, nil
}

// —— 分类增删 ——

// CreateCategory 新建分类（可绑定保存路径）。
func (c *Client) CreateCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/createCategory",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("createCategory status %d", status)
	}
	return nil
}

// EditCategory 编辑分类保存路径。
func (c *Client) EditCategory(ctx context.Context, name, savePath string) error {
	form := url.Values{}
	form.Set("category", name)
	form.Set("savePath", savePath)
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/editCategory",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("editCategory status %d", status)
	}
	return nil
}

// DeleteCategories 删除分类（categories 换行分隔）。
func (c *Client) DeleteCategories(ctx context.Context, names []string) error {
	form := url.Values{}
	form.Set("categories", strings.Join(names, "\n"))
	status, _, err := c.do(ctx, http.MethodPost, "/api/v2/torrents/removeCategories",
		form.Encode(), "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("removeCategories status %d", status)
	}
	return nil
}
