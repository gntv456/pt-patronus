package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 批1：纯抽奖类 6 Game 解析测试（httptest mock 各端点响应）。

func TestCbgWheelPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","new_magic_point":100,"prize_ids":[1,2],"extra_message":"魔力500"}`))
	}))
	defer srv.Close()
	res, err := CbgWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 100 || len(res.Prizes) != 2 || res.Prize != "魔力500" {
		t.Errorf("cbg_wheel = %+v", res)
	}
}

func TestCbgWheelPlayMulti(t *testing.T) {
	var gotCount string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCount = r.FormValue("count") // JSON body 非 form；Body 兜底
		if gotCount == "" {
			gotCount = strings.TrimSpace(string(readBody(r)))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","new_magic_point":100,"prize_ids":[1,2,3]}`))
	}))
	defer srv.Close()
	res, err := CbgWheelGame{}.PlayMulti(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || len(res.Prizes) != 3 {
		t.Errorf("cbg_wheel multi = %+v", res)
	}
	if !strings.Contains(gotCount, "10") {
		t.Errorf("count body = %q, want 10", gotCount)
	}
}

func TestCbgSlotPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","results_index":[1,2,3]}`))
	}))
	defer srv.Close()
	res, err := CbgSlotGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Prize, "1 | 2 | 3") {
		t.Errorf("cbg_slot = %+v", res)
	}
}

func TestUbWheelPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.RawQuery, "lottery_json") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rid":3,"rname":"魔力500"}`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><meta name="csrf-token" content="abc"><div>U币余额: 8888.0</div></html>`))
	}))
	defer srv.Close()
	res, err := UbWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.PrizeID != 3 || res.Balance != 8888 || res.Prize != "魔力500" {
		t.Errorf("ub_wheel = %+v", res)
	}
}

func TestLuckyDrawPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"msg":"","data":{"angle":120,"prize_text":"VIP7天"}}`))
	}))
	defer srv.Close()
	res, err := LuckyDrawWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Prize != "VIP7天" {
		t.Errorf("luckydraw = %+v", res)
	}
}

func TestMarchWheelPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"results":[{"name":"魔力500"}],"remaining_bonus":200}`))
	}))
	defer srv.Close()
	res, err := MarchWheelGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 200 || res.Prize != "魔力500" {
		t.Errorf("march = %+v", res)
	}
}

func TestTangptLotteryPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"name":"魔力500","amount":500}],"draw_count":1,"user_bonus_after":300}`))
	}))
	defer srv.Close()
	res, err := TangptLotteryGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 300 || res.Prize != "魔力500" {
		t.Errorf("tangpt = %+v", res)
	}
}

// readBody 读取请求体（测试辅助，用于断言 JSON body 的 count）。
func readBody(r *http.Request) []byte {
	buf := make([]byte, 1024)
	n, _ := r.Body.Read(buf)
	return buf[:n]
}
