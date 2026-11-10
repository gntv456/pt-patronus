// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package fl

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// torrentProps Flood 单种子属性（jesec/legacy 共用字段子集）。
// 字段对齐 jesec/flood 的 TorrentProperties 类型（shared/types/Torrent.ts）。
type torrentProps struct {
	Hash             string   `json:"hash"`
	Name             string   `json:"name"`
	BytesDone        int64    `json:"bytesDone"`
	SizeBytes        int64    `json:"sizeBytes"`
	PercentComplete  float64  `json:"percentComplete"`
	Ratio            float64  `json:"ratio"`
	DownRate         int64    `json:"downRate"`
	UpRate           int64    `json:"upRate"`
	DownTotal        int64    `json:"downTotal"`
	UpTotal          int64    `json:"upTotal"`
	Directory        string   `json:"directory"`
	DateAdded        float64  `json:"dateAdded"`
	DateFinished     float64  `json:"dateFinished"`
	IsComplete       bool     `json:"isComplete"`
	IsInitialSeeding bool     `json:"isInitialSeeding"`
	Status           []string `json:"status"`
	Tags             []string `json:"tags"`
	SeedsConnected   int      `json:"seedsConnected"`
	PeersConnected   int      `json:"peersConnected"`
	SeedsTotal       int      `json:"seedsTotal"`
	PeersTotal       int      `json:"peersTotal"`
	Priority         int      `json:"priority"`
	IsSequential     bool     `json:"isSequential"`
}

// ListTorrents 列任务。
func (c *Client) ListTorrents(ctx context.Context) ([]downloader.Torrent, error) {
	a, err := c.detectAPI(ctx)
	if err != nil {
		return nil, err
	}
	var m map[string]torrentProps
	if a == apiLegacy {
		m, err = c.legacyTorrentList(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		status, b, err := c.requestJSON(ctx, http.MethodGet, "getTorrents", nil)
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("getTorrents status %d", status)
		}
		var wrap struct {
			Torrents map[string]torrentProps `json:"torrents"`
		}
		if err := json.Unmarshal(b, &wrap); err != nil {
			return nil, fmt.Errorf("parse torrents: %w", err)
		}
		m = wrap.Torrents
	}
	out := make([]downloader.Torrent, 0, len(m))
	for h, p := range m {
		if p.Hash == "" {
			p.Hash = h
		}
		out = append(out, toTorrent(p))
	}
	return out, nil
}

// AddTorrent 添加。
func (c *Client) AddTorrent(ctx context.Context, opts downloader.AddOptions) error {
	post := map[string]any{
		"destination": opts.SavePath,
		"tags":        []string{},
		"start":       !opts.Paused,
	}
	// 二轮 P2-3：Category 与 Tags 合并下发（此前只放 Category，ptpsrc 丢失 →
	// 整理 Path A 秒识别降级为文件名搜索）。Flood 的 tags 是扁平列表，无冲突。
	tags := append([]string{}, opts.Tags...)
	if opts.Category != "" {
		tags = append(tags, opts.Category)
	}
	if len(tags) > 0 {
		post["tags"] = tags
	}
	if len(opts.FileContent) > 0 && opts.URL == "" {
		a, err := c.detectAPI(ctx)
		if err != nil {
			return err
		}
		if a == apiJesec {
			post["files"] = []string{base64.StdEncoding.EncodeToString(opts.FileContent)}
			status, _, err := c.requestJSON(ctx, http.MethodPost, "addTorrentByFile", post)
			if err != nil {
				return err
			}
			if status != 200 && status != 202 {
				return fmt.Errorf("add-files status %d", status)
			}
			return nil
		}
		fields := map[string]string{
			"destination": opts.SavePath,
			"start":       fmt.Sprintf("%t", !opts.Paused),
		}
		if opts.Category != "" {
			fields["tags"] = opts.Category
		}
		status, _, err := c.requestMultipart(ctx, "addTorrentByFile", fields, opts.FileName, opts.FileContent)
		if err != nil {
			return err
		}
		if status != 200 && status != 202 {
			return fmt.Errorf("add-files status %d", status)
		}
		return nil
	}
	post["urls"] = []string{opts.URL}
	status, _, err := c.requestJSON(ctx, http.MethodPost, "addTorrentByUrl", post)
	if err != nil {
		return err
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("add-urls status %d", status)
	}
	return nil
}

// Pause → stopTorrent。
func (c *Client) Pause(ctx context.Context, hashes []string) error {
	status, _, err := c.requestJSON(ctx, http.MethodPost, "stopTorrent", map[string]any{"hashes": upperAll(hashes)})
	if err != nil {
		return err
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("stop status %d", status)
	}
	return nil
}

// Resume → startTorrent。
func (c *Client) Resume(ctx context.Context, hashes []string) error {
	status, _, err := c.requestJSON(ctx, http.MethodPost, "startTorrent", map[string]any{"hashes": upperAll(hashes)})
	if err != nil {
		return err
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("start status %d", status)
	}
	return nil
}

// Delete。
func (c *Client) Delete(ctx context.Context, hashes []string, deleteFiles bool) error {
	a, err := c.detectAPI(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{"deleteData": deleteFiles}
	if a == apiJesec {
		payload["hashes"] = upperAll(hashes)
	} else {
		payload["hash"] = upperAll(hashes)
	}
	status, _, err := c.requestJSON(ctx, http.MethodPost, "deleteTorrent", payload)
	if err != nil {
		return err
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("delete status %d", status)
	}
	return nil
}

// SetCategory → tags（首个 tag 当 label）。
func (c *Client) SetCategory(ctx context.Context, hashes []string, category string) error {
	// jesec: PATCH /api/torrents/tags 或 set-taxonomy
	// 简化：POST /api/torrents/tags
	payload := map[string]any{
		"hashes": upperAll(hashes),
		"tags":   []string{category},
	}
	// 尝试 jesec 路径
	status, _, err := c.raw(ctx, http.MethodPatch, "/api/torrents/tags", mustJSON(payload), "application/json", true)
	if err != nil {
		return err
	}
	if status == 404 {
		// legacy 可能无此接口
		return downloader.ErrNotImplemented
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("set tags status %d", status)
	}
	return nil
}

// GlobalStats 汇总。
func (c *Client) GlobalStats(ctx context.Context) (*downloader.GlobalStats, error) {
	list, err := c.ListTorrents(ctx)
	if err != nil {
		return nil, err
	}
	s := &downloader.GlobalStats{}
	for _, t := range list {
		s.DLRate += t.DLRate
		s.UPRate += t.UPRate
		s.TotalSize += t.Size
	}
	return s, nil
}

func upperAll(hs []string) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = strings.ToUpper(h)
	}
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
