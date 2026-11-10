// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
)

// generic_extra.go — 网络后端(WebDAV/SMB/Alist/Rclone)的通用扩展能力。
// 这些后端已有 Download/Upload/List，据此桥接出 ReadFile/WriteFile/DirSize，
// 无需各自重复实现。压缩/解压/chmod 受协议限制不做（返回 ErrUnsupported 由 chain 层处理）。
//
// 复用约束：调用方传入的 backend 必须实现 StorageBackend（含 Download/Upload/List）。

const genericMaxReadSize = 10 * 1024 * 1024

// genericReadFile 通过 Download 全读到内存（限 10MB），供网络后端复用。
func genericReadFile(ctx context.Context, b StorageBackend, p string) ([]byte, error) {
	rc, err := b.Download(ctx, p)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, genericMaxReadSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > genericMaxReadSize {
		return nil, fmt.Errorf("文件过大，超过 %d 限制", genericMaxReadSize)
	}
	return data, nil
}

// genericWriteFile 通过 Upload 写入，供网络后端复用。
func genericWriteFile(ctx context.Context, b StorageBackend, p string, data []byte) error {
	return b.Upload(ctx, p, bytes.NewReader(data))
}

// genericDirSize 通过 List 递归累加文件大小，供网络后端复用。
// 深度限制 32 层防恶意环/超深目录。
func genericDirSize(ctx context.Context, b StorageBackend, p string) (int64, error) {
	return genericDirSizeRec(ctx, b, p, 0)
}

func genericDirSizeRec(ctx context.Context, b StorageBackend, p string, depth int) (int64, error) {
	if depth > 32 {
		return 0, fmt.Errorf("目录层级过深（>32）")
	}
	entries, err := b.List(ctx, p)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range entries {
		if e.IsDir {
			sub, err := genericDirSizeRec(ctx, b, e.Path, depth+1)
			if err != nil {
				return 0, err
			}
			total += sub
		} else {
			total += e.Size
		}
	}
	return total, nil
}
