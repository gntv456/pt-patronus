// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/downloader/qb"
	"gorm.io/gorm"
)

// qbCall 记录一次非登录请求：路径 + 表单里的 hashes。
type qbCall struct {
	path   string
	hashes string
}

// newRecordingQB 起一个记录请求的假 qB WebUI API（表单类请求解析出 hashes 字段）。
func newRecordingQB(t *testing.T) (*httptest.Server, *[]qbCall) {
	t.Helper()
	var mu sync.Mutex
	calls := &[]qbCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api/v2/auth/login") {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		// WebUI API 的批量端点都是 application/x-www-form-urlencoded 正文。
		_ = r.ParseForm()
		mu.Lock()
		*calls = append(*calls, qbCall{path: r.URL.Path, hashes: r.Form.Get("hashes")})
		mu.Unlock()
		_, _ = w.Write([]byte("Ok."))
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func newQBDownloadChain(t *testing.T) (*DownloadChain, uint, *[]qbCall) {
	t.Helper()
	srv, calls := newRecordingQB(t)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/dl_hash.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Downloader{Name: "qb", Type: "qb", URL: srv.URL}).Error; err != nil {
		t.Fatal(err)
	}
	dlOp := oper.NewDownloaderOper(db, nil)
	ch := NewDownloadChain(NewBase(module.New(), event.New()), manager.NewManager(dlOp), dlOp, nil)
	return ch, 1, calls
}

// TestDownloadBatchOpsRejectEmptyHashes 按 hash 定位的批量操作必须拒空。
// 回归：此前空列表一路传到 qB 适配器，joinHashes 空 → 发 "hashes=all"，
// 语义是「对下载器内全部种子操作」。前端漏选/竞态、AI 工具传 {"hashes":[]}
// （JSON Schema 的 required 只保证键存在）、自动化筛选命中 0 条，任一都会
// 变成一次全站暂停 / 改路径 / 删除（delete_files=true 时连数据文件一起没）。
func TestDownloadBatchOpsRejectEmptyHashes(t *testing.T) {
	ch, id, calls := newQBDownloadChain(t)
	ctx := context.Background()

	ops := []struct {
		name string
		call func([]string) error
	}{
		{"Pause", func(h []string) error { return ch.Pause(ctx, id, h) }},
		{"Resume", func(h []string) error { return ch.Resume(ctx, id, h) }},
		{"Delete", func(h []string) error { return ch.Delete(ctx, id, h, true) }},
		{"SetCategory", func(h []string) error { return ch.SetCategory(ctx, id, h, "movie") }},
		{"SetTags", func(h []string) error { return ch.SetTags(ctx, id, h, []string{"t"}) }},
		{"AddTags", func(h []string) error { return ch.AddTags(ctx, id, h, []string{"t"}) }},
		{"RemoveTags", func(h []string) error { return ch.RemoveTags(ctx, id, h, []string{"t"}) }},
		{"Recheck", func(h []string) error { return ch.Recheck(ctx, id, h) }},
		{"Reannounce", func(h []string) error { return ch.Reannounce(ctx, id, h) }},
		{"SetLocation", func(h []string) error { return ch.SetLocation(ctx, id, h, "/data/x") }},
		{"SetLimits", func(h []string) error {
			return ch.SetLimits(ctx, id, h, downloader.Limits{UploadLimit: 1024, DownloadLimit: 1024})
		}},
		{"SetShareLimits", func(h []string) error {
			return ch.SetShareLimits(ctx, id, h, downloader.ShareLimits{RatioLimit: 1})
		}},
		{"SetQueuePriority", func(h []string) error { return ch.SetQueuePriority(ctx, id, h, "top") }},
		{"SetForceStart", func(h []string) error { return ch.SetForceStart(ctx, id, h, true) }},
		{"ToggleSequentialDownload", func(h []string) error { return ch.ToggleSequentialDownload(ctx, id, h) }},
		{"ToggleFirstLastPiece", func(h []string) error { return ch.ToggleFirstLastPiece(ctx, id, h) }},
	}

	for _, op := range ops {
		*calls = nil
		err := op.call(nil)
		if !errors.Is(err, ErrEmptyHashes) {
			t.Errorf("%s: 空 hashes 应被拒绝，got %v", op.name, err)
		}
		if len(*calls) != 0 {
			t.Errorf("%s: 拒空必须在发请求之前，绝不能把批量操作打到下载器上，got %+v", op.name, *calls)
		}
	}

	// 正控：指定了 hash 时同样的操作照常下发（证明上面不是把批量操作整体堵死）。
	*calls = nil
	if err := ch.Delete(ctx, id, []string{"abcd1234"}, true); err != nil {
		t.Fatalf("指定 hash 的删除应执行: %v", err)
	}
	var seen []qbCall
	for _, c := range *calls {
		if strings.Contains(c.path, "/torrents/delete") {
			seen = append(seen, c)
		}
	}
	if len(seen) != 1 || seen[0].hashes != "abcd1234" {
		t.Fatalf("删除应只打到指定 hash，got %+v", *calls)
	}
}

