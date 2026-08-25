package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// checkinKeywords 用于探测签到入口：链接 href / 锚文本 / 表单 action / 按钮文字含其一即视为签到入口。
var checkinKeywords = []string{"签到", "簽到", "attendance", "sign", "checkin", "check-in", "check_in"}

func isCheckinEntry(href, text string) bool {
	hay := strings.ToLower(href + " " + text)
	for _, kw := range checkinKeywords {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

// probeResult 是首页探测结果：真实签到端点、HTTP 方法、表单字段（探测 hidden 与配置 Form 合并）。
type probeResult struct {
	path   string
	method string
	form   map[string]string
}

// probeCheckin GET 首页（index.php），从 HTML 提取真实签到入口：
//  1. 优先签到 <form>：取 action（端点）+ method + 所有 <input type="hidden">（含 formhash）。
//  2. 回退签到 <a href>（非 javascript:）：取该链接为端点，方法 GET。
//  3. 均未命中 → ok=false，调用方回退 def.Checkin.Path/Method/Form。
//
// 解决 attendance.php 端点不对（404）与空 POST 缺 formhash（500）两类问题；
// 探测对正常站无害——失败即回退默认 Path+POST。
func (n *NexusPHP) probeCheckin(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (probeResult, bool) {
	doc, _, err := n.fetchDoc(ctx, baseURL, "index.php", cookie)
	if err != nil || doc == nil {
		return probeResult{}, false
	}
	// 1. 签到表单
	var hit *goquery.Selection
	doc.Find("form").EachWithBreak(func(_ int, f *goquery.Selection) bool {
		action, _ := f.Attr("action")
		btnText := f.Find(`input[type="submit"],input[type="button"],button,a`).Text()
		if isCheckinEntry(action, btnText) {
			hit = f
			return false
		}
		return true
	})
	if hit != nil {
		action := strings.TrimSpace(hit.AttrOr("action", ""))
		method := strings.ToUpper(strings.TrimSpace(hit.AttrOr("method", "")))
		if method == "" {
			method = "GET"
		}
		form := cloneStringMap(def.Checkin.Form)
		hit.Find(`input[type="hidden"]`).Each(func(_ int, inp *goquery.Selection) {
			if k := inp.AttrOr("name", ""); k != "" {
				if _, ok := form[k]; !ok {
					form[k] = inp.AttrOr("value", "")
				}
			}
		})
		if p := relPath(action); p != "" {
			return probeResult{path: p, method: method, form: form}, true
		}
	}
	// 2. 签到链接
	var linkHref string
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href := strings.TrimSpace(a.AttrOr("href", ""))
		if href == "" || strings.HasPrefix(strings.ToLower(href), "javascript:") {
			return true
		}
		if isCheckinEntry(href, a.Text()) {
			linkHref = href
			return false
		}
		return true
	})
	if p := relPath(linkHref); p != "" {
		return probeResult{path: p, method: "GET", form: cloneStringMap(def.Checkin.Form)}, true
	}
	return probeResult{}, false
}

// cloneStringMap 复制 map（避免改写站点原 def）。
func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// relPath 把探测到的 action/href 归一为同站相对路径：去掉 hash、前导斜杠；
// 绝对外站链接（http(s)://）与 javascript: 视为不可用（防跨站/SSRF）。保留 query（GET 签到常带 sign token）。
func relPath(href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	lower := strings.ToLower(href)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "javascript:") {
		return ""
	}
	if i := strings.Index(href, "#"); i >= 0 {
		href = href[:i]
	}
	for len(href) > 0 && href[0] == '/' {
		href = href[1:]
	}
	return strings.TrimSpace(href)
}

