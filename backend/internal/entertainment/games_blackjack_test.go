package entertainment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 批3：Blackjack 状态机 + 红包雨 测试。

func TestAudiencesBlackjack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"mode":"content","html":"<div>玩家 A♠ K♥ 你赢了！</div>","bonus":500,"ok":true}`))
	}))
	defer srv.Close()
	res, err := AudiencesBlackjackGame{}.Blackjack(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"}, "hit")
	if err != nil {
		t.Fatal(err)
	}
	if res.Balance != 500 || res.Result != "win" || res.Stage != "finished" {
		t.Errorf("blackjack = %+v", res)
	}
	if len(res.PlayerCards) == 0 {
		t.Errorf("player cards empty: %v", res.PlayerCards)
	}
}

func TestHxptRedpacketPlay(t *testing.T) {
	clicks := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "redpacket_api") {
			_, _ = w.Write([]byte(`{"success":true,"message":"触发成功","click_limit":2}`))
			return
		}
		clicks++
		_, _ = w.Write([]byte(`{"success":true,"reward_type":"seedbonus","reward_value":100}`))
	}))
	defer srv.Close()
	res, err := HxptRedpacketGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success || clicks != 2 || res.Amount != 200 {
		t.Errorf("redpacket = %+v, clicks=%d", res, clicks)
	}
	if !strings.Contains(res.Prize, "2") {
		t.Errorf("redpacket prize = %q", res.Prize)
	}
}

func TestHxptRedpacketTriggerFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":false,"message":"今日已触发5次"}`))
	}))
	defer srv.Close()
	res, err := HxptRedpacketGame{}.Play(context.Background(), newTestClient(t), srv.URL, Creds{Cookie: "c=1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Success || !strings.Contains(res.Message, "5") {
		t.Errorf("redpacket trigger fail = %+v", res)
	}
}
