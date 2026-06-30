package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 批2：Bettable 下注/选号类测试。

func TestCbgSicboBet(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","new_magic_point":500,"message":"赢"}`))
	}))
	defer srv.Close()
	res, err := CbgSicboGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "big", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 500 || !strings.Contains(res.Prize, "big") {
		t.Errorf("cbg_sicbo = %+v", res)
	}
	if !strings.Contains(gotBody, `"type":"big"`) || !strings.Contains(gotBody, `"amount":100`) {
		t.Errorf("bet body = %q", gotBody)
	}
}

func TestCrabUnionLottoBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"m":"","data":{}}`))
	}))
	defer srv.Close()
	res, err := CrabUnionLottoGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{
		Extra: map[string]string{"reds": "1,2,3,4,5,6", "blue": "7"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Prize, "7") {
		t.Errorf("crab_unionlotto = %+v", res)
	}
	// 号码不足应失败
	bad, _ := CrabUnionLottoGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{}, BetRequest{})
	if bad.Success {
		t.Error("unionlotto should fail without nums")
	}
}

func TestPandaFlipBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ret":0,"data":{"balance":100,"flipResult":{"tier":"一等奖","reward":"VIP7天"}}}`))
	}))
	defer srv.Close()
	res, err := PandaFlipGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 100 || !strings.Contains(res.Prize, "一等奖") {
		t.Errorf("panda_flip = %+v", res)
	}
}

func TestHxptBigSmallBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "bigsmall.php") && !strings.Contains(r.URL.RawQuery, "action") {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html data-csrf-token="tok123"></html>`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.RawQuery, "action=game_status") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"game":{"id":42,"status":"betting"},"round_token":"rtok","balance":1000,"min_bet":100}}`))
			return
		}
		if strings.Contains(r.URL.RawQuery, "action=bet") {
			_, _ = w.Write([]byte(`{"code":0,"data":{"balance":900,"bet_type":"big"}}`))
		}
	}))
	defer srv.Close()
	res, err := HxptBigSmallGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "big", Amount: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 900 || !strings.Contains(res.Prize, "big") {
		t.Errorf("hxpt_bigsmall = %+v", res)
	}
}

func TestOurbitsBetPlaceholder(t *testing.T) {
	// 押注表单待补抓，Bet 返回失败占位（不请求站点）
	res, err := OurbitsBetGame{}.Play(context.Background(), newTestClient(t), "http://x", Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		t.Error("ourbits_bet should be placeholder (not success)")
	}
}

func TestUbitsRedpacketBet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><div>领取成功</div><span>U币: 8888.0</span></html>`))
	}))
	defer srv.Close()
	res, err := UbitsRedpacketGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, BetRequest{Selection: "claim"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Balance != 8888 {
		t.Errorf("ubits_redpacket = %+v", res)
	}
}
