package chain

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/subtitle"
	"gorm.io/gorm"
)

// fakeSubProvider 测试用字幕源（无需网络）。
type fakeSubProvider struct {
	name   string
	subs   []subtitle.Subtitle
	err    error
	dlData []byte
	dlName string
}

func (f *fakeSubProvider) Name() string { return f.name }
func (f *fakeSubProvider) Search(context.Context, string, subtitle.SearchOpts) ([]subtitle.Subtitle, error) {
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
