package chain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// fakeAdapter 是测试用站点适配器（实现 site.Adapter + module.Module）。
type fakeAdapter struct {
	schema   string
	torrents []site.Torrent
	ui       *site.UserInfo
	err      error         // Search 返回的错误（零值 nil，向后兼容已有用例）
	delay    time.Duration // Search 前的模拟耗时（零值不阻塞）
	panicOn  bool          // Search 直接 panic（模拟适配器解析畸形 HTML 崩溃）
	// panicUserinfoHost 非空时，baseURL 命中的站在 FetchUserInfo 现场 panic
	// （连通检测 TestConnections/TestConnectionsStream 的 per-site 隔离用例）。
	panicUserinfoHost string
}

func (f *fakeAdapter) Name() string   { return "fake" }
func (f *fakeAdapter) Priority() int  { return 100 }
func (f *fakeAdapter) Schema() string { return f.schema }
func (f *fakeAdapter) Search(ctx context.Context, _ *site.SiteMetadata, _, _, _, _ string, _ int) ([]site.Torrent, error) {
	if f.panicOn {
		panic("nil pointer dereference on malformed html")
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.torrents, f.err
}
func (f *fakeAdapter) FetchUserInfo(_ context.Context, _ *site.SiteMetadata, baseURL, _ string) (*site.UserInfo, error) {
	if f.panicUserinfoHost != "" && strings.Contains(baseURL, f.panicUserinfoHost) {
		panic("nil pointer dereference on malformed userinfo page")
	}
	return f.ui, nil
}

func newChainDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/chain.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	// 释放文件锁，避免 Windows 下 TempDir 清理失败
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestSiteChainDispatch(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x"})

	mm := module.New()
	mm.Register(&fakeAdapter{
		schema:   "nexusphp",
		torrents: []site.Torrent{{Title: "A", Seeders: 5}},
		ui:       &site.UserInfo{Name: "u", Level: "VIP"},
	})
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	// UserInfo 调度到 fakeAdapter
	ui, err := ch.UserInfo(context.Background(), s)
	if err != nil || ui == nil || ui.Level != "VIP" {
		t.Errorf("UserInfo dispatch: ui=%+v err=%v", ui, err)
	}

	// Search 调度
	ts, err := ch.Search(context.Background(), s, "kw", "", 1)
	if err != nil || len(ts) != 1 || ts[0].Title != "A" || ts[0].Seeders != 5 {
		t.Errorf("Search dispatch: ts=%+v err=%v", ts, err)
	}

	// 未知 schema → ErrNoAdapter
	s2, _ := siteOp.Create(oper.SiteInput{Name: "t2", Platform: "mteam", BaseURL: "https://y"})
	if _, err := ch.UserInfo(context.Background(), s2); !errors.Is(err, ErrNoAdapter) {
		t.Errorf("unknown schema err=%v, want ErrNoAdapter", err)
	}
}

func TestSiteChainDefFallback(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	// 无适配器注册时，Search 返回 ErrNoAdapter 而非 panic
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x"})
	_, err := ch.Search(context.Background(), s, "kw", "", 1)
	if !errors.Is(err, ErrNoAdapter) {
		t.Errorf("no adapter: err=%v want ErrNoAdapter", err)
	}
}
