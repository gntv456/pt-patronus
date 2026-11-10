// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

// 云盘文件操作面（从 cloud.go 拆出以满足单文件 400 行门禁）：建目录/移动/复制/重命名/
// 删除/离线下载/分享转存 + 名称与路径钳制。逻辑零变化，纯搬运。

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fileManager 为网盘构建文件管理器客户端（不支持的网盘返回明确错误）。
func (c *CloudChain) fileManager(id uint) (cloud.FileManager, *models.CloudDrive, error) {
	cli, m, err := c.client(id)
	if err != nil {
		return nil, nil, err
	}
	fm, ok := cli.(cloud.FileManager)
	if !ok {
		return nil, m, fmt.Errorf("网盘类型 %s 不支持文件管理操作", m.Type)
	}
	return fm, m, nil
}

// sanitizeCloudName 校验并钳制云盘新建/重命名名称：去首尾空白、按路径语义钳为该级纯名，
// 防 "a/../../x" 类名称经 joinCloud+CleanPath 解析后逃出 RootPath（在网盘根逐级建目录）。
// 与本地 filemanager.sanitizeNewName 同款容错：钳制后为空 / "." / ".." 才拒绝。
func sanitizeCloudName(name string) (string, error) {
	n := path.Base(strings.ReplaceAll(strings.TrimSpace(name), `\`, "/"))
	if n == "" || n == "." || n == ".." || n == "/" {
		return "", fmt.Errorf("非法名称: %q", name)
	}
	return n, nil
}

// CloudMkdir 建目录（单层；幂等）。
func (c *CloudChain) CloudMkdir(ctx context.Context, id uint, parent, name string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	safe, err := sanitizeCloudName(name)
	if err != nil {
		return err
	}
	return fm.Mkdir(ctx, clampCloudPath(c.rootOf(id), parent), safe)
}

// CloudMove 移动。
func (c *CloudChain) CloudMove(ctx context.Context, id uint, src, dstDir string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Move(ctx, clampCloudPath(c.rootOf(id), src), clampCloudPath(c.rootOf(id), dstDir))
}

// CloudCopy 复制。
func (c *CloudChain) CloudCopy(ctx context.Context, id uint, src, dstDir string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Copy(ctx, clampCloudPath(c.rootOf(id), src), clampCloudPath(c.rootOf(id), dstDir))
}

// CloudRename 重命名。
func (c *CloudChain) CloudRename(ctx context.Context, id uint, src, newName string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	safe, err := sanitizeCloudName(newName)
	if err != nil {
		return err
	}
	return fm.Rename(ctx, clampCloudPath(c.rootOf(id), src), safe)
}

// CloudDelete 删除。
func (c *CloudChain) CloudDelete(ctx context.Context, id uint, src string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.Delete(ctx, clampCloudPath(c.rootOf(id), src))
}

// CloudOfflineAdd 离线下载（磁力/URL → 云盘目录）。
func (c *CloudChain) CloudOfflineAdd(ctx context.Context, id uint, dir, rawURL string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.OfflineAdd(ctx, clampCloudPath(c.rootOf(id), dir), rawURL)
}

// CloudSaveShare 分享链接转存。
func (c *CloudChain) CloudSaveShare(ctx context.Context, id uint, dir, shareURL, pwd string) error {
	fm, _, err := c.fileManager(id)
	if err != nil {
		return err
	}
	return fm.SaveShare(ctx, clampCloudPath(c.rootOf(id), dir), shareURL, pwd)
}

// rootOf 网盘 RootPath（钳制用；查询失败返 "/"）。
func (c *CloudChain) rootOf(id uint) string {
	if m, err := c.op.Get(id); err == nil {
		return m.RootPath
	}
	return "/"
}
