package chain

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// spyGame 记录 Play 有没有真的打出去、以及带出去的凭据长什么样。
type spyGame struct {
	id    string
	calls int32
	creds chan entertainment.Creds
}

func (g *spyGame) ID() string   { return g.id }
func (g *spyGame) Name() string { return "spy" }
func (g *spyGame) Play(_ context.Context, _ *httpclient.Client, _ string, cr entertainment.Creds) (*entertainment.PlayResult, error) {
	atomic.AddInt32(&g.calls, 1)
	g.creds <- cr
	return &entertainment.PlayResult{Success: true, Message: "ok"}, nil
}

func newSpy(id string) *spyGame {
	return &spyGame{id: id, creds: make(chan entertainment.Creds, 1)}
}

// TestPlayStopsWhenSiteCredentialUndecryptable 回归：resolve 从前写成
// `cookie, _ := siteOp.Cookie(s)`——主密钥轮换（或备份恢复到新实例）之后库里明明还留着
// CookieEnc，解密错误被丢掉，链层拿着**空 cookie** 照样去打站点：
// 用户看到「游戏失败」「余额 0」，而这条路是要消耗游戏币的，且根本不知道该去重新保存凭据。
func TestPlayStopsWhenSiteCredentialUndecryptable(t *testing.T) {
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{
		Name: "幻月", Platform: "nexusphp", BaseURL: "https://game.test",
		Cookie: "uid=7; pass=secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	game := newSpy("g1")
	reg := entertainment.NewRegistry()
	reg.Register("game.test", game)

	rotated := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil,
		oper.NewSiteOper(db, security.NewCrypto("rotated-key")))

	res, err := rotated.Play(context.Background(), s.ID, "g1")
	if err == nil {
		t.Fatal("凭据解不开时必须失败，不能拿着空 cookie 去打站点")
	}
	if !errors.Is(err, ErrSiteCredentialUnreadable) {
		t.Errorf("要能被判成「站点凭据读不出来」这一档，got %v", err)
	}
	if res != nil {
		t.Errorf("失败时不该返回结果：%+v", res)
	}
	// 花钱路径的执行级红证：一次站点请求都没发出去。
	if got := atomic.LoadInt32(&game.calls); got != 0 {
		t.Errorf("Play 实际进入次数 = %d, want 0（发了请求就可能已扣游戏币）", got)
	}
	if len(game.creds) != 0 {
		t.Errorf("不该有凭据被带出去：%+v", <-game.creds)
	}
	// 原因要指引下一步（去哪儿修），并钉死「没发请求」，而不是含糊的「游戏失败」。
	for _, want := range []string{"幻月", "无法解密", "重新保存", "未向站点发请求"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里缺 %q：%q", want, err.Error())
		}
	}
}

// 正控一：主密钥没换过 → 同一份库照常打得着站点，且解密后的 cookie 真的传进去了
// （否则上面的「拦下来」可能只是测试自己搭坏）。
func TestPlayStillRunsWhenCredentialReadable(t *testing.T) {
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{
		Name: "幻月", Platform: "nexusphp", BaseURL: "https://game.test",
		Cookie: "uid=7; pass=secret", Username: "me", Password: "pw",
	})
	if err != nil {
		t.Fatal(err)
	}
	game := newSpy("g1")
	reg := entertainment.NewRegistry()
	reg.Register("game.test", game)
	same := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil,
		oper.NewSiteOper(db, security.NewCrypto("old-key")))

	if _, err := same.Play(context.Background(), s.ID, "g1"); err != nil {
		t.Fatalf("同一把主密钥下应照常可用：%v", err)
	}
	if got := atomic.LoadInt32(&game.calls); got != 1 {
		t.Fatalf("Play 进入次数 = %d, want 1", got)
	}
	cr := <-game.creds
	if cr.Cookie != "uid=7; pass=secret" || cr.Password != "pw" || cr.Username != "me" {
		t.Errorf("传给站点的凭据不对：%+v", cr)
	}
}

// 正控二：本来就没配凭据的站点（Decrypt("") 归一成 ("", nil)）在换过主密钥的实例上
// 也不许被判成「读不出来」——否则这条修复会把新站点的娱乐功能整体说成坏了。
func TestPlayAllowsSiteWithoutCredentials(t *testing.T) {
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("old-key"))
	s, err := writer.Create(oper.SiteInput{Name: "无凭据站", Platform: "pt", BaseURL: "https://game2.test"})
	if err != nil {
		t.Fatal(err)
	}
	game := newSpy("g2")
	reg := entertainment.NewRegistry()
	reg.Register("game2.test", game)
	rotated := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil,
		oper.NewSiteOper(db, security.NewCrypto("rotated-key")))

	if _, err := rotated.Play(context.Background(), s.ID, "g2"); err != nil {
		t.Fatalf("没配凭据的站点不该被判成解不开：%v", err)
	}
	if got := atomic.LoadInt32(&game.calls); got != 1 {
		t.Errorf("Play 进入次数 = %d, want 1", got)
	}
}

// TestFarmReportsUnsupportedViaSentinel 回归：entertainment_actions.go 里 26 处
// 「该游戏不支持 X」从前是裸 errors.New，接口层的分类只认 entertainment.go 那 15 处，
// 于是农场/押注/五子棋这些面板把「这个游戏没有该功能」报成 502「上游服务不可用，
// 是否已扣费以站点页面为准」——用户既不用核对余额，也得不到「换游戏」的指引。
func TestFarmReportsUnsupportedViaSentinel(t *testing.T) {
	db := newChainDB(t)
	writer := oper.NewSiteOper(db, security.NewCrypto("k"))
	s, err := writer.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://game.test"})
	if err != nil {
		t.Fatal(err)
	}
	reg := entertainment.NewRegistry()
	reg.Register("game.test", newSpy("g1")) // spy 只实现 Play，不是 Farmable
	ch := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil,
		oper.NewSiteOper(db, security.NewCrypto("k")))

	_, err = ch.Farm(context.Background(), s.ID, "g1")
	if err == nil {
		t.Fatal("非 Farmable 游戏调农场必须失败")
	}
	if !errors.Is(err, ErrGameUnsupported) {
		t.Errorf("「不支持」必须挂上哨兵，才能被接口层认出这一档，got %v", err)
	}
	if !strings.Contains(err.Error(), "农场") {
		t.Errorf("要说清不支持的是哪项能力：%q", err.Error())
	}
}
