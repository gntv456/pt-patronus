package entertainment

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// ── 憨憨 hhanclub.net 幸运大转盘（lucky.php，LuckyDraw 插件同源） ──────────
// POST /plugin/lucky-draw {}（表单空 body）→ {ret,msg,data:{angle,duration,easing,prize_text}}；
// 页面 lucky.php：余额 .bean-number、每次消耗「每次消耗憨豆： 2000」、奖品内嵌 prizes JSON
// （{"type","typeText","amountText","priority"}，unicode 转义由 JSON 反序列化还原）。
type HhanLuckyWheelGame struct{}

func (HhanLuckyWheelGame) ID() string   { return "hhan_lucky_wheel" }
func (HhanLuckyWheelGame) Name() string { return "幸运大转盘" }

func hhanLuckyPost(ctx context.Context, hc *httpclient.Client, baseURL, cookie string) (map[string]any, error) {
	u := joinPath(baseURL, "plugin/lucky-draw")
	resp, err := hc.Do(ctx, http.MethodPost, u, []byte{}, map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          joinPath(baseURL, "lucky.php"),
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	logResp("hhan lucky-draw", u, resp)
	return parseRaw(resp.Body)
}

func (HhanLuckyWheelGame) Play(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*PlayResult, error) {
	m, err := hhanLuckyPost(ctx, hc, baseURL, cr.Cookie)
	if err != nil || m == nil {
		return &PlayResult{Success: false, Message: "站点响应异常，可能未登录或 cookie 已失效"}, nil
	}
	// ret!==0 时站点回 {ret,msg}（余额不足等）；与 LuckyDraw 插件同一响应结构。
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
	return res, nil
}

// hhanBeanNumberRe 抠 lucky.php 余额（.bean-number 文本；页面同时含 bean_icon 图标类名，
// 用 class 边界避免误中其它数字）。
var hhanBeanNumberRe = regexp.MustCompile(`(?is)class="bean-number"[^>]*>\s*([\d,.]+)\s*<`)

// hhanCostRe 每次消耗憨豆数（区分余额与奖品金额：带「每次消耗」前缀唯一）。
var hhanCostRe = regexp.MustCompile(`每次消耗憨豆[：:]\s*([\d,.]+)`)

func (HhanLuckyWheelGame) Stats(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) (*StatsResult, error) {
	page := joinPath(baseURL, "lucky.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return &StatsResult{}, nil
	}
	st := &StatsResult{}
	if m := hhanBeanNumberRe.FindSubmatch(resp.Body); m != nil {
		st.Balance = novaHDNum(string(m[1]))
	}
	if m := hhanCostRe.FindSubmatch(resp.Body); m != nil {
		st.CostPerPlay = novaHDNum(string(m[1]))
	}
	return st, nil
}

// hhanPrizeObjRe 抠 lucky.php 内嵌 prizes 数组的每个奖品对象（扁平无嵌套花括号，
// type/amount 文本为 \uXXXX 转义，先截原文再走 JSON 反序列化还原）。
var hhanPrizeObjRe = regexp.MustCompile(`\{[^{}]*"typeText"[^{}]*\}`)

// Prizes 奖品扇区（通用 SpinnerGame 画真转盘用；解析失败返回空，前端降级装饰扇区）。
func (HhanLuckyWheelGame) Prizes(ctx context.Context, hc *httpclient.Client, baseURL string, cr Creds) ([]PrizeDef, error) {
	page := joinPath(baseURL, "lucky.php")
	resp, err := hc.GetWithHeaders(ctx, page, cr.Cookie, map[string]string{"Referer": page}, true)
	if err != nil {
		return nil, err
	}
	matches := hhanPrizeObjRe.FindAll(resp.Body, -1)
	out := make([]PrizeDef, 0, len(matches))
	for i, raw := range matches {
		// raw 以 '}' 结尾，去掉再补 "id" 字段（直接追加会变成 }}，JSON 尾随垃圾报错）
		trimmed := raw[:len(raw)-1]
		if len(trimmed) > 0 && trimmed[len(trimmed)-1] != '{' {
			trimmed = append([]byte{}, trimmed...)
			trimmed = append(trimmed, []byte(`,"id":`+itoaSimple(i+1)+`}`)...)
		} else {
			trimmed = append(append([]byte{}, trimmed...), []byte(`"id":`+itoaSimple(i+1)+`}`)...)
		}
		m, err := parseRaw(trimmed)
		if err != nil {
			continue
		}
		typeText := strings.TrimSpace(strField(m, "typeText"))
		amountText := strings.TrimSpace(strField(m, "amountText"))
		if typeText == "" {
			continue
		}
		out = append(out, PrizeDef{
			ID:   i + 1,
			Name: strings.TrimSpace(typeText + " " + amountText),
			Type: typeText,
		})
	}
	return out, nil
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
