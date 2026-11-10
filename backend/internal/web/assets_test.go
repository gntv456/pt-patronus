// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAssetServer 造一个本地 HTTP 服务冒充上游，返回 manifest 里的真实内容。
// 不打真实外网：CI 无外网、且测试不该依赖 GitHub 可用性。
func writeFakeAsset(t *testing.T, dir, name string) []byte {
	t.Helper()
	body := []byte("fake-" + name)
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
	return body
}

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TestAssetStoreManifest 清单自洽：三个文件、sha256 格式、字节数。
func TestAssetStoreManifest(t *testing.T) {
	if len(Assets) != 3 {
		t.Fatalf("清单应含 3 个文件，实际 %d", len(Assets))
	}
	seen := map[string]bool{}
	for _, a := range Assets {
		if seen[a.File] {
			t.Errorf("清单重复：%s", a.File)
		}
		seen[a.File] = true
		if len(a.SHA256) != 64 {
			t.Errorf("%s sha256 长度 %d，应为 64", a.File, len(a.SHA256))
		}
		if a.Bytes <= 0 {
			t.Errorf("%s 字节数非正", a.File)
		}
	}
	// decoder.wasm 是三件套里最大的那个，也是最可能漏的
	if !seen["decoder.wasm"] || !seen["decoder.js"] || !seen["jessibuca.js"] {
		t.Errorf("三件套不全：%v", seen)
	}
}

// TestAssetStoreValidOnDisk 磁盘校验：存在且摘要匹配才算就位。
func TestAssetStoreValidOnDisk(t *testing.T) {
	dir := t.TempDir()
	store := NewAssetStore(dir)
	a := Assets[0]
	p, _, err := store.pathFor(a.File)
	if err != nil {
		t.Fatal(err)
	}

	if store.validOnDisk(p, a) {
		t.Fatal("文件不存在时不应视为有效")
	}
	if err := os.MkdirAll(filepath.Join(dir, "jessibuca"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := writeFakeAsset(t, filepath.Join(dir, "jessibuca"), a.File)
	a.Bytes = int64(len(body))
	a.SHA256 = sha(body)
	if !store.validOnDisk(p, a) {
		t.Error("摘要匹配时应视为有效")
	}
	// 内容被改（大小不变）也必须判无效——这是摘要而非仅大小的意义
	body[0] = 'X'
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if store.validOnDisk(p, a) {
		t.Error("内容被改后仍视为有效（只比大小会漏）")
	}
}

// TestAssetStorePathTraversal 清单外的名字一律拒绝——防路径穿越把任意文件读出去。
func TestAssetStorePathTraversal(t *testing.T) {
	store := NewAssetStore(t.TempDir())
	for _, name := range []string{"", "../secret", "..\\secret", "a/b/c", "/etc/passwd", "jessibuca.js/../x"} {
		if _, _, err := store.pathFor(name); err == nil {
			t.Errorf("应拒绝非法名 %q", name)
		}
	}
	if _, _, err := store.pathFor("jessibuca.js"); err != nil {
		t.Errorf("合法名被拒：%v", err)
	}
}

// TestSafeName HTTP 路径清洗：去掉目录部分，只留文件名。
func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"/jessibuca/decoder.wasm": "decoder.wasm",
		"decoder.js":              "decoder.js",
		"/a/b/../../decoder.wasm": "decoder.wasm",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q)=%q，应为 %q", in, got, want)
		}
	}
}

// TestAssetHandlerMethod 只允许 GET/HEAD。
func TestAssetHandlerMethod(t *testing.T) {
	store := NewAssetStore(t.TempDir())
	h := AssetHandler(store)
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/jessibuca/jessibuca.js", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s 应 405，实际 %d", m, rec.Code)
		}
	}
}

// TestAssetHandlerNotFoundInManifest 清单外的路径返回 404，
// 关键是不能回落成 200 + index.html——浏览器会把 HTML 当 wasm 报解码错误。
func TestAssetHandlerNotFoundInManifest(t *testing.T) {
	store := NewAssetStore(t.TempDir())
	h := AssetHandler(store)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/jessibuca/not-in-manifest.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("清单外文件应 404，实际 %d body=%q", rec.Code, rec.Body.String())
	}
}

// TestAssetHandlerServesFromDisk 已就位时正确直出，且 MIME 正确
// （wasm 必须 application/wasm，否则浏览器不走 instantiateStreaming）。
func TestAssetHandlerServesFromDisk(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(dataDir, "jessibuca")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := writeFakeAsset(t, dir, "jessibuca.js")

	h := AssetHandler(NewAssetStore(dataDir))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/jessibuca/jessibuca.js", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("应 200，实际 %d", rec.Code)
	}
	if got := rec.Body.String(); got != string(body) {
		t.Errorf("内容不符：%q", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/javascript") {
		t.Errorf("js MIME 不对：%q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("不可变资源应长缓存，实际 %q", cc)
	}
}

// TestContentType wasm 走 application/wasm。
func TestContentType(t *testing.T) {
	if ct := contentType("decoder.wasm"); ct != "application/wasm" {
		t.Errorf("wasm MIME=%q，应为 application/wasm", ct)
	}
	if ct := contentType("decoder.js"); !strings.Contains(ct, "javascript") {
		t.Errorf("js MIME=%q", ct)
	}
}

// TestAssetStoreReady 空目录应为未就位。
func TestAssetStoreReady(t *testing.T) {
	store := NewAssetStore(t.TempDir())
	if store.Ready() {
		t.Error("空目录不应视为已就位")
	}
	if len(store.Status()) != len(Assets) {
		t.Errorf("Status 条目数 %d，应为 %d", len(store.Status()), len(Assets))
	}
}
