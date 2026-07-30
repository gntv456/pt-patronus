package entertainment

import (
	"context"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── CBG 骰宝竞技场（cangbao.ge） ──────────────────────────
// POST api/api_sicbo.php {amount,type} → {status,new_magic_point,message}（复用 cbgApiPost）。
type CbgSicboGame struct{}

func (CbgSicboGame) ID() string   { return "cbg_sicbo" }
func (CbgSicboGame) Name() string { return "骰宝竞技场" }

func (CbgSicboGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	sel := req.Selection
	if sel == "" {
		sel = "big"
	}
	amt := req.Amount
	if amt <= 0 {
		amt = 100
	}
	if amt > maxBetCeiling {
		amt = maxBetCeiling
	}
	m, err := cbgApiPost(ctx, hc, baseURL, "api/api_sicbo.php", "sicboarena.php", cr.Cookie, map[string]any{"amount": amt, "type": sel})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return &PlayResult{
		Success: strField(m, "status") == "success",
		Balance: numField(m, "new_magic_point"),
		Prize:   "押 " + sel + " " + strconv.FormatFloat(amt, 'f', -1, 64),
		Message: strField(m, "message"),
		Raw:     m,
	}, nil
}

func (CbgSicboGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return CbgSicboGame{}.Bet(ctx, hc, baseURL, cr, BetRequest{Selection: "big", Amount: 100})
}

func (CbgSicboGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "sicboarena.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	return &StatsResult{Balance: cbgBalance(resp.Body)}, nil
}
