package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

// mtAPIBase 把 baseURL 归一化到 M-Team 的 JSON API 主机 api.m-team.cc。
//
// M-Team 的 JSON API（/api/...）只服务于 api.m-team.cc；网页主机 kp.m-team.cc /
// personal.m-team.cc 受 Cloudflare 严防且并无该 API。MP/.ptd 导入与手动添加常把
// 网页地址写进 BaseURL，致连通性检测 / 搜索 / 下载签名全部撞 Cloudflare（表现为
// 「blocked by cloudflare on https://kp.m-team.cc」）。
//
// 仅改写 M-Team 官方域（m-team.cc 及其子域）下、非 api 的主机；用户自建反代
// （如 https://mt-proxy.example.com）与任意第三方域原样保留，不破坏自定义接入。
// 解析失败也原样返回，交由下游 joinURL / 请求层报错，不在 helper 里吞异常。
func mtAPIBase(baseURL string) string {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return baseURL
	}
	host := strings.ToLower(u.Hostname())
	if host == "api.m-team.cc" {
		return baseURL // 已是 API 主机，原样返回（不引入 path/大小写等意外差异）
	}
	if host == "m-team.cc" || strings.HasSuffix(host, ".m-team.cc") {
		u.Host = "api.m-team.cc"
		return u.String()
	}
	return baseURL
}

// mtRequest 发送 JSON POST，校验信封，返回 data 原始 JSON。
// HTTP 401/403 或鉴权类业务错误 → site.ErrNotLoggedIn；其它 code!="0" → 带消息的错误。
func (m *MTeam) mtRequest(ctx context.Context, baseURL, token, path string, body map[string]any) (json.RawMessage, error) {
	baseURL = mtAPIBase(baseURL) // 网页主机（kp./personal.）→ api.m-team.cc，绕 Cloudflare
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := m.hc.Do(ctx, "POST", joinURL(baseURL, path), payload, map[string]string{
		"x-api-key":    token,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, "", true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var env mtEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("parse mteam response: %w", err)
	}
	code := env.Code.String()
	if code != "0" {
		if isMTAuthError(code, env.Message) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("mteam api error: code=%s msg=%s", code, env.Message)
	}
	return env.Data, nil
}

// GenDlToken 取 MTeam 签名下载 URL：multipart/form-data POST /api/torrent/genDlToken {id}，
// 头 x-api-key。契约源 pt_mate mteam_adapter.genDlToken（data 为签名 URL 字符串）。
// URL 有时效（签名），调用方应在「添加」时现取，勿提前入库缓存。
func (m *MTeam) GenDlToken(ctx context.Context, baseURL, apiKey, torrentID string) (string, error) {
	baseURL = mtAPIBase(baseURL) // 同 mtRequest：网页主机 → api.m-team.cc
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("id", torrentID)
	if err := w.Close(); err != nil {
		return "", err
	}
	resp, err := m.hc.Do(ctx, "POST", joinURL(baseURL, "/api/torrent/genDlToken"), buf.Bytes(), map[string]string{
		"Content-Type": w.FormDataContentType(),
		"x-api-key":    apiKey,
		"Accept":       "application/json, text/plain, */*",
	}, "", true)
	if err != nil {
		return "", err
	}
	if resp.IsCloudflare() {
		return "", fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", site.ErrNotLoggedIn
	}
	var env mtEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return "", fmt.Errorf("parse genDlToken response: %w", err)
	}
	if env.Code.String() != "0" {
		return "", fmt.Errorf("genDlToken error: code=%s msg=%s", env.Code.String(), env.Message)
	}
	var dlURL string
	if err := json.Unmarshal(env.Data, &dlURL); err != nil {
		return "", fmt.Errorf("genDlToken data not a string: %w", err)
	}
	if dlURL == "" {
		return "", fmt.Errorf("genDlToken returned empty url (id=%s)", torrentID)
	}
	return dlURL, nil
}

// parseMTTorrent 把一条搜索结果映射为 site.Torrent。
func parseMTTorrent(it map[string]any) site.Torrent {
	status, _ := it["status"].(map[string]any)
	id := fmt.Sprint(it["id"])
	t := site.Torrent{
		ID:       id,
		Title:    getString(it, "name"),
		SubTitle: getString(it, "smallDescr"),
		Size:     formatMTBytes(toInt64(it["size"])),
		Seeders:  int(toInt64(getAny(status, "seeders"))),
		Leechers: int(toInt64(getAny(status, "leechers"))),
	}
	// MTeam 无公开详情 HTML 页；合成 web 深链作 DetailURL（同时承载 id 供详情抓取解析 sn）。
	// 首页/搜索结果点击可跳 MTeam web；后端详情端点从该 URL 抽 id 调 /api/torrent/detail。
	if id != "" {
		t.DetailURL = "https://kp.m-team.cc/detail/" + id
	}
	// FREE 判定：置顶(toppingLevel==1) 或 优惠(discount/promotionRule.discount==FREE)
	if toInt64(getAny(status, "toppingLevel")) == 1 {
		t.Free = true
	} else if mtIsFree(status) {
		t.Free = true
	}
	return t
}

func mtIsFree(status map[string]any) bool {
	disc := ""
	if pr, _ := status["promotionRule"].(map[string]any); pr != nil {
		disc = getString(pr, "discount")
	}
	if disc == "" {
		disc = getString(status, "discount")
	}
	return strings.Contains(strings.ToUpper(disc), "FREE")
}

// isMTAuthError 启发式判定业务错误是否为鉴权失效（apiKey 无效/过期）。
func isMTAuthError(code, msg string) bool {
	low := strings.ToLower(code + " " + msg)
	for _, k := range []string{"api key", "apikey", "token", "授权", "未授权", "unauthor"} {
		if strings.Contains(low, k) {
			return true
		}
	}
	return false
}

// ── 用户等级映射 ──

// mtRoles 对照 NexusPHP 标准 class（MTeam profile 的 role 字段）→ 等级名。
// 契约来源：MoviePilot MTeam_sysRoleList / PT-depiler levelRequirements（二者一致）。
var mtRoles = map[int64]string{
	1: "User", 2: "Power User", 3: "Elite User", 4: "Crazy User",
	5: "Insane User", 6: "Veteran User", 7: "Extreme User", 8: "Ultimate User",
	9: "mTorrent Master", 10: "VIP", 11: "Retiree", 12: "Uploader",
	13: "Moderator", 14: "Administrator", 15: "Sysop", 16: "Staff",
}

// mtLevelName 把 MTeam profile 的 role（用户组 levelId）映射为等级名。
// role 缺失/为空 → ""（杜绝 fmt.Sprint(nil) 产生 "<nil>"）；
// 未命中编号 → 回退原始字符串或数字，保留可读性。
func mtLevelName(role any) string {
	if id := toInt64(role); id > 0 {
		if name, ok := mtRoles[id]; ok {
			return name
		}
		return strconv.FormatInt(id, 10)
	}
	if s, ok := role.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// ── 容错数值/字符串解析（MTeam JSON 数字字段类型不稳定：int/float/string 混用）──

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	case bool:
		if n {
			return 1
		}
	}
	return 0
}

func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	return 0
}

func toBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return site.ToBool(x)
	case float64:
		return x != 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case json.Number:
		i, _ := x.Int64()
		return i != 0
	}
	return false
}

func getString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func getAny(m map[string]any, key string) any {
	if m == nil {
		return nil
	}
	return m[key]
}

// formatMTBytes 字节 → "1.5 GB"（与 NexusPHP Size 文本风格一致，供前端统一展示）。
func formatMTBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
