// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/subtitle"
	"gorm.io/gorm"
)

// fakeSubProvider 测试用字幕源（无需网络）。panicOn=true 时 Search 现场 panic
// （模拟解析器崩在畸形结果页，验 per-source 隔离与「全源崩≠没字幕」）。
type fakeSubProvider struct {
	name    string
	subs    []subtitle.Subtitle
	err     error
	dlData  []byte
	dlName  string
	panicOn bool
}

func (f *fakeSubProvider) Name() string { return f.name }
func (f *fakeSubProvider) Search(context.Context, string, subtitle.SearchOpts) ([]subtitle.Subtitle, error) {
	if f.panicOn {
		panic("nil pointer dereference on malformed subtitle page")
	}
	return f.subs, f.err
}
func (f *fakeSubProvider) Download(context.Context, subtitle.Subtitle) ([]byte, string, error) {
	return f.dlData, f.dlName, nil
}

// newSubtitleTestProvider 构建接内存 DB 的 provider（可注入 fake 源覆盖）。
func newSubtitleTestProvider(t *testing.T, targetLang string, fakes ...subtitle.Provider) *SubtitleSourceProvider {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sub.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	cfgOp := oper.NewSystemConfigOper(db)
	if targetLang != "" {
		_ = cfgOp.Set(cfgKeySubtitleTarget, targetLang)
	}
	p, err := NewSubtitleSourceProvider(cfgOp, SubtitleSettings{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	p.mu.Lock()
	p.providers = fakes // 覆盖为 fake（rebuild 已建好真实源，此处替换）
	p.mu.Unlock()
	return p
}

func TestSubtitleChainSearchAggregate(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "a", subs: []subtitle.Subtitle{
			{Source: "a", SourceID: "1", Title: "低分", LangCode: "chs", Score: 1},
			{Source: "a", SourceID: "2", Title: "高分", LangCode: "chs", Score: 99},
		}},
		&fakeSubProvider{name: "b", err: errFake}, // 失败源应被跳过，不阻断
		&fakeSubProvider{name: "c", subs: []subtitle.Subtitle{
			{Source: "c", SourceID: "9", Title: "中分", LangCode: "eng", Score: 50},
		}},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, err := ch.Search(context.Background(), "q", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("want 3 aggregated, got %d", len(res))
	}
	// 按 Score 降序：99, 50, 1
	if res[0].Score != 99 || res[2].Score != 1 {
		t.Errorf("sort wrong: %+v", []int{res[0].Score, res[1].Score, res[2].Score})
	}
}

func TestSubtitleChainSearchLanguageFilter(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "a", subs: []subtitle.Subtitle{
			{Source: "a", SourceID: "1", LangCode: "chs"},
			{Source: "a", SourceID: "2", LangCode: "cht"},
			{Source: "a", SourceID: "3", LangCode: "eng"},
			{Source: "a", SourceID: "4", LangCode: "bi"}, // 双语始终保留
		}},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, _ := ch.Search(context.Background(), "q", "chs")
	// chs + bi 两条（cht/eng 被过滤；bi 双语始终保留）
	if len(res) != 2 {
		t.Errorf("language filter: want 2 (chs+bi), got %d", len(res))
	}
}

func TestSubtitleChainDownloadConvert(t *testing.T) {
	// fake 源返回 zip（内含繁体 .srt），chain 应解压 + 繁转简（target=chs）
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	fw, _ := zw.Create("sub.srt")
	fw.Write([]byte("1\n00:00:01 --> 00:00:02\n繁體字幕\n"))
	zw.Close()

	p := newSubtitleTestProvider(t, "chs",
		&fakeSubProvider{name: "a", dlData: buf.Bytes(), dlName: "pkg.zip"},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	out, name, err := ch.Download(context.Background(), subtitle.Subtitle{
		Source: "a", SourceID: "1", LangCode: "cht", FileName: "sub.srt", Ext: ".srt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "sub.srt" {
		t.Errorf("name=%q want sub.srt", name)
	}
	if !bytes.Contains(out, []byte("繁体字幕")) {
		t.Errorf("繁转简失败，got %q", out)
	}
	if bytes.Contains(out, []byte("繁體")) {
		t.Errorf("未完成繁转简，仍含繁体: %q", out)
	}
}

func TestShouldConvert(t *testing.T) {
	cases := []struct {
		ext, src, target string
		want             bool
	}{
		{".srt", "cht", "chs", true},
		{".srt", "chs", "cht", true},
		{".srt", "chs", "chs", false}, // 相同
		{".srt", "bi", "chs", false},  // 双语不转
		{".srt", "eng", "chs", false}, // 英文不转
		{".mp4", "cht", "chs", false}, // 非文本字幕
		{".srt", "cht", "", false},    // 无目标
	}
	for _, c := range cases {
		if got := shouldConvert(c.ext, c.src, c.target); got != c.want {
			t.Errorf("shouldConvert(%q,%q,%q)=%v want %v", c.ext, c.src, c.target, got, c.want)
		}
	}
}

// errFake 任意非 ErrNotConfigured 错误（模拟源失败）。
var errFake = bytesError("source down")

type bytesError string

func (b bytesError) Error() string { return string(b) }

// TestSubtitleSearchReportsAllSourcesFailed 全部源都崩掉时 Search 必须报错，而不是回「空列表 + nil」：
// 修复前每个崩掉的源留下 outs[i] 零值（err=nil），下游当成「这个源没搜到字幕」，于是
// 「一个源都没读回来」和「没找到字幕」在页面上完全同形（健康中心据此判定「无字幕」也是同一条链）。
func TestSubtitleSearchReportsAllSourcesFailed(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "a", panicOn: true},
		&fakeSubProvider{name: "b", panicOn: true},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, err := ch.Search(context.Background(), "q", "")
	if err == nil {
		t.Fatalf("全部源崩溃必须报错（否则与「没找到字幕」同形），got res=%+v", res)
	}
	if !strings.Contains(err.Error(), "共 2 个源") {
		t.Errorf("错误要点明参与源数（63/69 口径），got %q", err)
	}
	if strings.Contains(err.Error(), "subtitle sources all failed") {
		t.Errorf("给用户看的句子不得夹英文 id（handler 用 WithDetail 把整句送进响应体），got %q", err)
	}
	if res != nil {
		t.Errorf("全源失败不该返回半截结果，got %+v", res)
	}
}

// TestSubtitleSearchSurvivesOnePanickingSource 单源崩溃不阻断其它源：仍出结果、不报错
// （反向对照，防我把聚合错误写成「有失败就报错」）。
func TestSubtitleSearchSurvivesOnePanickingSource(t *testing.T) {
	p := newSubtitleTestProvider(t, "",
		&fakeSubProvider{name: "boom", panicOn: true},
		&fakeSubProvider{name: "ok", subs: []subtitle.Subtitle{{Source: "ok", SourceID: "1", Title: "字幕", Score: 5}}},
	)
	ch := NewSubtitleChain(NewBase(module.New(), event.New()), p)
	res, err := ch.Search(context.Background(), "q", "")
	if err != nil {
		t.Fatalf("部分源失败不应报错：%v", err)
	}
	if len(res) != 1 || res[0].Title != "字幕" {
		t.Errorf("正常源的结果要照常出，got %+v", res)
	}
}
