// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package sitedefs

// 远端定义源同步的全链路单测：内存 HTTP 替身当「GitHub tarball 源」，
// 验证 拉取→解包→落地→热加载 与 min_app_version 门控、失败保缓存语义。

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// buildTarball 把 id→yaml 表打成 GitHub 风格 tar.gz（顶层 <repo>/ 前缀）。
func buildTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: "ptpatronus-sites-main/" + name, Mode: 0o644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const defDemo = `id: demo-remote
name: 远端演示站
schema: nexusphp
urls:
  - https://demo.example.com
`

const defTooNew = `id: too-new
name: 需要新版本的站
schema: nexusphp
min_app_version: "99.99.99"
urls:
  - https://new.example.com
`

// newTestEnv 起 tarball 替身源 + 带两个 nexusphp 适配器的 registry + 临时数据目录。
func newTestEnv(t *testing.T, tarball []byte, status int) (*Syncer, *site.Registry, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true, Timeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	reg := site.NewRegistry()
	// 测试里没有真 nexusphp 适配器依赖，注册占位（校验只查 schema 已注册）。
	reg.RegisterAdapter(fakeSchemaAdapter{})
	dataDir := t.TempDir()
	url := srv.URL
	s := NewSyncer(hc, reg, dataDir, "1.03.27", nil, func() string { return url })
	return s, reg, dataDir
}

type fakeSchemaAdapter struct{}

func (fakeSchemaAdapter) Name() string   { return "fake" }
func (fakeSchemaAdapter) Priority() int  { return 1 }
func (fakeSchemaAdapter) Schema() string { return "nexusphp" }

func (fakeSchemaAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (fakeSchemaAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return nil, site.ErrNotSupported
}

func TestSync_FetchLandReload(t *testing.T) {
	s, reg, dataDir := newTestEnv(t, buildTarball(t, map[string]string{
		"demo-remote.yaml": defDemo,
		"too-new.yaml":     defTooNew, // min_app_version 不满足 → 门控跳过
	}), 0)

	n, err := s.Sync(context.Background())
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n != 1 {
		t.Fatalf("synced defs = %d, want 1（too-new 应被版本门控跳过）", n)
	}
	// 落地校验
	if _, err := os.Stat(filepath.Join(dataDir, "site-definitions", "demo-remote.yaml")); err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
	// 门控只发生在加载期：缓存保留远端全集（含 too-new），升级程序后无需重拉
	// 即可在下次启动/热加载直接生效。
	if _, err := os.Stat(filepath.Join(dataDir, "site-definitions", "too-new.yaml")); err != nil {
		t.Fatalf("远端全集应完整落缓存: %v", err)
	}
	// 热加载校验
	if _, ok := reg.Definition("demo-remote"); !ok {
		t.Fatal("demo-remote 未热加载进 registry")
	}
	if _, ok := reg.Definition("too-new"); ok {
		t.Fatal("too-new 不应进 registry（min_app_version 门控）")
	}
}

func TestSync_KeepCacheOnFailure(t *testing.T) {
	s, reg, _ := newTestEnv(t, buildTarball(t, map[string]string{
		"demo-remote.yaml": defDemo,
	}), 0)
	if _, err := s.Sync(context.Background()); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// 第二次：源 500 → 报错且旧缓存/旧注册表原样保留
	s2, reg2 := s, reg
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv2.Close()
	url2 := srv2.URL
	s3 := NewSyncer(s2.hc, reg2, s2.dataDir, "1.03.27", nil, func() string { return url2 })
	if _, err := s3.Sync(context.Background()); err == nil {
		t.Fatal("源 500 应报错")
	}
	if _, ok := reg2.Definition("demo-remote"); !ok {
		t.Fatal("失败后旧注册表应保留")
	}
}

func TestSync_EmptyRemoteKeepsOld(t *testing.T) {
	s, reg, _ := newTestEnv(t, buildTarball(t, map[string]string{
		"readme.md": "# not a definition",
	}), 0)
	if _, err := s.Sync(context.Background()); err == nil {
		t.Fatal("远端空集应报错（防误清空）")
	}
	if _, ok := reg.Definition("demo-remote"); ok {
		t.Fatal("空表未热加载（无残留）")
	}
}

func TestSync_Non200(t *testing.T) {
	s, _, _ := newTestEnv(t, []byte("x"), http.StatusNotFound)
	if _, err := s.Sync(context.Background()); err == nil {
		t.Fatal("404 应报错")
	}
}