// TestDownloadBatchOpsDropBlankHashes 「看起来选了、其实是空的」列表同样必须拒掉：
// ["", "  "] 只判 len==0 会放行，下发的是 hashes=| 这种空选择器。
// 同时证明剔除只去掉空白项、不动真实目标。
func TestDownloadBatchOpsDropBlankHashes(t *testing.T) {
	ch, id, calls := newQBDownloadChain(t)
	ctx := context.Background()

	*calls = nil
	if err := ch.Pause(ctx, id, []string{"", "  "}); !errors.Is(err, ErrEmptyHashes) {
		t.Fatalf("全空白列表应按空处理，got %v", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("全空白列表不得发出任何请求，got %+v", *calls)
	}

	*calls = nil
	if err := ch.AddTags(ctx, id, []string{"", "abcd"}, []string{" good "}); err != nil {
		t.Fatalf("含真实 hash 的列表应执行: %v", err)
	}
	var hit *qbCall
	for i := range *calls {
		if strings.Contains((*calls)[i].path, "/addTags") {
			hit = &(*calls)[i]
		}
	}
	if hit == nil || hit.hashes != "abcd" {
		t.Fatalf("应只把非空白 hash 下发，got %+v", *calls)
	}
}

// TestDownloadTagAndTrackerOpsRejectEmptyValues 取值为空时操作毫无意义，
// 但请求照发、200 照回、上层照报「已添加」——UI 自己挡了空输入，
// AI 工具/插件/工作流节点没挡，护栏必须在 chain 层。
func TestDownloadTagAndTrackerOpsRejectEmptyValues(t *testing.T) {
	ch, id, calls := newQBDownloadChain(t)
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"SetTags 空标签", func() error { return ch.SetTags(ctx, id, []string{"abcd"}, nil) }, "标签"},
		{"RemoveTags 空白标签", func() error { return ch.RemoveTags(ctx, id, []string{"abcd"}, []string{" "}) }, "标签"},
		{"AddTrackers 空 hash", func() error { return ch.AddTrackers(ctx, id, "", []string{"http://t/announce"}) }, "未指定任何种子"},
		{"RemoveTrackers 空 URL", func() error { return ch.RemoveTrackers(ctx, id, "abcd", nil) }, "Tracker 地址"},
		{"DeleteCategories 空列表", func() error { return ch.DeleteCategories(ctx, id, nil) }, "分类"},
	}
	for _, c := range cases {
		*calls = nil
		err := c.call()
		if err == nil {
			t.Errorf("%s: 空取值应被拒绝", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: 错误应含 %q，got %v", c.name, c.want, err)
		}
		if len(*calls) != 0 {
			t.Errorf("%s: 拒绝必须发生在发请求之前，got %+v", c.name, *calls)
		}
	}

	// 正控：取值齐全时照常下发。
	*calls = nil
	if err := ch.AddTags(ctx, id, []string{"abcd"}, []string{"keep"}); err != nil {
		t.Fatalf("正常加标签应执行: %v", err)
	}
}

// TestQBAdapterEmptyHashesMeansAll 把危险的来源写明（而非锁死实现）：
// qB 适配器把空列表发成 hashes=all，语义就是「全部种子」。chain 层因此必须拒空。
// 若哪天有人把 joinHashes 改成发空串，本测试会红——那正是需要重估这条护栏的时刻。
func TestQBAdapterEmptyHashesMeansAll(t *testing.T) {
	srv, calls := newRecordingQB(t)
	c, err := qb.New(srv.URL, "u", "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(context.Background(), nil, true); err != nil {
		t.Fatal(err)
	}
	var hit *qbCall
	for i := range *calls {
		if strings.Contains((*calls)[i].path, "/torrents/delete") {
			hit = &(*calls)[i]
		}
	}
	if hit == nil {
		t.Fatal("未记录到 delete 请求")
	}
	if hit.hashes != "all" {
		t.Fatalf("qB 适配器空列表语义应为 all（本测试用于说明拒空的必要），got %q", hit.hashes)
	}
}
