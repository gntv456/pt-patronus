// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanOneLevelHostDirFollowsScanDir 「宿主机路径」是把容器内扫描根换成 COMPOSE_ROOT
// 算出来的。从前根目录写死 "/host/compose"：运维用 COMPOSE_SCAN_DIR 换挂载点时，
// 前缀对不上 → 每个项目的宿主机路径原样显示容器内路径，用户照着去宿主机找不到文件，
// 而界面把它当成正常数据展示（没有任何「换算没生效」的痕迹）。
func TestScanOneLevelHostDirFollowsScanDir(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "media-stack")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COMPOSE_SCAN_DIR", root)
	t.Setenv("COMPOSE_ROOT", "/srv")

	var got []composeProject
	var hide1 []string
	scanOneLevel(proj, &got, &hide1)
	if len(got) != 1 {
		t.Fatalf("应扫到一个项目, got %d", len(got))
	}
	if want := "/srv/media-stack"; got[0].HostDir != want {
		t.Errorf("换了扫描根后宿主机路径要能换算出来: want %q got %q（旧行为把容器内路径原样显示）", want, got[0].HostDir)
	}

	// 对照：默认根（/host/compose）下的目录不做换算——别把宿主机上任意路径都拼到 COMPOSE_ROOT 上。
	t.Setenv("COMPOSE_SCAN_DIR", "")
	var got2 []composeProject
	var hide2 []string
	scanOneLevel(proj, &got2, &hide2)
	if len(got2) != 1 {
		t.Fatalf("默认根下仍应扫到项目, got %d", len(got2))
	}
	if got2[0].HostDir != proj {
		t.Errorf("扫描根之外的目录不该被改写: want %q got %q", proj, got2[0].HostDir)
	}
}
