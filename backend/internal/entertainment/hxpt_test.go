// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

func newTestClient(t *testing.T) *httpclient.Client {
	t.Helper()
	c, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestJggGamePlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jgg.php" || r.FormValue("action") != "spin" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"new_balance":2333.5,"message":"恭喜获得 100 火花","prize_name":"五等奖","prize_value":100}`))
	}))
	defer srv.Close()

	res, err := JggGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 2333.5 || res.Amount != 100 || res.Prize != "五等奖" {
		t.Errorf("jgg result = %+v", res)
	}
}

func TestMagicScratchGamePlay(t *testing.T) {
	settleCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("action") {
		case "buy":
			// 站点 buy 返回 HTML（内嵌 currentScratchId），非 JSON。
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><script>var currentScratchId = 77;</script></html>`))
		case "scratch":
			if r.FormValue("scratch_id") != "77" {
				w.WriteHeader(400)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"prize":3000,"prize_level":"三等奖","msg":"三等奖 3000 火花"}`))
		case "settle":
			settleCalled = true
			w.WriteHeader(200)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	res, err := MagicScratchGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Amount != 3000 || res.Prize != "三等奖" {
		t.Errorf("scratch result = %+v", res)
	}
	if !settleCalled {
		t.Error("中奖后未调用 settle 结算")
	}
}

// 真实信封：奖品在嵌套 prize 对象 {id,name,...,value}——PrizeID 供九宫格动画定位。
func TestJggGamePlayNested(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"new_balance":2333.5,"message":"恭喜获得 1GB 上传量","prize":{"id":3,"name":"1GB上传量","icon":"💾","color":"#9C27B0","type":"upload","value":1}}`))
	}))
	defer srv.Close()
	res, err := JggGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.PrizeID != 3 || res.Prize != "1GB上传量" || res.Balance != 2333.5 {
		t.Errorf("nested jgg result = %+v", res)
	}
}

func TestJggFailMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"message":"火花不足"}`))
	}))
	defer srv.Close()
	res, _ := JggGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: ""})
	if res.Success || res.Message != "火花不足" {
		t.Errorf("fail result = %+v", res)
	}
}

// 连抽：POST jgg/magic_grid_bulk.php bulk_count=N，results 聚合进 Prizes，value 按类型换算（字节→GB）。
func TestJggGamePlayMulti(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jgg/magic_grid_bulk.php" || r.FormValue("bulk_count") != "10" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[{"prize":{"id":3,"name":"1GB上传量","type":"upload","value":10737418240},"message":"获得 1GB上传量"},{"prize":{"id":8,"name":"谢谢参与","type":"nothing","value":0},"message":"很遗憾，没有中奖"}],"prize_stats":{"1GB上传量":1,"谢谢参与":1},"total_cost":30000,"new_balance":100000}`))
	}))
	defer srv.Close()

	res, err := JggGame{}.PlayMulti(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || len(res.Prizes) != 2 || res.Balance != 100000 {
		t.Fatalf("jgg bulk result = %+v", res)
	}
	win := res.Prizes[0]
	if win.PrizeName != "1GB上传量" || win.Amount != 10 || win.UnitName != "GB" || !win.IsWin {
		t.Errorf("first outcome = %+v", win)
	}
	if res.Prizes[1].IsWin {
		t.Errorf("nothing outcome should not be win: %+v", res.Prizes[1])
	}
}

// 连抽次数仅 10/20/50/100，非法值直接拒绝（不发请求）。
func TestJggGamePlayMultiInvalidCount(t *testing.T) {
	res, err := JggGame{}.PlayMulti(context.Background(), newTestClient(t), "http://example.com", Creds{Cookie: "c=1"}, 7)
	if err != nil || res.Success || res.Message == "" {
		t.Errorf("invalid count should fail inline, got err=%v res=%+v", err, res)
	}
}

// 历史：POST jgg.php action=get_history，prize_value 为 PHP serialize（i:10737418240; → 10GB）。
func TestJggGameHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/jgg.php" || r.FormValue("action") != "get_history" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"history":[{"prize_name":"1GB上传量","prize_type":"upload","prize_value":"i:10737418240;","cost":"3000","created_at":"2026-06-30 12:00:00"},{"prize_name":"谢谢参与","prize_type":"nothing","prize_value":"i:0;","cost":"3000","created_at":"2026-06-30 12:01:00"}]}`))
	}))
	defer srv.Close()

	records, err := JggGame{}.History(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("history len = %d", len(records))
	}
	if records[0].PrizeName != "1GB上传量" || records[0].Amount != 10 || records[0].Cost != 3000 {
		t.Errorf("first record = %+v", records[0])
	}
}

// 回归：游戏请求必须带 AJAX 来源头（X-Requested-With + Referer），否则 NexusPHP
// checkAjaxReferer() 返回 HTML/「安全验证失败」（线上魔力刮刮乐/勋章合成因此挂）。
func TestGameSendsAjaxHeaders(t *testing.T) {
	var gotXrw, gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotXrw = r.Header.Get("X-Requested-With")
		gotReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"new_balance":1}`))
	}))
	defer srv.Close()

	if _, err := (JggGame{}).Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}); err != nil {
		t.Fatal(err)
	}
	if gotXrw != "XMLHttpRequest" {
		t.Errorf("X-Requested-With = %q, want XMLHttpRequest", gotXrw)
	}
	if gotReferer != srv.URL {
		t.Errorf("Referer = %q, want base URL %q", gotReferer, srv.URL)
	}
}

func TestRegistryByHost(t *testing.T) {
	r := NewRegistry()
	r.Register("https://hxpt.org:443/", JggGame{})
	r.Register("hxpt.org", MagicScratchGame{})

	games := r.Games("https://hxpt.org/torrents.php")
	if len(games) != 2 {
		t.Fatalf("expected 2 games for hxpt.org, got %d", len(games))
	}
	if g, ok := r.Game("https://hxpt.org", "jgg"); !ok || g.ID() != "jgg" {
		t.Error("jgg not found")
	}
	if _, ok := r.Game("other.pt", "jgg"); ok {
		t.Error("other.pt should have no games")
	}
}
