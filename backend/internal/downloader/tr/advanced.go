// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package tr

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// bytesToKB 把 bytes/s 转 KiB/s（向上取整，避免 <1024 截断为 0）。0 或负数返回 0（=不限）。
func bytesToKB(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + 1023) / 1024
}

// extractAddedID 从 torrent-add 响应取新增任务 id（torrent-added 或 torrent-duplicate）。
func extractAddedID(raw json.RawMessage) int {
	var r struct {
		Added *struct {
			ID int `json:"id"`
		} `json:"torrent-added"`
		Duplicate *struct {
			ID int `json:"id"`
		} `json:"torrent-duplicate"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0
	}
	if r.Added != nil {
		return r.Added.ID
	}
	if r.Duplicate != nil {
		return r.Duplicate.ID
	}
	return 0
}

// —— 生命周期 ——

// Recheck 强制重新校验。
func (c *Client) Recheck(ctx context.Context, hashes []string) error {
	_, err := c.rpc(ctx, "torrent-verify", map[string]any{"ids": hashes})
	return err
}

// Reannounce 重新汇报。
func (c *Client) Reannounce(ctx context.Context, hashes []string) error {
	_, err := c.rpc(ctx, "torrent-reannounce", map[string]any{"ids": hashes})
	return err
}

// SetLocation 移动保存路径（TR 3.0+；旧版会 RPC 报错）。
func (c *Client) SetLocation(ctx context.Context, hashes []string, location string) error {
	_, err := c.rpc(ctx, "torrent-set-location", map[string]any{
		"ids":      hashes,
		"location": location,
		"move":     true,
	})
	return err
}

// —— 分类 / 标签 ——

// ListCategories：Transmission 无分类概念。
func (c *Client) ListCategories(ctx context.Context) ([]string, error) {
	return nil, downloader.ErrNotImplemented
}

// ListTags 聚合全部任务的 labels 去重。
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{"fields": []string{"labels"}})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Labels []string `json:"labels"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}
	seen := map[string]bool{}
	var tags []string
	for _, t := range out.Torrents {
		for _, l := range t.Labels {
			if !seen[l] {
				seen[l] = true
				tags = append(tags, l)
			}
		}
	}
	return tags, nil
}

// setLabels 覆盖式设置某组任务的 labels。
func (c *Client) setLabels(ctx context.Context, hashes []string, labels []string) error {
	_, err := c.rpc(ctx, "torrent-set", map[string]any{"ids": hashes, "labels": labels})
	return err
}

// labelsOf 取单个任务的当前 labels。
func (c *Client) labelsOf(ctx context.Context, hash string) ([]string, error) {
	raw, err := c.rpc(ctx, "torrent-get", map[string]any{
		"ids":    []string{hash},
		"fields": []string{"labels"},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []struct {
			Labels []string `json:"labels"`
		} `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Torrents) == 0 {
		return nil, nil
	}
	return out.Torrents[0].Labels, nil
}

// SetTags 覆盖式设置标签（TR labels 天然覆盖语义）。
func (c *Client) SetTags(ctx context.Context, hashes []string, tags []string) error {
	return c.setLabels(ctx, hashes, tags)
}

// AddTags 追加标签（TR labels 是 per-torrent，逐个合并去重）。
func (c *Client) AddTags(ctx context.Context, hashes []string, tags []string) error {
	for _, h := range hashes {
		old, err := c.labelsOf(ctx, h)
		if err != nil {
			return err
		}
		if err := c.setLabels(ctx, []string{h}, mergeUnique(old, tags)); err != nil {
			return err
		}
	}
	return nil
}

// RemoveTags 移除标签（逐个过滤）。
func (c *Client) RemoveTags(ctx context.Context, hashes []string, tags []string) error {
	rm := make(map[string]bool, len(tags))
	for _, t := range tags {
		rm[t] = true
	}
	for _, h := range hashes {
		old, err := c.labelsOf(ctx, h)
		if err != nil {
			return err
		}
		var kept []string
		for _, l := range old {
			if !rm[l] {
				kept = append(kept, l)
			}
		}
		if err := c.setLabels(ctx, []string{h}, kept); err != nil {
			return err
		}
	}
	return nil
}

func mergeUnique(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
