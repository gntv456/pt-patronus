// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ErrCloudReadOnly 网盘后端为只读（STRM 浏览/下载场景）；写操作返回此错误。
var ErrCloudReadOnly = fmt.Errorf("网盘后端为只读（仅支持浏览/下载，不支持上传/删除/建目录）")

// cloudBackend 把网盘（115/夸克/123）client 适配为 filemanager 的 StorageBackend（只读）。
// 复用 cloud.Client.List；Upload/Delete/Mkdir 返回 ErrCloudReadOnly。cookie/token 由构造方解密传入。
// 同 Alist 范式：basePath 把浏览约束在该子目录下（防 ../ 越界）。
type cloudBackend struct {
	cli      cloud.Client
	typ      string
	basePath string
}

// NewCloudBackend 按 type 构造网盘后端。token 为明文 cookie/accessToken；basePath 为浏览根。
func NewCloudBackend(typ, token, basePath string) (*cloudBackend, error) {
	hc, err := httpclient.New(httpclient.Options{Timeout: 30 * time.Second, MaxRetries: 2})
	if err != nil {
		return nil, err
	}
	cli, err := cloud.NewClient(cloud.Drive{Type: typ, Token: token, RootPath: basePath}, hc)
	if err != nil {
		return nil, err
	}
	return &cloudBackend{cli: cli, typ: typ, basePath: basePath}, nil
}

func (b *cloudBackend) Type() string { return b.typ }

func (b *cloudBackend) List(ctx context.Context, p string) ([]Entry, error) {
	p = resolvePath(b.basePath, p)
	files, err := b.cli.List(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(files))
	for _, f := range files {
		out = append(out, Entry{
			Name: f.Name, Path: f.Path, IsDir: f.IsDir, Size: f.Size, ModTime: f.ModTime,
		})
	}
	return out, nil
}

// Download STRM 场景仅需浏览；下载需网盘直链解析（115/夸克 cookie 限时），暂不支持，返回只读错误。
// TODO 联调：若需文件管理器直接下载网盘文件，在此解析直链并 GET 返回 ReadCloser。
func (b *cloudBackend) Download(ctx context.Context, p string) (io.ReadCloser, error) {
	return nil, ErrCloudReadOnly
}

func (b *cloudBackend) Upload(ctx context.Context, p string, r io.Reader) error {
	return ErrCloudReadOnly
}

func (b *cloudBackend) Delete(ctx context.Context, p string) error {
	return ErrCloudReadOnly
}

func (b *cloudBackend) Mkdir(ctx context.Context, p string) error {
	return ErrCloudReadOnly
}

// 网盘只读：Rename/Move/Copy 均不支持（同 Upload/Delete/Mkdir）。
func (b *cloudBackend) Rename(ctx context.Context, p, newName string) error { return ErrCloudReadOnly }
func (b *cloudBackend) Move(ctx context.Context, src, dstDir string) error  { return ErrCloudReadOnly }
func (b *cloudBackend) Copy(ctx context.Context, src, dstDir string) error  { return ErrCloudReadOnly }
