package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 蟹黄堡 双色球（crabpt.vip） ───────────────────────────
// POST /union-lotto {nums:[6红+1蓝]} → {code,m,data}。extra.reds="1,2,3,4,5,6" / extra.blue="7"。
type CrabUnionLottoGame struct{}

func (CrabUnionLottoGame) ID() string   { return "crab_unionlotto" }
func (CrabUnionLottoGame) Name() string { return "双色球" }

func (CrabUnionLottoGame) Bet(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, req BetRequest) (*PlayResult, error) {
	nums := make([]string, 0, 7)
	for _, r := range strings.Split(req.Extra["reds"], ",") {
		if r = strings.TrimSpace(r); r != "" {
			nums = append(nums, r)
		}
	}
	if b := strings.TrimSpace(req.Extra["blue"]); b != "" {
		nums = append(nums, b)
	}
	if len(nums) < 7 {
		return &PlayResult{Success: false, Message: "请选 6 红 1 蓝（extra.reds=逗号分隔 / extra.blue）"}, nil
	}
	u := joinPath(baseURL, "union-lotto")
	payload, _ := json.Marshal(map[string]any{"nums": nums})
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "union-lotto.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("crab_unionlotto", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return &PlayResult{Success: false, Message: "解析投注结果失败"}, nil
	}
	return &PlayResult{
		Success: int(longptNum(m["code"])) == 0,
		Message: strField(m, "m"),
		Prize:   strings.Join(nums, ","),
		Raw:     m,
	}, nil
}

func (CrabUnionLottoGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return &PlayResult{Success: false, Message: "双色球需选号：请用 Bet（extra.reds=6红 / extra.blue=1蓝）"}, nil
}
