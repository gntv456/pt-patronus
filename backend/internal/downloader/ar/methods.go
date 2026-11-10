// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ar

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// ListTorrents 合并 tellActive + tellWaiting + tellStopped，仅保留 bittorrent 任务。
// system.multicall 成功项为 [result]，result 本身是 task 数组 → 结构 [[task,...]]。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	raw, err := c.multiCall(ctx, []map[string]any{
		{"methodName": "aria2.tellActive", "params": []any{}},
		{"methodName": "aria2.tellWaiting", "params": []any{0, 1000}},
		{"methodName": "aria2.tellStopped", "params": []any{0, 1000}},
	})
	if err != nil {
		return nil, err
	}
	var batches []json.RawMessage
	if err := json.Unmarshal(raw, &batches); err != nil {
		return nil, fmt.Errorf("parse multicall: %w", err)
	}
	var out []downloader.Torrent
	for _, batch := range batches {
		// 成功：[result]；result = []rawTask
		var wrap []json.RawMessage
		if err := json.Unmarshal(batch, &wrap); err != nil || len(wrap) == 0 {
			continue
		}
		var tasks []rawTask
		if err := json.Unmarshal(wrap[0], &tasks); err != nil {
			// 兼容直接 []rawTask 的 mock
			if err2 := json.Unmarshal(batch, &tasks); err2 != nil {
				continue
			}
		}
		for _, t := range tasks {
			if t.Bittorrent == nil {
				continue
			}
			out = append(out, toTorrent(t))
		}
	}
	return out, nil
}

// AddTorrent 添加种子（magnet/链接 或 .torrent 字节）。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	addOpt := map[string]any{}
	if opts.Paused {
		addOpt["pause"] = "true"
	}
	if opts.SavePath != "" {
		addOpt["dir"] = opts.SavePath
	}
	if opts.UploadLimit > 0 {
		addOpt["max-upload-limit"] = strconv.FormatInt(opts.UploadLimit, 10)
	}
	if opts.DownloadLimit > 0 {
		addOpt["max-download-limit"] = strconv.FormatInt(opts.DownloadLimit, 10)
	}

	var method string
	var params []any
	if len(opts.FileContent) > 0 && opts.URL == "" {
		method = "aria2.addTorrent"
		params = []any{base64.StdEncoding.EncodeToString(opts.FileContent), []any{}, addOpt}
	} else {
		method = "aria2.addUri"
		params = []any{[]string{opts.URL}, addOpt}
	}
	_, err := c.call(ctx, method, params)
	return err
}

// Pause 暂停（aria2.pause）。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	return c.forEachGID(ctx, "aria2.pause", hashes)
}

// Resume 恢复（aria2.unpause）。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	return c.forEachGID(ctx, "aria2.unpause", hashes)
}

// Delete 删除任务（remove + removeDownloadResult）。
// Aria2 无原生 deleteFiles；deleteFiles 走 forceRemove 尽力删除。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	method := "aria2.remove"
	if deleteFiles {
		method = "aria2.forceRemove"
	}
	for _, h := range hashes {
		// removeDownloadResult 仅对已 stopped/removed 的任务有效；先 remove 再清理结果。
		// remove 失败立即返回；清理结果失败不阻断（尽力删除本地记录，Aria2 不影响实际文件）。
		if _, err := c.call(ctx, method, []any{h}); err != nil {
			return err
		}
		if _, err := c.call(ctx, "aria2.removeDownloadResult", []any{h}); err != nil {
			return fmt.Errorf("remove download result %s: %w", h, err)
		}
	}
	return nil
}

// SetCategory：Aria2 无 label/category。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	return downloader.ErrNotImplemented
}

// GlobalStats → aria2.getGlobalStat。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	raw, err := c.call(ctx, "aria2.getGlobalStat", nil)
	if err != nil {
		return nil, err
	}
	var s struct {
		DownloadSpeed string `json:"downloadSpeed"`
		UploadSpeed   string `json:"uploadSpeed"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse globalStat: %w", err)
	}
	return &downloader.GlobalStats{
		DLRate: parseI64(s.DownloadSpeed),
		UPRate: parseI64(s.UploadSpeed),
	}, nil
}

// forEachGID 对每个 GID 调 method(gid)。
func (c *Client) forEachGID(ctx context.Context, method string, gids []string) error {
	for _, g := range gids {
		if _, err := c.call(ctx, method, []any{g}); err != nil {
			return err
		}
	}
	return nil
}