// Checkin 实现 site.CheckinCapable。流程：
//  1. EnsureDefaults 填默认；未启用直接返回。
//  2. 探测首页真实签到入口（端点/方法/formhash），命中则用之，否则回退配置。
//  3. 按方法构造请求体（POST=url-encoded form，GET=空），附 def.Checkin.Headers。
//  4. Do 发请求；parseCheckinResponse 识别 HTML/JSON 响应。
func (n *NexusPHP) Checkin(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.CheckinResult, error) {
	def = site.EnsureDefaults(def)
	if !def.Checkin.Enabled {
		return &site.CheckinResult{Success: false, Message: "签到未启用"}, nil
	}

	path, method, form := def.Checkin.Path, def.Checkin.Method, def.Checkin.Form
	if pr, ok := n.probeCheckin(ctx, def, baseURL, cookie); ok {
		path = pr.path
		method = pr.method
		form = pr.form
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != "GET" && method != "POST" {
		method = "POST"
	}

	v := url.Values{}
	for k, val := range form {
		v.Set(k, val)
	}
	headers := cloneStringMap(def.Checkin.Headers)
	var body []byte
	if method == "POST" {
		body = []byte(v.Encode())
		headers["Content-Type"] = "application/x-www-form-urlencoded"
	}

	hc := n.hc
	if def.Checkin.UTLS {
		// WAF 站（如 HDKylin SafeLine 468）签到用 uTLS（Chrome 指纹）拨号绕 TLS 指纹拦截；
		// 构造失败则回退标准 hc（宁可试一次也不静默放弃）。
		if u := n.utlsClient(); u != nil {
			hc = u
		}
	}
	resp, err := hc.Do(ctx, method, joinURL(baseURL, path), body, headers, cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	return parseCheckinResponse(resp, def), nil
}

// parseCheckinResponse 识别签到响应（HTML 与 JSON 通用）：
//  1. 文本关键字：签到成功 / 已签到 / already（JSON 的 msg 文本同样命中，因 body 含其文本）。
//  2. JSON 结构化判定：msg 文本为主，success/ret 辅助（fork 间 ret 语义不统一）。
//  3. loggedOut 未登录。
//  4. 兜底「签到响应未识别(状态N)」。
func parseCheckinResponse(resp *httpclient.Response, def *site.SiteMetadata) *site.CheckinResult {
	body := string(resp.Body)
	lower := strings.ToLower(body)

	// 1. 文本关键字（覆盖纯 HTML 站与 JSON body 含中文/英文关键字的站）
	if strings.Contains(body, "签到成功") || strings.Contains(body, "簽到成功") {
		return &site.CheckinResult{Success: true, Message: "签到成功", Bonus: parseCheckinBonus(body)}
	}
	if strings.Contains(body, "已经签到") || strings.Contains(body, "已经簽到") ||
		strings.Contains(body, "已签到") || strings.Contains(body, "已簽到") ||
		strings.Contains(lower, "already") {
		return &site.CheckinResult{Success: true, Message: "今天已签到"}
	}

	// 2. JSON 结构化判定（msg 未命中关键字时）
	if r := parseCheckinJSON(resp.Body, body); r != nil {
		return r
	}

	// 3. 未登录
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if loggedOut(resp, doc, def.NoLogin) {
		return &site.CheckinResult{Success: false, Message: site.ErrNotLoggedIn.Error()}
	}
	// 4. 兜底
	return &site.CheckinResult{Success: false, Message: fmt.Sprintf("签到响应未识别(状态%d)", resp.StatusCode)}
}

// nexusCheckinJSON 是 NexusPHP JSON 签到响应的宽松结构（字段名随 fork 变化，仅取共性）。
type nexusCheckinJSON struct {
	Ret     *int   `json:"ret"`
	Success *bool  `json:"success"`
	Msg     string `json:"msg"`
	Message string `json:"message"`
}

// parseCheckinJSON 尝试把响应当 JSON 解析；非 JSON 或无法判定返回 nil。
// 成功/已签判定优先用 msg 文本（跨 fork 最稳）；msg 缺失时 success/ret 辅助。
func parseCheckinJSON(raw []byte, body string) *site.CheckinResult {
	var j nexusCheckinJSON
	if json.Unmarshal(raw, &j) != nil {
		return nil
	}
	msg := strings.TrimSpace(j.Msg)
	if msg == "" {
		msg = strings.TrimSpace(j.Message)
	}
	lower := strings.ToLower(msg)
	bonus := parseCheckinBonus(body)

	switch {
	// 已签（视为成功）
	case strings.Contains(lower, "already") || strings.Contains(msg, "重复") ||
		strings.Contains(msg, "已签") || strings.Contains(msg, "已经"):
		return &site.CheckinResult{Success: true, Message: "今天已签到"}
	// msg 明示成功
	case strings.Contains(msg, "成功") || strings.Contains(lower, "success"):
		return &site.CheckinResult{Success: true, Message: orStr(msg, "签到成功"), Bonus: bonus}
	// 显式 success:true
	case j.Success != nil && *j.Success:
		return &site.CheckinResult{Success: true, Message: orStr(msg, "签到成功"), Bonus: bonus}
	// msg 缺失时按 ret 推断（NexusPHP 官方 ret==0 成功，但 fork 不一；仅在无 msg 时用）
	case j.Ret != nil && msg == "":
		if *j.Ret == 0 {
			return &site.CheckinResult{Success: true, Message: "签到成功", Bonus: bonus}
		}
		return &site.CheckinResult{Success: false, Message: fmt.Sprintf("签到失败(ret=%d)", *j.Ret)}
	// JSON 但带错误 msg
	case msg != "":
		return &site.CheckinResult{Success: false, Message: msg}
	}
	return nil
}

var bonusRe = regexp.MustCompile(`获得\s*(\d+(?:\.\d+)?)\s*(?:个)?\s*(?:火花|魔力|bonus)`)

// parseCheckinBonus 从响应抽取奖励：先 JSON bonus/reward/money/gain 字段（复用 flarumCheckinBonusRe），
// 再 HTML「获得 N 火花/魔力」。
func parseCheckinBonus(body string) float64 {
	if m := flarumCheckinBonusRe.FindStringSubmatch(body); len(m) >= 2 {
		if f, _ := strconv.ParseFloat(m[1], 64); f > 0 {
			return f
		}
	}
	if m := bonusRe.FindStringSubmatch(body); len(m) >= 2 {
		if f, _ := strconv.ParseFloat(m[1], 64); f > 0 {
			return f
		}
	}
	return 0
}
