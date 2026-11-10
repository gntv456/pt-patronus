// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package cloud

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteStrmFileMountTemplate 修复/直写路径必须与规则生成同渲染：{mount} 不得丢失
// （此前 writeStrmAt 以空 mount 渲染，6h 一轮 repair cron 把规则生成的挂载路径 .strm
// 静默改写成裸云盘路径 → Emby 当本地路径播放失败）。
func TestWriteStrmFileMountTemplate(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "x.mkv.strm")
	if _, err := WriteStrmFile(WriteStrmOptions{
		OutputDir: dir, OutPath: outPath, CloudPath: "/电影/x.mkv", FileName: "x.mkv",
		Template: "{mount}{cloudpath}", MountRoot: "/CloudDrive2/115", DriveID: 3,
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "/CloudDrive2/115/电影/x.mkv\n" {
		t.Errorf("content=%q want %q", b, "/CloudDrive2/115/电影/x.mkv\n")
	}
}

// TestWriteStrmFileDirectPublic 外网双地址模板 {directPublic} 必须被替换（此前直写路径
// 只替换 {direct}，{directPublic} 留字面占位符写进 .strm）。
func TestWriteStrmFileDirectPublic(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "y.mkv.strm")
	if _, err := WriteStrmFile(WriteStrmOptions{
		OutputDir: dir, OutPath: outPath, CloudPath: "/电影/y.mkv", FileName: "y.mkv",
		Template: "{directPublic}{cloudpath}", DriveID: 5,
		AppBaseURL: "http://nas:8080", PublicBaseURL: "http://pub:8080",
	}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "http://pub:8080/api/v1/cloud/direct/5/电影/y.mkv\n"
	if string(b) != want {
		t.Errorf("content=%q want %q", b, want)
	}
}
