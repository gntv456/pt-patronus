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

func TestLuckyDrawPrizes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// 页面内嵌奖品 JSON（含一个无关 "id": 干扰项，验证整体对象匹配不串位）
		_, _ = w.Write([]byte(`<html>var other = {"id":999,"foo":"bar"};
		var prizes = [{"id":3,"name":null,"type":101,"typeText":"彩虹 ID","amountText":"7 Day(s)"},
		{"id":8,"type":1001,"typeText":"魔力","amountText":"200 "}]</html>`))
	}))
	defer srv.Close()
	prizes, err := LuckyDrawWheelGame{}.Prizes(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prizes) != 2 {
		t.Fatalf("prizes = %+v", prizes)
	}
	if prizes[0].ID != 3 || prizes[0].Name != "彩虹 ID 7 Day(s)" {
		t.Errorf("prizes[0] = %+v", prizes[0])
	}
	if prizes[1].ID != 8 || prizes[1].Name != "魔力 200" {
		t.Errorf("prizes[1] = %+v", prizes[1])
	}
}

func TestLuckyDrawStatsHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "winning-records") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":[{"created_at":"2026-07-01 10:00","result":"88 魔力","amount":88},{"created_at":"2026-07-01 09:00","result":"谢谢参与"}]}`))
			return
		}
		// luckywheel.php
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div>当前用户拥有魔力：615108.7</div><div>魔力：2000（每次）</div></html>`))
	}))
	defer srv.Close()
	st, err := LuckyDrawWheelGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 615108.7 {
		t.Errorf("luckydraw stats balance = %v, want 615108.7", st.Balance)
	}
	recs, err := LuckyDrawWheelGame{}.History(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].PrizeName != "88 魔力" || recs[0].Amount != 88 || recs[0].CreatedAt != "2026-07-01 10:00" {
		t.Errorf("luckydraw history = %+v", recs)
	}
}

func TestMarchStatsMulti(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "spin") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"results":[{"name":"88 魔力","amount":88},{"name":"88 魔力"},{"name":"谢谢参与"}],"remaining_bonus":1500}`))
			return
		}
		// luckywheel.php
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><span id="user-bonus-amount">魔力1500</span></html>`))
	}))
	defer srv.Close()
	st, err := MarchWheelGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 1500 {
		t.Errorf("march stats balance = %v, want 1500", st.Balance)
	}
	res, err := MarchWheelGame{}.PlayMulti(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Prizes) != 3 || res.Balance != 1500 {
		t.Errorf("march multi = %+v", res)
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

// TestLuckyDrawPrizesBaozi 包子（t-baozi.cc）形态：内嵌 JSON 双重转义（" → \u0022），
// 且奖品名本身是 \uXXXX 转义（\u9b54\u529b=魔力）。验证先解一层引号再配对、名字最终还原。
func TestLuckyDrawPrizesBaozi(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html>var cfg = {\u0022prizes\u0022:[{\u0022id\u0022:4,\u0022typeText\u0022:\u0022\\u8c22\\u8c22\\u53c2\\u4e0e\u0022,\u0022amountText\u0022:\u0022\u0022},{\u0022id\u0022:7,\u0022typeText\u0022:\u0022\\u9b54\\u529b\u0022,\u0022amountText\u0022:\u00222000\u0022}]}</html>`))
	}))
	defer srv.Close()
	prizes, err := LuckyDrawWheelGame{}.Prizes(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prizes) != 2 {
		t.Fatalf("prizes = %+v", prizes)
	}
	if prizes[0].ID != 4 || prizes[0].Name != "谢谢参与" {
		t.Errorf("prizes[0] = %+v", prizes[0])
	}
	if prizes[1].ID != 7 || prizes[1].Name != "魔力 2000" {
		t.Errorf("prizes[1] = %+v", prizes[1])
	}
}
