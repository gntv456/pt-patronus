package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 快三（蟹黄堡）：状态解析 + 投注参数校验（httptest 本地服务，不触真站）。
func TestCrabFastThreePlayAndBetValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "message": "查询成功",
			"data": map[string]any{
				"history": []any{}, "next": []any{"2026-09-01 21:15:00"},
				"amount": map[string]any{"totalBetAmount": nil, "totalWinningAmount": 120.5},
			},
		})
	}))
	defer srv.Close()
	hc := newTestClient(t)
	g := CrabFastThreeGame{}
	res, err := g.Play(context.Background(), hc, srv.URL, Creds{})
	if err != nil || res == nil || !res.Success {
		t.Fatalf("play failed: %v %+v", err, res)
	}
	// nil 投注显示 -、中奖金额原样（不得变科学计数法）
	if !stringContains(res.Message, "120.5") || !stringContains(res.Message, "-") {
		t.Fatalf("message formatting wrong: %q", res.Message)
	}

	// 角色校验：非法角色拒绝，不真发网（在合法 URL 上 Selection 非法）
	bad, _ := g.Bet(context.Background(), hc, srv.URL, Creds{}, BetRequest{Selection: "超梦"})
	if bad == nil || bad.Success {
		t.Fatalf("invalid character should fail, got %+v", bad)
	}
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
