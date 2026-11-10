// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/transfer"
	"gorm.io/gorm"
)

// TestRepairStrmKeepsRuleRender 修复链路必须按记录所属规则忠实重渲染：
// ① 显式 {mount}{cloudpath} 模板：mount 不得丢（此前 mount 恒空 → 裸云盘路径）；
// ② 空模板规则（生成侧默认 {mount}{cloudpath}）：不得被套成上传直链模板；
// ③ 修复后记录 strm_path/size 保留（此前先清空再写、写后不回填 → 永久退化为登记型）。
func TestRepairStrmKeepsRuleRender(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	out := t.TempDir()
	mkRule := func(name, cloudPath, tmpl string) {
		if _, err := ch.CreateRule(oper.StrmRuleInput{
			Name: name, DriveID: 3, CloudPath: cloudPath, OutputDir: out,
			MountRoot: "/CloudDrive2/115", Template: tmpl, Recursive: boolPtr(true),
		}); err != nil {
			t.Fatal(err)
		}
	}
	mkRule("电影", "/电影", "{mount}{cloudpath}")
	mkRule("剧集", "/剧集", "") // 空模板：生成侧默认 {mount}{cloudpath}

	// 模拟规则生成落盘的 .strm（生成侧同样渲染路径）
	gen := func(cloudPath string, tmpl string) string {
		p := filepath.Join(out, filepath.Base(cloudPath)+".strm")
		if _, err := cloud.WriteStrmFile(cloud.WriteStrmOptions{
			OutputDir: out, CloudPath: cloudPath, Template: tmpl, MountRoot: "/CloudDrive2/115",
			FileName: filepath.Base(cloudPath), DriveID: 3, OutPath: p,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := ch.ruleOp.UpsertRecord(3, cloudPath, p, 123, 0); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p1 := gen("/电影/x.mkv", "{mount}{cloudpath}")
	p2 := gen("/剧集/S01E01.mp4", cloud.DefaultStrmTemplate)

	res, err := ch.RepairStrm(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.Repaired != 2 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("repaired=%d skipped=%d failed=%d, want 2/0/0", res.Repaired, res.Skipped, res.Failed)
	}
	b1, _ := os.ReadFile(p1)
	if string(b1) != "/CloudDrive2/115/电影/x.mkv\n" {
		t.Errorf("mount 模板被改写: %q", b1)
	}
	b2, _ := os.ReadFile(p2)
	if string(b2) != "/CloudDrive2/115/剧集/S01E01.mp4\n" {
		t.Errorf("空模板规则应沿用 {mount}{cloudpath}: %q", b2)
	}
	recs, _ := ch.ruleOp.ListRecords(3)
	for _, rec := range recs {
		if rec.StrmPath == "" {
			t.Errorf("修复后记录 strm_path 不得被清空: %+v", rec)
		}
		if rec.Size != 123 {
			t.Errorf("修复不应清 size: %+v", rec)
		}
	}
}

// TestRepairStrmWriteFailureKeepsRecord .strm 写入失败时记录必须保持原样
// （此前 UpsertRecord 先清 strm_path 再试写，失败后记录永久退化为登记型、永不重试）。
func TestRepairStrmWriteFailureKeepsRecord(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	strmPath := filepath.Join(blocked, "x.strm") // 父路径是文件 → 写入必失败
	if _, err := ch.ruleOp.UpsertRecord(3, "/电影/y.mkv", strmPath, 9, 0); err != nil {
		t.Fatal(err)
	}
	res2, err := ch.RepairStrm(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	// 写不进去必须是 Failed 而不是 Skipped：混在一起时「修复 0 / 跳过 1」看起来
	// 像「这条没什么可修的」，而实际是那个 .strm 至今仍是坏的。
	if res2.Repaired != 0 || res2.Skipped != 0 || res2.Failed != 1 {
		t.Fatalf("repaired=%d skipped=%d failed=%d, want 0/0/1", res2.Repaired, res2.Skipped, res2.Failed)
	}
	if len(res2.Errors) != 1 || !strings.Contains(res2.Errors[0], "x.strm") {
		t.Errorf("Failed 要留下哪个文件坏的原因，got %v", res2.Errors)
	}
	recs, _ := ch.ruleOp.ListRecords(3)
	if len(recs) != 1 || recs[0].StrmPath != strmPath || recs[0].Size != 9 {
		t.Errorf("写入失败时记录应保持原样（不清 strm_path/size）: %+v", recs)
	}
}

// TestUploadStrmBackfillsRecordAndResigns 上传型 .strm：UpsertStrm 必须回填记录 strm_path，
// ResignAllStrm/RepairStrm 才能重签（此前记录 strm_path 恒空 → 签名 7 天过期后无人重签）。
func TestUploadStrmBackfillsRecordAndResigns(t *testing.T) {
	ch, _, _ := newCloudTaskTestChain(t)
	ch.SetDirectLinkSignKey([]byte("test-sign-key"))
	out := t.TempDir()
	ctx := context.Background()

	if err := ch.UpsertStrm(ctx, 9, "/媒体/电影/a.mkv", out, ""); err != nil {
		t.Fatal(err)
	}
	recs, _ := ch.ruleOp.ListRecords(9)
	if len(recs) != 1 || recs[0].StrmPath == "" {
		t.Fatalf("上传型记录必须回填 strm_path: %+v", recs)
	}
	strmPath := recs[0].StrmPath
	b, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "?sign=") {
		t.Fatalf("上传 .strm 应带直链签名: %q", b)
	}
	// 模拟签名过期/被改写（无签名旧内容）→ RepairStrm 必须重签回填
	if err := os.WriteFile(strmPath, []byte("/api/v1/cloud/direct/9/媒体/电影/a.mkv\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res3, err := ch.RepairStrm(ctx, 9)
	if err != nil {
		t.Fatal(err)
	}
	if res3.Repaired != 1 || res3.Failed != 0 {
		t.Fatalf("修复应覆盖上传型记录, repaired=%d failed=%d", res3.Repaired, res3.Failed)
	}
	b, _ = os.ReadFile(strmPath)
	if !strings.Contains(string(b), "?sign=") {
		t.Errorf("RepairStrm 未重签: %q", b)
	}
}

// fakeCloudSink 上传+STRM 双能力假实现（只测 emitCloudStrm 的上传成功过滤）。
type fakeCloudSink struct {
	fail    map[string]bool
	upserts []string
}

func (f *fakeCloudSink) UploadFile(ctx context.Context, driveID uint, cloudPath, localPath string, size int64) error {
	if f.fail[localPath] {
		return errors.New("upload boom")
	}
	return nil
}

func (f *fakeCloudSink) UpsertStrm(ctx context.Context, driveID uint, cloudPath, outputDir, template string) error {
	f.upserts = append(f.upserts, cloudPath)
	return nil
}

// TestEmitCloudStrmOnlyUploaded 上传失败的文件不得登记 StrmRecord/写 .strm
// （此前对 res.Files 全量 emit：为云端不存在的对象写 .strm + 触发订阅追更误报）。
func TestEmitCloudStrmOnlyUploaded(t *testing.T) {
	lib := t.TempDir()
	fileA := filepath.Join(lib, "A.mkv")
	fileB := filepath.Join(lib, "B.mkv")
	for _, p := range []string{fileA, fileB} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/cfg.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	scOp := oper.NewSystemConfigOper(db)
	raw, _ := json.Marshal(map[string]any{
		"enabled": true, "movie_dir": lib,
		"cloud_upload": map[string]any{
			"enabled": true, "drive_id": 5, "cloud_base": "/media",
			"upload_videos": true, "strm_output_dir": t.TempDir(),
		},
	})
	if err := scOp.Set("transfer.config", string(raw)); err != nil {
		t.Fatal(err)
	}

	sink := &fakeCloudSink{fail: map[string]bool{fileB: true}}
	tc := &TransferChain{Base: NewBase(module.New(), event.New()), cfg: scOp}
	tc.SetCloudUploader(sink)
	res := &transfer.Result{Files: []string{fileA, fileB}, DestDir: lib}
	tc.afterTransferCloudUpload(context.Background(), res, transfer.Load(scOp))
	tc.cloudWg.Wait()

	if len(sink.upserts) != 1 || sink.upserts[0] != "/media/A.mkv" {
		t.Errorf("只应为上传成功文件登记 STRM, got %v", sink.upserts)
	}
}

// TestSanitizeCloudName 云盘名称钳制：路径分隔与 ../ 上溯不得穿透（防逃出 RootPath）。
func TestSanitizeCloudName(t *testing.T) {
	ok := map[string]string{
		"x": "x", " ../evil": "evil", `..\evil`: "evil", "a/b": "b", " .hidden": ".hidden",
	}
	for in, want := range ok {
		got, err := sanitizeCloudName(in)
		if err != nil || got != want {
			t.Errorf("sanitizeCloudName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", ".", "..", "/", "a/..", `..\..`} {
		if got, err := sanitizeCloudName(in); err == nil {
			t.Errorf("sanitizeCloudName(%q) should fail, got %q", in, got)
		}
	}
}
