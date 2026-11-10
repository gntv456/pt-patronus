// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"gorm.io/gorm"
)

// #277/#278 接口层取证。路径照生产抄（routes_entertainment.go：/api/v1 + ent group），
// 域内码本身即证明打到了路由（NoRoute 只会回通用 not_found）。
//
// 站点凭据解不开（主密钥换过）时，娱乐域四条出口从前各有各的错法：
// Play 拿着空 cookie 真的去打站点（可能已扣游戏币，界面只说「参数有误」/「上游不可用」）；
// gobang-state 兜底成裸 BadGateway「上游服务不可用，请稍后重试」；
// farm / medal-tree 干脆回 400「请求参数有误，请检查后重试」——而这一项没有输入可检查。

// credsSpy 只实现 Play：既能验花钱出口确实没出门，也天然不是 Farmable/Gobangable/MedalTreeable。
type credsSpy struct {
	id    string
	calls int32
}

func (g *credsSpy) ID() string   { return g.id }
func (g *credsSpy) Name() string { return "spy" }
func (g *credsSpy) Play(_ context.Context, _ *httpclient.Client, _ string, _ entertainment.Creds) (*entertainment.PlayResult, error) {
	atomic.AddInt32(&g.calls, 1)
	return &entertainment.PlayResult{Success: true}, nil
}

// entCredRouter 用 credKey 写站点凭据、用 readKey 建链（两者不同即「换过主密钥」）。
func entCredRouter(t *testing.T, credKey, readKey string, withCookie bool) (*gin.Engine, *credsSpy, uint) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ent.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	if readKey == "" {
		readKey = credKey
	}
	in := oper.SiteInput{Name: "幻月府", Platform: "nexusphp", BaseURL: "https://game.test"}
	if withCookie {
		in.Cookie = "uid=7; pass=secret"
		in.Password = "pw"
	}
	s, err := oper.NewSiteOper(db, security.NewCrypto(credKey)).Create(in)
	if err != nil {
		t.Fatal(err)
	}
	game := &credsSpy{id: "g1"}
	reg := entertainment.NewRegistry()
	reg.Register("game.test", game)
	h := NewEntertainmentHandler(
		chain.NewEntertainmentChain(chain.NewBase(module.New(), event.New()), reg, nil,
			oper.NewSiteOper(db, security.NewCrypto(readKey))), nil, nil)

	r := gin.New()
	r.POST("/api/v1/entertainment/:id/play", h.Play)
	r.GET("/api/v1/entertainment/:id/games/:game/gobang-state", h.GobangState)
	r.GET("/api/v1/entertainment/:id/games/:game/farm", h.Farm)
	r.GET("/api/v1/entertainment/:id/games/:game/medal-tree", h.MedalTree)
	return r, game, s.ID
}

func doEntReq(t *testing.T, r *gin.Engine, method, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	var env map[string]any
	if e := json.Unmarshal(rec.Body.Bytes(), &env); e != nil {
		t.Fatalf("%s %s → %d，响应不是 JSON：%q", method, path, rec.Code, rec.Body.String())
	}
	return rec.Code, env
}

func envStr(env map[string]any, k string) string {
	v, _ := env[k].(string)
	return v
}

