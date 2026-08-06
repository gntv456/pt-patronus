package entertainment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// LongPT（longpt.org）娱乐插件系统：幸运大转盘 + 每日抽奖。
//
// 契约来自站点 SPA bundle（页面 /plugins/lucky）——后端走 /pt-api/v1/lucky/*，
// **纯 Cookie 会话鉴权**（无 Bearer / 无应用层 CSRF），统一信封 {code,msg,data}，code==0 为成功。
// LongPT 是 NexusPHP 站，登录 cookie 由 httpclient.seedCookies 自动注入（同好学/财神链路）。
// 参考：cspt.go（JSON API + Do 调用模式）、hxpt.go（Prizeable/Historable 契约）。

// longptAPIPrefix 是 /pt-api/v1 前缀。
const longptAPIPrefix = "pt-api/v1"

// longptEnvelope LongPT 统一响应信封。data 类型随端点变化（对象 / 数组 / 数字），故用 RawMessage 延迟解析。
type longptEnvelope struct {
	Code int             `json:"code"` // 0 = 成功
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// longptURL 拼 {base}/pt-api/v1/{path}。
func longptURL(base, path string) string {
	return joinPath(joinPath(base, longptAPIPrefix), path)
}

// longptGet 发 GET（参数放 query），返回 (data, code, msg)。
func longptGet(ctx context.Context, hc *httpclient.Client, base, path, query, cookie string) (json.RawMessage, int, string, error) {
	url := longptURL(base, path)
	if query != "" {
		url += "?" + query
	}
	resp, err := hc.Do(ctx, http.MethodGet, url, nil, longptHeaders(base), cookie, true)
	if err != nil {
		return nil, 0, "", err
	}
	logResp("longpt "+pathTail(path), url, resp)
	return longptUnwrap(resp.Body)
}

// longptPost 发 POST（JSON body），返回 (data, code, msg)。
func longptPost(ctx context.Context, hc *httpclient.Client, base, path string, body any, cookie string) (json.RawMessage, int, string, error) {
	url := longptURL(base, path)
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	headers := longptHeaders(base)
	headers["Content-Type"] = "application/json"
	resp, err := hc.Do(ctx, http.MethodPost, url, payload, headers, cookie, true)
	if err != nil {
		return nil, 0, "", err
	}
	logResp("longpt "+pathTail(path), url, resp)
	return longptUnwrap(resp.Body)
}

// longptUnwrap 解析信封，返回 (data, code, msg)。站点返回 HTML 登录页（cookie 失效）时解析失败 → 透传片段。
func longptUnwrap(body []byte) (json.RawMessage, int, string, error) {
	var env longptEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		snippet := string(body)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, -1, "", fmt.Errorf("parse longpt response: %s", snippet)
	}
	return env.Data, env.Code, env.Msg, nil
}

// longptHeaders LongPT API 公共头：同源浏览器请求自动带 Origin/Referer，部分端点
// （/plugins/medals/ 的 POST）校验来源防 CSRF；Go 端须显式补齐，否则返回 HTML 登录页而非 JSON。
// 对齐 cspt.csptAPIHeaders（Origin+Referer）与 hxpt PostFormAjax（X-Requested-With+Referer）。
func longptHeaders(baseURL string) map[string]string {
	origin := baseURL
	if i := strings.Index(origin, "://"); i >= 0 {
		rest := origin[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			rest = rest[:j]
		}
		origin = origin[:i+3] + rest
	}
	return map[string]string{
		"Accept":           "application/json, text/plain, */*",
		"Origin":           origin,
		"Referer":          baseURL,
		"X-Requested-With": "XMLHttpRequest",
	}
}

