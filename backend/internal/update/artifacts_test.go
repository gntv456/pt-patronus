// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package update

import (
	"os"
	"path/filepath"
	"testing"
)

// writeArtifact 造一个内容确定的规范产物文件（sha256 随内容可预测）。
func writeArtifact(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
	if err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestParseArtifactName(t *testing.T) {
	cases := []struct {
		in           string
		wantPlatform string
		wantVersion  string
	}{
		{"android-1.03.31.apk", "android", "1.03.31"},
		{"ohos-1.03.31.hap", "ohos", "1.03.31"},
		{"ptpatronus-windows-1.03.31.zip", "windows", "1.03.31"},
		{"server-linux-amd64-1.03.31.tar.gz", "server-linux-amd64", "1.03.31"},
		{"agent-linux-arm64-0.9.1.gz", "agent-linux-arm64", "0.9.1"},
		{"ios-1.03.31.IPA", "ios", "1.03.31"}, // 大写后缀容错
		{"random-1.0.txt", "", ""},            // 后缀不在白名单
		{"readme.md", "", ""},                 // 非产物
		{"-1.03.31.apk", "", ""},              // 空 platform
		{"android-.apk", "", ""},              // 空 version
		{"unknownplat-1.0.apk", "", ""},       // platform 不在白名单
	}
	for _, c := range cases {
		p, v := ParseArtifactName(c.in)
		if p != c.wantPlatform || v != c.wantVersion {
			t.Errorf("ParseArtifactName(%q) = (%q, %q), want (%q, %q)",
				c.in, p, v, c.wantPlatform, c.wantVersion)
		}
	}
}

func TestScanArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeArtifact(t, dir, "android-1.03.30.apk", "old")
	writeArtifact(t, dir, "android-1.03.31.apk", "newest")
	writeArtifact(t, dir, "ohos-1.03.31.hap", "hap")
	writeArtifact(t, dir, "windows-1.03.31.zip", "win")
	writeArtifact(t, dir, "macos-1.03.31.zip", "mac")
	writeArtifact(t, dir, "release-notes.txt", "junk") // 非规范名：跳过

	m, err := ScanArtifacts(dir, nil)
	if err != nil {
		t.Fatalf("ScanArtifacts: %v", err)
	}
	if m == nil || m.LatestVersion != "1.03.31" {
		t.Fatalf("LatestVersion = %v, want 1.03.31", m)
	}
	for _, plat := range []string{"android", "ohos", "windows", "macos"} {
		tg, ok := m.Targets[plat]
		if !ok {
			t.Fatalf("targets 缺 %s", plat)
		}
		want := "/api/v1/system/downloads/" + plat + "-1.03.31."
		if plat == "android" {
			want += "apk"
		} else if plat == "ohos" {
			want += "hap"
		} else {
			want += "zip"
		}
		if tg.URL != want {
			t.Errorf("%s url = %q, want %q", plat, tg.URL, want)
		}
		if tg.SHA256 == "" || len(tg.SHA256) != 64 {
			t.Errorf("%s sha256 非法: %q", plat, tg.SHA256)
		}
	}
	// android 必须挂的是最新版本文件（1.03.30 被忽略）。
	if tg := m.Targets["android"]; !endsWith(tg.URL, "android-1.03.31.apk") {
		t.Errorf("android 应挂最新版本: %s", tg.URL)
	}
}

func TestScanArtifactsReleaseMeta(t *testing.T) {
	dir := t.TempDir()
	writeArtifact(t, dir, "android-1.03.31.apk", "x")
	writeArtifact(t, dir, "release.json",
		`{"releaseName":"体验优化","releaseNotes":"修复分发包过期","minVersion":"0.60.00"}`)

	m, err := ScanArtifacts(dir, nil)
	if err != nil {
		t.Fatalf("ScanArtifacts: %v", err)
	}
	if m.ReleaseName != "体验优化" || m.ReleaseNotes != "修复分发包过期" {
		t.Fatalf("release.json 字段未合并: %+v", m)
	}
	if m.MinVersion != "0.60.00" {
		t.Errorf("release.json 字段未合并: %+v", m)
	}

	// 无 release.json 时保留 prev 的人工字段（一次扫描不许抹掉发布说明）。
	writeArtifact(t, dir, "release.json.tmp~", "x") // 干扰项（非 release.json）
	os.Remove(filepath.Join(dir, "release.json"))
	prev := &Manifest{ReleaseName: "人工填写", ReleaseNotes: "人工说明"}
	m2, err := ScanArtifacts(dir, prev)
	if err != nil {
		t.Fatalf("ScanArtifacts(prev): %v", err)
	}
	if m2.ReleaseName != "人工填写" || m2.ReleaseNotes != "人工说明" {
		t.Errorf("prev 人工字段丢失: %+v", m2)
	}
}

func TestScanArtifactsEmpty(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	if m, err := ScanArtifacts(missing, nil); m != nil || err != nil {
		t.Errorf("目录不存在应 (nil,nil), got (%v,%v)", m, err)
	}
	empty := t.TempDir()
	if m, err := ScanArtifacts(empty, nil); m != nil || err != nil {
		t.Errorf("空目录应 (nil,nil), got (%v,%v)", m, err)
	}
	// ApplyArtifacts 对空目录不动现有 overlay。
	// 同包直构 Service（同 gate 测试口径），overlayPath 指向临时目录。
	s := &Service{overlayPath: filepath.Join(t.TempDir(), "update-manifest.json")}
	prev := &Manifest{LatestVersion: "9.9.9", Targets: map[string]Target{}}
	if err := s.SetOverlay(prev); err != nil {
		t.Fatalf("SetOverlay: %v", err)
	}
	if err := s.ApplyArtifacts(empty); err != nil {
		t.Fatalf("ApplyArtifacts(empty): %v", err)
	}
	if got := s.OverlaySnapshot(); got == nil || got.LatestVersion != "9.9.9" {
		t.Errorf("空目录扫描不应动现有清单: %+v", got)
	}
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
