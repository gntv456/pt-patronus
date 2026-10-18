package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureTriggersDownload 未就位时 Ensure 应触发下载并落盘。
// 用 httptest 冒充上游，验证「首次访问 → 自动下载 → 校验 → 原子落盘」整条链路，
// 不依赖真实外网。
func TestEnsureTriggersDownload(t *testing.T) {
	// 造上游：内容为 test-<name>，sha 现场算
	body := []byte("test-jessibuca.js")
	want := sha256.Sum256(body)
	wantHex := hex.EncodeToString(want[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	// 临时替换上游地址
	oldBase := assetBaseURL
	assetBaseURL = srv.URL
	defer func() { assetBaseURL = oldBase }()

	dir := t.TempDir()
	store := NewAssetStore(dir)

	// 覆盖清单为本例的期望值
	oldAssets := Assets
	Assets = []Asset{{"jessibuca.js", wantHex, int64(len(body))}}
	defer func() { Assets = oldAssets }()

	if err := store.Ensure("jessibuca.js"); err != nil {
		t.Fatalf("Ensure 应成功：%v", err)
	}
	p := filepath.Join(dir, "jessibuca", "jessibuca.js")
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Errorf("落盘内容不符：%q", got)
	}
	if !store.Ready() {
		t.Error("下载后应就位（但另两个文件缺失，Ready 应为 false）")
	}
	// 单文件就位
	if store.Status()[0].Ready != true {
		t.Error("Status 首项应为 ready")
	}
}

// TestEnsureCorruptRedownload 磁盘文件损坏时应重新下载覆盖。
func TestEnsureCorruptRedownload(t *testing.T) {
	body := []byte("good-bytes")
	want := sha256.Sum256(body)
	wantHex := hex.EncodeToString(want[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	oldBase := assetBaseURL
	assetBaseURL = srv.URL
	defer func() { assetBaseURL = oldBase }()

	dir := t.TempDir()
	store := NewAssetStore(dir)
	oldAssets := Assets
	Assets = []Asset{{"jessibuca.js", wantHex, int64(len(body))}}
	defer func() { Assets = oldAssets }()

	// 先写一个大小相同但内容错的文件
	if err := os.MkdirAll(filepath.Join(dir, "jessibuca"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := []byte("BAD!bytes-x") // 需与 body 等长
	if len(bad) != len(body) {
		bad = bad[:len(body)]
	}
	if err := os.WriteFile(filepath.Join(dir, "jessibuca", "jessibuca.js"), bad, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := store.Ensure("jessibuca.js"); err != nil {
		t.Fatalf("Ensure 应重新下载并修复：%v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "jessibuca", "jessibuca.js"))
	if string(got) != string(body) {
		t.Errorf("修复后内容不符：%q", got)
	}
}
