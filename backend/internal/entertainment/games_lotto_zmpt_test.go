package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 织梦双色球：状态解析 + 选号校验 + POST 落点（/nodeapi 前缀）。
func TestZmptLottoStateAndBet(t *testing.T) {
	var gotPath, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 0, "message": "查询本期购买信息成功",
				"data": map[string]any{
					"time": "2026-09-03(周四)21:15:00", "list": []any{},
					"totalAmount": -44558, "UnionLottoInceptionAmount": 1000000,
				},
			})
			return
		}
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		buf := make([]byte, 256)
		n, _ := r.Body.Read(buf)
		gotBody = string(buf[:n])
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "购买成功"})
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := ZmptLottoGame{}

	res, err := g.Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	for _, want := range []string{"-44558", "21:15:00"} {
		if !stringContains(res.Message, want) {
			t.Fatalf("message missing %s: %q", want, res.Message)
		}
	}

	// 非法：红球越界
	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "1,2,3,4,5,34|7"})
	if bad == nil || bad.Success {
		t.Fatalf("red 34 should be rejected")
	}
	// 合法投注 → /nodeapi/union-lotto + json nums
	ok, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "1,2,3,4,5,6|7"})
	if ok == nil || !ok.Success {
		t.Fatalf("valid bet failed: %+v", ok)
	}
	if gotPath != "/nodeapi/union-lotto" || !strings.Contains(gotBody, "nums") || !strings.Contains(gotCT, "json") {
		t.Fatalf("bet posted wrong: path=%s ct=%s body=%s", gotPath, gotCT, gotBody)
	}
}
