// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package entertainment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── LuckyDraw 插件（PANDA pandapt.net + 蟹黄堡 crabpt.vip 共用） ────────────
// POST /plugin/lucky-draw {}（单）/ /plugin/lucky-draw-bulk {"option":N}（连）→ {ret,msg,data:{angle,prize_text}}。
type LuckyDrawWheelGame struct{}

func (LuckyDrawWheelGame) ID() string   { return "luckydraw_wheel" }
func (LuckyDrawWheelGame) Name() string { return "幸运大转盘" }

func luckyDrawPost(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string, body any) (map[string]any, error) {
	u := joinPath(baseURL, path)
	payload, _ := json.Marshal(body)
	resp, err := hc.Do(ctx, http.MethodPost, u, payload, map[string]string{
		"Content-Type":     "application/json",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "luckywheel.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("luckydraw "+path, u, resp)
	return parseRaw(resp.Body)
}

func luckyDrawResult(m map[string]any) *PlayResult {
	ret := int(longptNum(m["ret"]))
	res := &PlayResult{Success: ret == 0, Message: strField(m, "msg"), Raw: m}
	if data, ok := m["data"].(map[string]any); ok {
		res.Prize = strField(data, "prize_text")
	}
	if res.Success && res.Prize == "" {
		res.Prize = "中奖"
	}
	if !res.Success && res.Message == "" {
		res.Message = "抽奖失败"
	}
	return res
}

func (LuckyDrawWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := luckyDrawPost(ctx, hc, baseURL, "plugin/lucky-draw", cr.Cookie, map[string]any{})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return luckyDrawResult(m), nil
}

func (LuckyDrawWheelGame) PlayMulti(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds, count int) (*PlayResult, error) {
	if count < 1 {
		count = 1
	}
	if count > maxMultiCount {
		count = maxMultiCount
	}
	m, err := luckyDrawPost(ctx, hc, baseURL, "plugin/lucky-draw-bulk", cr.Cookie, map[string]any{"option": count})
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	return luckyDrawResult(m), nil
}

// Prizes 抠转盘页内嵌的奖品 JSON（{"id","typeText","amountText"}）作为转盘扇区。
// 供通用 SpinnerGame 画真转盘用；解析失败返回空（前端降级为装饰扇区）。
// 包子（t-baozi.cc）等站把内嵌 JSON 双重转义（" 写作 \u0022），先解一层再匹配；
// 奖品名本身也可能是 \u9b54\u529b 转义形式，配对后统一 unescape。
func (LuckyDrawWheelGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	body, err := luckyDrawWheelPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte(`"typeText":"`)) && bytes.Contains(body, []byte(`typeText`)) {
		body = bytes.ReplaceAll(body, []byte(`\u0022`), []byte(`"`))
	}
	// 整体匹配每个奖品对象（扁平、无嵌套花括号），再从中抠 id/typeText/amountText，
	// 避免页面里其它 "id": 干扰配对。
	objRe := compileRegex(`\{[^{}]*"typeText":"((?:[^"\\]|\\.)*)"[^{}]*"amountText":"((?:[^"\\]|\\.)*)"[^{}]*\}`)
	idRe := compileRegex(`"id":(\d+)`)
	matches := objRe.FindAllSubmatch(body, -1)
	out := make([]PrizeDef, 0, len(matches))
	for i, m := range matches {
		typeText := unescapeJSONString(string(m[1]))
		amountText := strings.TrimSpace(unescapeJSONString(string(m[2])))
		name := strings.TrimSpace(typeText + " " + amountText)
		pd := PrizeDef{ID: i + 1, Name: name, Type: typeText}
		if idm := idRe.FindSubmatch(m[0]); idm != nil {
			pd.ID = int(longptNum(string(idm[1])))
		}
		out = append(out, pd)
	}
	return out, nil
}

// unescapeJSONString 还原 JSON 字符串字面量里的 \uXXXX 等转义（奖品名常为 \u9b54\u529b 形式；
// 双重转义站点的 \\\\u9b54 需调两次）。
func unescapeJSONString(s string) string {
	for range 2 {
		if !strings.ContainsRune(s, '\\') {
			return s
		}
		var out string
		if err := json.Unmarshal([]byte(`"`+s+`"`), &out); err != nil {
			return s
		} else if out == s {
			return s
		} else {
			s = out
		}
	}
	return s
}

// luckyDrawPagePath 转盘页路径：站点插件升级后从 luckywheel.php 迁到 /plugin/lucky-draw
// （2026-08 实测 PANDA/蟹黄堡 luckywheel.php 均 404，余额/奖品 JSON 都在新页）。
// 取页时先试新路径，404/空回退旧路径（老版本插件站仍用 luckywheel.php）。
const (
	luckyDrawNewPage = "plugin/lucky-draw"
	luckyDrawOldPage = "luckywheel.php"
)

// luckyDrawWheelPage GET 转盘页（取余额/奖品共用）。新路径 200 优先；否则回退旧路径。
func luckyDrawWheelPage(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) ([]byte, error) {
	for _, p := range []string{luckyDrawNewPage, luckyDrawOldPage} {
		page := joinPath(baseURL, p)
		resp, err := hc.GetWithHeaders(ctx, page, cookie, map[string]string{"Referer": page}, true)
		if err != nil {
			continue
		}
		// 有效转盘页判定：200 且含转盘特征（奖品 JSON typeText / 余额行）；短页面（测试桩/
		// 精简皮肤）也认，避免误回退。404 页（站点无此路径）特征都不含 → 落到下一候选。
		if resp.StatusCode == 200 && (bytes.Contains(resp.Body, []byte("typeText")) ||
			bytes.Contains(resp.Body, []byte("当前用户拥有")) || len(resp.Body) > 1000) {
			return resp.Body, nil
		}
	}
	return nil, fmt.Errorf("luckydraw page unreachable")
}

// Stats 余额：抠转盘页「当前用户拥有<货币>：N」（PANDA=魔力、蟹黄堡=蟹币——货币名随站不同，
// 用「当前用户拥有」前缀区分奖品金额；消耗同理「每次抽奖需要<货币>：N」）。
func (LuckyDrawWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	body, err := luckyDrawWheelPage(ctx, hc, baseURL, cr.Cookie)
	if err != nil {
		return &StatsResult{}, nil
	}
	st := &StatsResult{Balance: novaHDNum(firstGroup(body, `当前用户拥有[^：:\d]{0,6}[：:]\s*([\d,.]+)`))}
	st.CostPerPlay = novaHDNum(firstGroup(body, `每次抽奖需要[^：:\d]{0,6}[：:]\s*([\d,.]+)`))
	return st, nil
}

// History 抽奖记录：GET /plugin/lucky-draw/winning-records（layui data-table）→ {data:[{created_at,result}]}。
// 字段防御：result/prize/prize_text + created_at/time/created_time。
func (LuckyDrawWheelGame) History(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]GameRecord, error) {
	u := joinPath(baseURL, "plugin/lucky-draw/winning-records")
	resp, err := hc.Do(ctx, http.MethodGet, u, nil, map[string]string{
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "luckywheel.php"),
	}, cr.Cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("luckydraw records", u, resp)
	m, err := parseRaw(resp.Body)
	if err != nil {
		return []GameRecord{}, nil
	}
	items := recordItems(m)
	out := make([]GameRecord, 0, len(items))
	for _, it := range items {
		name := firstNonEmpty(strField(it, "result"), strField(it, "prize"), strField(it, "prize_text"))
		ts := firstNonEmpty(strField(it, "created_at"), strField(it, "time"), strField(it, "created_time"))
		if name == "" && ts == "" {
			continue
		}
		out = append(out, GameRecord{PrizeName: name, Amount: numField(it, "amount"), CreatedAt: ts})
	}
	return out, nil
}

// recordItems 从 layui/laravel data-table 响应里取记录数组（兼容 data:[...] 与 data:{data:[...]}）。
func recordItems(m map[string]any) []map[string]any {
	if arr, ok := m["data"].([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			if em, ok := e.(map[string]any); ok {
				out = append(out, em)
			}
		}
		return out
	}
	if d, ok := m["data"].(map[string]any); ok {
		if arr, ok := d["data"].([]any); ok {
			out := make([]map[string]any, 0, len(arr))
			for _, e := range arr {
				if em, ok := e.(map[string]any); ok {
					out = append(out, em)
				}
			}
			return out
		}
	}
	return nil
}