// TestCredentialUnreadableOutlets 凭据解不开：四条出口一律 503 + 点名站点 + 说「没发请求」，
// 且一次站点请求都不许发出去。
func TestCredentialUnreadableOutlets(t *testing.T) {
	cases := []struct{ name, method, path, body string }{
		{"花钱 Play", http.MethodPost, "/api/v1/entertainment/%d/play", `{"game":"g1"}`},
		{"五子棋全景（gobangSiteErr 出口）", http.MethodGet, "/api/v1/entertainment/%d/games/g1/gobang-state", ""},
		{"农场全景（medals 出口，从前 400 参数有误）", http.MethodGet, "/api/v1/entertainment/%d/games/g1/farm", ""},
		{"勋章树（medals 只读出口）", http.MethodGet, "/api/v1/entertainment/%d/games/g1/medal-tree", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, game, id := entCredRouter(t, "old-key", "rotated-key", true)
			status, env := doEntReq(t, r, tc.method, fmt.Sprintf(tc.path, id), tc.body)
			if status != http.StatusServiceUnavailable || envStr(env, "code") != errcode.EntertainmentCredentialUnreadable.Code {
				t.Fatalf("status=%d 整包=%v，期望 503 + %q", status, env, errcode.EntertainmentCredentialUnreadable.Code)
			}
			for _, want := range []string{"幻月府", "无法解密", "重新保存", "未向站点发请求"} {
				if !strings.Contains(envStr(env, "message"), want) {
					t.Errorf("界面文案缺 %q：%q", want, envStr(env, "message"))
				}
			}
			// 「参数有误」会让人去改没得改的输入；「核对余额」会让人以为钱已经扣了；
			// 「上游服务不可用，请稍后重试」会让人反复重试同一件永远不会成功的事。
			for _, bad := range []string{"参数有误", "核对余额", "上游服务不可用"} {
				if strings.Contains(envStr(env, "message"), bad) {
					t.Errorf("说成了 %q：%q", bad, envStr(env, "message"))
				}
			}
			if got := atomic.LoadInt32(&game.calls); got != 0 {
				t.Errorf("站点请求发了 %d 次，want 0", got)
			}
		})
	}
}

// 正控：主密钥没换过时，这三条只读出口应红在「这个游戏不支持该面板」那一档（409），
// Play 则照常打得通——否则上面的 503 可能只是测试自己搭坏。
func TestCredentialReadableOutletsUnaffected(t *testing.T) {
	t.Run("只读全景 409 且点名能力", func(t *testing.T) {
		r, game, id := entCredRouter(t, "same-key", "", true)
		for _, tc := range []struct{ path, want string }{
			{"/api/v1/entertainment/%d/games/g1/farm", "农场"},
			{"/api/v1/entertainment/%d/games/g1/gobang-state", "五子棋"},
			{"/api/v1/entertainment/%d/games/g1/medal-tree", "勋章"},
		} {
			status, env := doEntReq(t, r, http.MethodGet, fmt.Sprintf(tc.path, id), "")
			if status != http.StatusConflict || envStr(env, "code") != errcode.EntertainmentGameUnsupported.Code {
				t.Fatalf("%s → status=%d 整包=%v，期望 409 + %q", tc.path, status, env, errcode.EntertainmentGameUnsupported.Code)
			}
			if !strings.Contains(envStr(env, "message"), tc.want) {
				t.Errorf("要说清不支持的是哪项能力：%q", envStr(env, "message"))
			}
			if !strings.Contains(envStr(env, "message"), "该游戏不支持") {
				t.Errorf("链层的人话原样抵达界面：%q", envStr(env, "message"))
			}
		}
		if got := atomic.LoadInt32(&game.calls); got != 0 {
			t.Errorf("这三个面板不该调 Play，实际 %d 次", got)
		}
	})

	t.Run("Play 在密钥没换过时照旧走得通", func(t *testing.T) {
		r, game, id := entCredRouter(t, "same-key", "", true)
		status, env := doEntReq(t, r, http.MethodPost, fmt.Sprintf("/api/v1/entertainment/%d/play", id), `{"game":"g1"}`)
		if status != http.StatusOK {
			t.Fatalf("status=%d 整包=%v，期望 200（正控：健康站点不许被说成坏了）", status, env)
		}
		if got := atomic.LoadInt32(&game.calls); got != 1 {
			t.Errorf("Play 进入次数 = %d, want 1", got)
		}
	})
}

// 正控：本来就没配凭据的站点（Decrypt("") 归一成 ("", nil)）在换过主密钥后
// 也不许被判成「解不开」——否则这条修复会把新站点的娱乐功能整体说成坏了。
func TestSiteWithoutCredentialsNotFlaggedUnreadable(t *testing.T) {
	r, _, id := entCredRouter(t, "old-key", "rotated-key", false)
	status, env := doEntReq(t, r, http.MethodGet, fmt.Sprintf("/api/v1/entertainment/%d/games/g1/farm", id), "")
	if envStr(env, "code") == errcode.EntertainmentCredentialUnreadable.Code {
		t.Fatalf("没配凭据的站点被判成凭据读不出来：status=%d 整包=%v", status, env)
	}
	if status != http.StatusConflict {
		t.Fatalf("应照常走到「不支持农场」这一档：status=%d 整包=%v", status, env)
	}
}