// longptAsList 把 data 转成 []json.RawMessage：兼容数组，或对象取 list/records/data/medals 字段。
// 逐项再解析以保 json.Number 精度（medalId 是大整数卦象）。
func longptAsList(data json.RawMessage) ([]json.RawMessage, error) {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err == nil {
		for _, k := range []string{"list", "records", "data", "medals"} {
			if l, ok := obj[k]; ok {
				if err := json.Unmarshal(l, &arr); err == nil {
					return arr, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("response is not a list")
}

// longptNum 把 any（float64/int/string）转 float64。LongPT 的 prizeId/prizeValue 类型不固定，统一兜底。
func longptNum(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		var f float64
		_, _ = fmt.Sscanf(x, "%f", &f)
		return f
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return 0
}

func longptInt(v any) int { return int(longptNum(v)) }

// longptValStr 数值转紧凑字符串（2000 → "2000"，去 %g 多余 0）。扇区标签拼接用。
func longptValStr(v any) string {
	f := longptNum(v)
	if f == 0 {
		return ""
	}
	return fmt.Sprintf("%g", f)
}

// ── 幸运大转盘 ────────────────────────────────────────────────

// LongPTWheelGame 幸运大转盘：GET luckyDraw?size=N。响应是奖品数组；
// 单抽取首项（prizeId 驱动前端扇区动画），连抽聚合全部入 Prizes。
type LongPTWheelGame struct{}

func (LongPTWheelGame) ID() string   { return "longpt_wheel" }
func (LongPTWheelGame) Name() string { return "幸运大转盘" }

// longptDrawItem luckyDraw 返回的单个奖品（camelCase）。
type longptDrawItem struct {
	PrizeID    any    `json:"prizeId"`
	PrizeName  string `json:"prizeName"`
	PrizeType  string `json:"prizeType"` // 类型码：01魔力 02VIP 03上传 … 99未中奖
	PrizeValue any    `json:"prizeValue"`
	UnitName   string `json:"unitName"`
}

// Play 单抽（size=1）。
func (g LongPTWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	return g.draw(ctx, hc, baseURL, cr, 1)
}

// PlayMulti 连抽（size=count）。
func (g LongPTWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	return g.draw(ctx, hc, baseURL, cr, count)
}

func (g LongPTWheelGame) draw(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, size int) (*PlayResult, error) {
	data, code, msg, err := longptGet(ctx, hc, baseURL, "lucky/luckyDraw", fmt.Sprintf("size=%d", size), cr.Cookie)
	if err != nil {
		// 站点返 HTML 登录页等非 JSON → 视为失败而非 error，让前端展示友好提示。
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	if code != 0 {
		return &PlayResult{Success: false, Message: msg}, nil
	}
	var items []longptDrawItem
	if err := json.Unmarshal(data, &items); err != nil {
		return &PlayResult{Success: false, Message: "解析抽奖结果失败"}, nil
	}
	return aggregateLongPTPrizes(items), nil
}

// aggregateLongPTPrizes 把 luckyDraw 奖品数组聚合成 PlayResult。
// 单抽/连抽通用：全部入 Prizes；Prize/PrizeID 取首个中奖项（无中奖取首项）；Amount 取中奖数值和。
func aggregateLongPTPrizes(items []longptDrawItem) *PlayResult {
	out := &PlayResult{Success: true, Prizes: make([]PrizeOutcome, 0, len(items))}
	var sum float64
	for _, it := range items {
		o := longptOutcome(it)
		out.Prizes = append(out.Prizes, o)
		if o.IsWin {
			sum += o.Amount
		}
	}
	for _, o := range out.Prizes {
		if o.IsWin {
			out.PrizeID, out.Prize = o.PrizeID, o.PrizeName
			break
		}
	}
	if out.Prize == "" && len(out.Prizes) > 0 {
		out.PrizeID, out.Prize = out.Prizes[0].PrizeID, out.Prizes[0].PrizeName
	}
	out.Amount = sum
	if out.Prize == "" {
		out.Prize, out.Message = "未中奖", "很遗憾，未中奖"
	}
	return out
}

func longptOutcome(it longptDrawItem) PrizeOutcome {
	return PrizeOutcome{
		PrizeID:   longptInt(it.PrizeID),
		PrizeName: it.PrizeName,
		PrizeType: it.PrizeType,
		Amount:    longptNum(it.PrizeValue),
		UnitName:  it.UnitName,
		IsWin:     it.PrizeType != "" && it.PrizeType != "99",
	}
}

// Stats 余额 / 单次消耗 / 连抽折扣（getLotteryBasicInfo）。
func (LongPTWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	data, code, msg, err := longptGet(ctx, hc, baseURL, "lucky/getLotteryBasicInfo", "", cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	return &StatsResult{
		Balance:     numField(m, "seedBonus"),
		CostPerPlay: numField(m, "costMagic"),
		Discount:    int(numField(m, "discount")),
	}, nil
}

// Prizes 转盘扇区奖品配置（luckyPrizeConfig/list）。扇区 index = 数组下标，前端动画据此定位。
func (LongPTWheelGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	data, code, msg, err := longptGet(ctx, hc, baseURL, "lucky/luckyPrizeConfig/list", "", cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	var items []struct {
		ID       any    `json:"id"`
		Name     string `json:"name"`
		Value    any    `json:"value"`
		UnitName string `json:"unitName"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse prize config: %w", err)
	}
	out := make([]PrizeDef, 0, len(items))
	for _, it := range items {
		name := it.Name
		if v := longptValStr(it.Value); v != "" {
			name += v + it.UnitName
		}
		out = append(out, PrizeDef{ID: longptInt(it.ID), Name: name})
	}
	return out, nil
}

// History 大转盘抽奖记录（luckyDrawRecord/getRecordsByPage）。记录字段以后端实测为准。
func (LongPTWheelGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	data, code, msg, err := longptPost(ctx, hc, baseURL, "lucky/luckyDrawRecord/getRecordsByPage",
		map[string]any{"pageNum": 1, "pageSize": 20}, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%s", msg)
	}
	return parseLongPTRecords(data)
}

