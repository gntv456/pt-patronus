package entertainment

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCity13LotteryStatsPlay 只读：余额（啤酒瓶）与开奖公告解析。
func TestCity13LotteryStatsPlay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><title>13City :: 彩票中心 - 每天20:00 开奖</title><body>
<p>剩余啤酒瓶：1,157,620.0</p><p>上期开奖号码 03 17 22 28 31</p></body></html>`)
	}))
	defer srv.Close()

	st, err := City13LotteryGame{}.Stats(context.Background(), newTestClient(t), srv.URL, Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Balance != 1157620 {
		t.Errorf("balance = %v", st.Balance)
	}
	res, err := City13LotteryGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || !strings.Contains(res.Prize, "开奖号码") {
		t.Errorf("play = %+v", res)
	}
}

// TestCity13LotteryBet 购买 5 注：POST action=purchase5 + numbers/bet_amount；success=false 透传 message。
func TestCity13LotteryBet(t *testing.T) {
	var gotForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			gotForm = r.Form.Get("action") + "|" + r.Form.Get("bet_amount")
			_, _ = io.WriteString(w, `{"success":true,"message":"购买成功！已购买5注彩票"}`)
			return
		}
		_, _ = io.WriteString(w, `<html></html>`)
	}))
	defer srv.Close()

	res, err := City13LotteryGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{},
		BetRequest{Selection: "purchase5", Amount: 10, Extra: map[string]string{"numbers": `["01","02","03","04","05"]`}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || res.Prize != "已购 5 注" || gotForm != "purchase5|10" {
		t.Errorf("bet = %+v form=%q", res, gotForm)
	}
}

// TestCity13LotteryBetNoAmount 无金额拒绝（防误触，不发请求）。
func TestCity13LotteryBetNoAmount(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = io.WriteString(w, `<html></html>`)
	}))
	defer srv.Close()
	res, err := City13LotteryGame{}.Bet(context.Background(), newTestClient(t), srv.URL, Creds{}, BetRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Message, "金额") || called {
		t.Errorf("no-amount = %+v called=%v", res, called)
	}
}
