// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strings"

	"github.com/hxpt/ptpatronus/internal/downloader"
)

// 本文件是下载器进阶操作的编排薄包装（取客户端 → 转发），与 Pause/Resume 同模式。

// —— 分类 / 标签 ——

func (c *DownloadChain) SetCategory(ctx context.Context, id uint, hashes []string, category string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetCategory(ctx, hs, category)
}

func (c *DownloadChain) ListCategories(ctx context.Context, id uint) ([]string, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListCategories(ctx)
}

func (c *DownloadChain) ListTags(ctx context.Context, id uint) ([]string, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListTags(ctx)
}

func (c *DownloadChain) SetTags(ctx context.Context, id uint, hashes []string, tags []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	if err := requireValues("标签", tags); err != nil {
		return err
	}
	return d.SetTags(ctx, hs, tags)
}

func (c *DownloadChain) AddTags(ctx context.Context, id uint, hashes []string, tags []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	if err := requireValues("标签", tags); err != nil {
		return err
	}
	return d.AddTags(ctx, hs, tags)
}

func (c *DownloadChain) RemoveTags(ctx context.Context, id uint, hashes []string, tags []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	if err := requireValues("标签", tags); err != nil {
		return err
	}
	return d.RemoveTags(ctx, hs, tags)
}

// —— 生命周期 ——

func (c *DownloadChain) Recheck(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.Recheck(ctx, hs)
}

func (c *DownloadChain) Reannounce(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.Reannounce(ctx, hs)
}

func (c *DownloadChain) SetLocation(ctx context.Context, id uint, hashes []string, location string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetLocation(ctx, hs, location)
}

// —— 限速 ——

func (c *DownloadChain) SetLimits(ctx context.Context, id uint, hashes []string, limits downloader.Limits) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetLimits(ctx, hs, limits)
}

func (c *DownloadChain) GetSpeedLimits(ctx context.Context, id uint) (*downloader.Limits, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.GetSpeedLimits(ctx)
}

func (c *DownloadChain) SetSpeedLimits(ctx context.Context, id uint, limits downloader.Limits) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.SetSpeedLimits(ctx, limits)
}

// —— 文件优先级 ——

func (c *DownloadChain) ListFiles(ctx context.Context, id uint, hash string) ([]downloader.TorrentFile, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListFiles(ctx, hash)
}

func (c *DownloadChain) SetFilePriority(ctx context.Context, id uint, hash string, indices []int, priority int) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.SetFilePriority(ctx, hash, indices, priority)
}

// —— Tracker ——

func (c *DownloadChain) ListTrackers(ctx context.Context, id uint, hash string) ([]downloader.Tracker, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListTrackers(ctx, hash)
}

func (c *DownloadChain) AddTrackers(ctx context.Context, id uint, hash string, urls []string) error {
	if strings.TrimSpace(hash) == "" {
		return ErrEmptyHashes
	}
	if err := requireValues("Tracker 地址", urls); err != nil {
		return err
	}
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.AddTrackers(ctx, hash, urls)
}

func (c *DownloadChain) RemoveTrackers(ctx context.Context, id uint, hash string, urls []string) error {
	if strings.TrimSpace(hash) == "" {
		return ErrEmptyHashes
	}
	if err := requireValues("Tracker 地址", urls); err != nil {
		return err
	}
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.RemoveTrackers(ctx, hash, urls)
}

// —— 做种限制 / 队列 / 强制 / 顺序 / Peers / 分类增删（P2）——

func (c *DownloadChain) SetShareLimits(ctx context.Context, id uint, hashes []string, limits downloader.ShareLimits) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetShareLimits(ctx, hs, limits)
}

func (c *DownloadChain) SetQueuePriority(ctx context.Context, id uint, hashes []string, action string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetQueuePriority(ctx, hs, action)
}

func (c *DownloadChain) SetForceStart(ctx context.Context, id uint, hashes []string, enable bool) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.SetForceStart(ctx, hs, enable)
}

func (c *DownloadChain) ToggleSequentialDownload(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.ToggleSequentialDownload(ctx, hs)
}

func (c *DownloadChain) ToggleFirstLastPiece(ctx context.Context, id uint, hashes []string) error {
	d, hs, err := c.clientForHashes(ctx, id, hashes)
	if err != nil {
		return err
	}
	return d.ToggleFirstLastPiece(ctx, hs)
}

func (c *DownloadChain) ListPeers(ctx context.Context, id uint, hash string) ([]downloader.Peer, error) {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return d.ListPeers(ctx, hash)
}

func (c *DownloadChain) CreateCategory(ctx context.Context, id uint, name, savePath string) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.CreateCategory(ctx, name, savePath)
}

func (c *DownloadChain) EditCategory(ctx context.Context, id uint, name, savePath string) error {
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.EditCategory(ctx, name, savePath)
}

func (c *DownloadChain) DeleteCategories(ctx context.Context, id uint, names []string) error {
	// 空列表在 qB 侧只是 categories=""（删不掉任何东西、返回 200），不是「全部删除」——
	// 但上层照样报「已删除分类」，仍是空操作报成功。
	if err := requireValues("分类", names); err != nil {
		return err
	}
	d, err := c.mgr.Get(ctx, id)
	if err != nil {
		return err
	}
	return d.DeleteCategories(ctx, names)
}
