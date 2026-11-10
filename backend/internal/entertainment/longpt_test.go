// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLongPTWheelPlay 单抽：luckyDraw?size=1 → prizeId 驱动动画 + 首项入 Prizes。
func TestLongPTWheelPlay(t *testing.T) {
	var gotSize string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "lucky/luckyDraw") {
			gotSize = r.URL.Query().Get("size")
			_, _ = w.Write([]byte(`{"code":0,"msg":"","data":[{"prizeId":3,"prizeName":"上传量","prizeType":"03","prizeValue":50,"unitName":"GB"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, err := LongPTWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.PrizeID != 3 || res.Prize != "上传量" || res.Amount != 50 {
		t.Errorf("wheel play = %+v", res)
	}
	if len(res.Prizes) != 1 || !res.Prizes[0].IsWin {
		t.Errorf("prizes = %+v", res.Prizes)
	}
	if gotSize != "1" {
		t.Errorf("size = %q, want 1", gotSize)
	}
}

// TestLongPTWheelPlayMulti 连抽：全部入 Prizes，Amount 为中奖金和，PrizeID 取首个中奖。
func TestLongPTWheelPlayMulti(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "lucky/luckyDraw") {
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"prizeId":3,"prizeName":"上传量","prizeType":"03","prizeValue":50,"unitName":"GB"},
				{"prizeId":1,"prizeName":"魔力","prizeType":"01","prizeValue":100,"unitName":"点"},
				{"prizeId":9,"prizeName":"未中奖","prizeType":"99","prizeValue":0}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, err := LongPTWheelGame{}.PlayMulti(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Prizes) != 3 {
		t.Fatalf("prizes len = %d, want 3", len(res.Prizes))
	}
	if res.Amount != 150 { // 50 + 100（未中奖不计）
		t.Errorf("amount = %v, want 150", res.Amount)
	}
	if res.PrizeID != 3 { // 首个中奖项
		t.Errorf("prizeId = %d, want 3", res.PrizeID)
	}
	if res.Prizes[2].IsWin { // prizeType 99
		t.Error("3rd prize should not be win")
	}
}

// TestLongPTWheelStats 余额 / 单次消耗 / 折扣。
func TestLongPTWheelStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "getLotteryBasicInfo") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"seedBonus":14535,"costMagic":2000,"discount":75}}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	st, err := LongPTWheelGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 14535 || st.CostPerPlay != 2000 || st.Discount != 75 {
		t.Errorf("stats = %+v", st)
	}
}

// TestLongPTWheelPrizes 扇区配置：name+value+unit 拼接，value=0 不拼。
func TestLongPTWheelPrizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "luckyPrizeConfig/list") {
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"id":1,"name":"魔力","value":2000,"unitName":"点"},
				{"id":9,"name":"未中奖","value":0,"unitName":""}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	ps, err := LongPTWheelGame{}.Prizes(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("prizes len = %d", len(ps))
	}
	if ps[0].Name != "魔力2000点" {
		t.Errorf("name[0] = %q, want 魔力2000点", ps[0].Name)
	}
	if ps[1].Name != "未中奖" {
		t.Errorf("name[1] = %q, want 未中奖", ps[1].Name)
	}
}

// TestLongPTWheelHistory 抽奖记录解析。
func TestLongPTWheelHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"records":[
			{"prizeName":"魔力","prizeType":"01","prizeValue":2000,"createTime":"2025-08-12 14:00:00"}],"total":1}}`))
	}))
	defer srv.Close()

	rs, err := LongPTWheelGame{}.History(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].PrizeName != "魔力" || rs[0].Amount != 2000 {
		t.Errorf("records = %+v", rs)
	}
}

// TestLongPTDailyPlay 参与（join）成功 + 已参与失败。
func TestLongPTDailyPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/join") {
			_, _ = w.Write([]byte(`{"code":0,"msg":"恭喜！成功参与抽奖"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, err := LongPTDailyGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Message != "恭喜！成功参与抽奖" {
		t.Errorf("daily play = %+v", res)
	}
}

// TestLongPTDailyAlreadyJoined 已参与（code!=0）→ 失败 + msg。
func TestLongPTDailyAlreadyJoined(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/join") {
			_, _ = w.Write([]byte(`{"code":1,"msg":"今天已参与过抽奖"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	res, _ := LongPTDailyGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if res.Success {
		t.Error("expected fail when already joined")
	}
	if !strings.Contains(res.Message, "已参与") {
		t.Errorf("msg = %q", res.Message)
	}
}

// TestLongPTDailyStats 参与人数（getCountJoin）；时间窗口为前端硬编码默认。
func TestLongPTDailyStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "getCountJoin") {
			_, _ = w.Write([]byte(`{"code":0,"data":222}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	st, err := LongPTDailyGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Participants != 222 {
		t.Errorf("participants = %d, want 222", st.Participants)
	}
	if st.OpenTime != "23:30" || st.StartTime != "00:00" || st.EndTime != "23:29" {
		t.Errorf("time window = %+v", st)
	}
}

// TestLongPTDailyPrizes 今日奖品（data 是数组 [{desc,prizeType}]，desc 取逗号前）。
func TestLongPTDailyPrizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "pluginsLuckyConfig/list") {
			_, _ = w.Write([]byte(`{"code":0,"data":[
				{"desc":"333333魔力,1","prizeType":"LUCKY_EVERY_DAY"},
				{"desc":"20GB上传,2","prizeType":"LUCKY_EVERY_DAY"}]}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	ps, err := LongPTDailyGame{}.Prizes(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("prizes len = %d", len(ps))
	}
	if ps[0].Name != "333333魔力" || ps[1].Name != "20GB上传" {
		t.Errorf("names = %q / %q", ps[0].Name, ps[1].Name)
	}
}

// TestLongPTCookieExpired 站点返 HTML 登录页（cookie 失效）→ Play 友好失败，不报 error。
func TestLongPTCookieExpired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>请先登录</body></html>`))
	}))
	defer srv.Close()

	res, err := LongPTWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Error("expected fail on html response")
	}
	if !strings.Contains(res.Message, "cookie") {
		t.Errorf("msg = %q", res.Message)
	}
}
