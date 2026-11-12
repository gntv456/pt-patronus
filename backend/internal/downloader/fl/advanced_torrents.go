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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// GetSpeedLimits 读取全局速度上限 → GET /api/client/settings。
// jesec ClientSettings.throttleGlobalDownSpeed / throttleGlobalUpSpeed，单位 B/s。
func (c *Client) GetSpeedLimits(ctx context.Context) (*downloader.Limits, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/client/settings", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("client settings status %d", status)
	}
	var s struct {
		Down int64 `json:"throttleGlobalDownSpeed"`
		Up   int64 `json:"throttleGlobalUpSpeed"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse client settings: %w", err)
	}
	return &downloader.Limits{DownloadLimit: s.Down, UploadLimit: s.Up}, nil
}

// SetSpeedLimits 设置全局速度上限 → PATCH /api/client/settings。
// throttleGlobalDownSpeed/UpSpeed 属 SAFE_CLIENT_SETTINGS（非 admin 字段），普通用户可改。
func (c *Client) SetSpeedLimits(ctx context.Context, limits downloader.Limits) error {
	payload := map[string]any{
		"throttleGlobalDownSpeed": limits.DownloadLimit,
		"throttleGlobalUpSpeed":   limits.UploadLimit,
	}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/client/settings",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set client settings status %d", status)
	}
	return nil
}

// —— 文件优先级 ——

// ListFiles 列出某任务内文件 → GET /api/torrents/:hash/contents。
// jesec TorrentContent：index / path / filename / percentComplete(0-100) /
// priority(0-2: 0=不下载 1=正常 2=高) / sizeBytes。
func (c *Client) ListFiles(ctx context.Context, hash string) ([]downloader.TorrentFile, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/contents", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("files status %d", status)
	}
	var files []struct {
		Index    int     `json:"index"`
		Path     string  `json:"path"`
		Filename string  `json:"filename"`
		Size     int64   `json:"sizeBytes"`
		Progress float64 `json:"percentComplete"`
		Priority int     `json:"priority"`
	}
	if err := json.Unmarshal(b, &files); err != nil {
		return nil, fmt.Errorf("parse files: %w", err)
	}
	out := make([]downloader.TorrentFile, 0, len(files))
	for _, f := range files {
		name := f.Path
		if name == "" {
			name = f.Filename
		}
		prog := f.Progress
		if prog > 1.0001 { // jesec 返回 0-100，归一化到 0-1
			prog /= 100
		}
		out = append(out, downloader.TorrentFile{
			Index: f.Index, Name: name, Size: f.Size, Progress: prog, Priority: f.Priority,
		})
	}
	return out, nil
}

// SetFilePriority 设置文件优先级 → PATCH /api/torrents/:hash/contents {indices, priority}。
// jesec priority 取值 0-2（0=不下载 1=正常 2=高），超出范围会被 zod 拒绝。
func (c *Client) SetFilePriority(ctx context.Context, hash string, indices []int, priority int) error {
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/"+strings.ToUpper(hash)+"/contents",
		mustJSON(map[string]any{"indices": indices, "priority": priority}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set file prio status %d", status)
	}
	return nil
}

// —— Peers ——

// ListPeers 列出某任务当前连接的对等体 → GET /api/torrents/:hash/peers。
// jesec TorrentPeer：address / clientVersion / completedPercent(0-100) /
// downloadRate / uploadRate / isEncrypted / isIncoming。
func (c *Client) ListPeers(ctx context.Context, hash string) ([]downloader.Peer, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/peers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("peers status %d", status)
	}
	var peers []struct {
		Address      string  `json:"address"`
		Client       string  `json:"clientVersion"`
		CompletedPct float64 `json:"completedPercent"`
		DlRate       int64   `json:"downloadRate"`
		UpRate       int64   `json:"uploadRate"`
		IsEncrypted  bool    `json:"isEncrypted"`
		IsIncoming   bool    `json:"isIncoming"`
	}
	if err := json.Unmarshal(b, &peers); err != nil {
		return nil, fmt.Errorf("parse peers: %w", err)
	}
	out := make([]downloader.Peer, 0, len(peers))
	for _, p := range peers {
		// 汇总标志位：E=加密 I=入站连接（qB flags 子集语义）
		var flags strings.Builder
		if p.IsEncrypted {
			flags.WriteByte('E')
		}
		if p.IsIncoming {
			flags.WriteByte('I')
		}
		prog := p.CompletedPct
		if prog > 1.0001 { // 0-100 → 0-1
			prog /= 100
		}
		out = append(out, downloader.Peer{
			IP:       p.Address,
			Client:   p.Client,
			Progress: prog,
			DLRate:   p.DlRate,
			UPRate:   p.UpRate,
			Flags:    flags.String(),
		})
	}
	return out, nil
}

// —— Tracker ——

// ListTrackers 列出某任务的 tracker → GET /api/torrents/:hash/trackers。
// jesec TorrentTracker：url / type(1=HTTP 2=UDP 3=DHT)。Flood 不暴露 tier，type 映射到 Status。
func (c *Client) ListTrackers(ctx context.Context, hash string) ([]downloader.Tracker, error) {
	status, b, err := c.raw(ctx, http.MethodGet, "/api/torrents/"+strings.ToUpper(hash)+"/trackers", nil, "", true)
	if err != nil {
		return nil, err
	}
	if status == 404 {
		return nil, downloader.ErrNotImplemented
	}
	if status != 200 {
		return nil, fmt.Errorf("trackers status %d", status)
	}
	var tr []struct {
		URL  string `json:"url"`
		Type int    `json:"type"`
	}
	if err := json.Unmarshal(b, &tr); err != nil {
		return nil, fmt.Errorf("parse trackers: %w", err)
	}
	out := make([]downloader.Tracker, 0, len(tr))
	for _, t := range tr {
		out = append(out, downloader.Tracker{URL: t.URL, Status: trackerTypeStatus(t.Type)})
	}
	return out, nil
}

// AddTrackers 追加 tracker。jesec 仅提供覆盖式 SET（PATCH /api/torrents/trackers），
// 无原生 add：读当前 tracker → 并入新 URL → 覆盖设置。
func (c *Client) AddTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.ListTrackers(ctx, hash)
	if err != nil {
		return err
	}
	existing := make([]string, 0, len(cur))
	for _, t := range cur {
		existing = append(existing, t.URL)
	}
	return c.setTrackers(ctx, []string{hash}, unionStrings(existing, urls))
}

// RemoveTrackers 移除 tracker。同样经 SET：读当前 → 减去要删的 → 覆盖设置。
func (c *Client) RemoveTrackers(ctx context.Context, hash string, urls []string) error {
	cur, err := c.ListTrackers(ctx, hash)
	if err != nil {
		return err
	}
	existing := make([]string, 0, len(cur))
	for _, t := range cur {
		existing = append(existing, t.URL)
	}
	return c.setTrackers(ctx, []string{hash}, subtractStrings(existing, urls))
}

// setTrackers jesec PATCH /api/torrents/trackers {hashes, trackers}（覆盖式 SET）。
// schema：hashes 非空数组、trackers 可为空（清空所有 tracker）。
func (c *Client) setTrackers(ctx context.Context, hashes []string, trackers []string) error {
	payload := map[string]any{"hashes": upperAll(hashes), "trackers": trackers}
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/trackers",
		mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set trackers status %d", status)
	}
	return nil
}

// —— 内部工具 ——

// postHashesJesec 公用：POST {hashes} 到 jesec 端点，404→ErrNotImplemented。
func (c *Client) postHashesJesec(ctx context.Context, path string, hashes []string) error {
	status, _, err := c.raw(ctx, http.MethodPost, path,
		mustJSON(map[string]any{"hashes": upperAll(hashes)}), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("%s status %d", path, status)
	}
	return nil
}

// tagsByHashes 一次 ListTorrents，构建 hash(小写)→tags 映射，供 AddTags/RemoveTags diff。
func (c *Client) tagsByHashes(ctx context.Context, hashes []string) (map[string][]string, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(list))
	for _, t := range list {
		out[t.Hash] = t.Tags
	}
	return out, nil
}
