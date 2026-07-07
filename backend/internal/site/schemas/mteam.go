package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// MTeam 是 M-Team（馒头）适配器：走 api.m-team.cc 的 JSON API，用 x-api-key 鉴权。
// 与 NexusPHP（HTML 抓取）不同——所有数据来自 POST JSON 端点，无需选择器。
// 契约来源：pt_mate-master/lib/services/api/mteam_adapter.dart（在用的实现）。
type MTeam struct {
	hc *httpclient.Client
}

// NewMTeam 构造适配器，注入共享 httpclient。
func NewMTeam(hc *httpclient.Client) *MTeam { return &MTeam{hc: hc} }

// Schema 返回平台标识。
func (m *MTeam) Schema() string { return "mteam" }

// Name / Priority 实现 module.Module（与 NexusPHP 同构）。
func (m *MTeam) Name() string  { return "site.mteam" }
func (m *MTeam) Priority() int { return 100 }

// mtEnvelope 是 MTeam 统一响应信封：Code=="0"（字符串）表示成功。
type mtEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Search POST /api/torrent/search。token 为 apiKey（由 chain.credential 分发）。
// category 决定搜索分类模式：movie→mode:movie、tv→mode:tvshow，其余（含 ""综合/music）
// 退化为站点 extra_param.mode（默认 normal）。music 无独立 mode，靠 keyword 命中。
func (m *MTeam) Search(ctx context.Context, def *site.SiteMetadata, baseURL, token, keyword, category string, page int) ([]site.Torrent, error) {
	if page < 1 {
		page = 1
	}
	// 解析 mode：先取 extra_param.mode（默认 normal），再按 category 覆盖。
	mode := "normal"
	if em := def.ExtraParam["mode"]; em != "" {
		mode = em
	}
	switch category {
	case "movie":
		mode = "movie"
	case "tv":
		mode = "tvshow"
	}
	body := map[string]any{
		"visible":    1,
		"pageNumber": page,
		"pageSize":   30,
		"mode":       mode,
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		body["keyword"] = keyword
	}
	// 合并站点定义的额外参数；跳过 mode 键（已单独解析，避免把上面的映射又冲回 normal）。
	for k, v := range def.ExtraParam {
		if k == "mode" {
			continue
		}
		body[k] = v
	}

	data, err := m.mtRequest(ctx, baseURL, token, "/api/torrent/search", body)
	if err != nil {
		return nil, err
	}
	// data: {total, pageNumber, pageSize, totalPages, data:[...]}
	// 注意：MTeam API 把 total/pageNumber 等数值返成字符串（"123"），用 json.Number
	// 容纳（同时接受数字与字符串形态），避免 unmarshal string→int64 报错。
	var pageData struct {
		Total json.Number      `json:"total"`
		Items []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(data, &pageData); err != nil {
		return nil, fmt.Errorf("parse mteam search data: %w", err)
	}
	out := make([]site.Torrent, 0, len(pageData.Items))
	for _, it := range pageData.Items {
		out = append(out, parseMTTorrent(it))
	}
	return out, nil
}

// FetchUserInfo POST /api/member/profile。
func (m *MTeam) FetchUserInfo(ctx context.Context, def *site.SiteMetadata, baseURL, token string) (*site.UserInfo, error) {
	data, err := m.mtRequest(ctx, baseURL, token, "/api/member/profile", map[string]any{})
	if err != nil {
		return nil, err
	}
	var prof struct {
		ID           any    `json:"id"` // 用户 UID（数字）
		Username     string `json:"username"`
		Email        string `json:"email"`       // 账号邮箱（best-effort，可能为空）
		AvatarURL    string `json:"avatarUrl"`   // 头像 URL（best-effort，可能为空或默认头像）
		Role         any    `json:"role"`        // 用户组 levelId（NexusPHP class：1=User … 10=VIP … 16=Staff），数字或字符串
		CreatedDate  string `json:"createdDate"` // 加入时间，如 "2023-01-02 03:04:05"
		MemberStatus struct {
			CreatedDate string `json:"createdDate"`
			LastBrowse  string `json:"lastBrowse"`
			Donor       any    `json:"donor"`
		} `json:"memberStatus"`
		MemberCount struct {
			Uploaded   any `json:"uploaded"`
			Downloaded any `json:"downloaded"`
			Bonus      any `json:"bonus"`
			ShareRate  any `json:"shareRate"`
		} `json:"memberCount"`
	}
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("parse mteam profile: %w", err)
	}
	peer := m.fetchMTPeerStats(ctx, baseURL, token)
	// UID 为数字（profile.id），转字符串；缺失留空。
	idStr := ""
	if id := toInt64(prof.ID); id > 0 {
		idStr = strconv.FormatInt(id, 10)
	}
	joinDate := prof.CreatedDate
	if joinDate == "" {
		joinDate = prof.MemberStatus.CreatedDate
	}
	levelID := int(toInt64(prof.Role))
	return &site.UserInfo{
		ID:           idStr,
		Name:         prof.Username,
		Email:        prof.Email,
		Avatar:       site.ApplyFilters(prof.AvatarURL, []site.Filter{{Name: "clean_avatar"}}),
		IsDonor:      toBool(prof.MemberStatus.Donor),
		Level:        mtLevelName(prof.Role),
		LevelID:      levelID,
		JoinDate:     joinDate,
		LastAccessAt: prof.MemberStatus.LastBrowse,
		Uploaded:     toInt64(prof.MemberCount.Uploaded),
		Downloaded:   toInt64(prof.MemberCount.Downloaded),
		Bonus:        toFloat64(prof.MemberCount.Bonus),
		Ratio:        toFloat64(prof.MemberCount.ShareRate),
		Seeding:      peer.Seeding,
		Leeching:     peer.Leeching,
		SeedingSize:  peer.SeedingSize,
		Uploads:      peer.Uploads,
		BonusPerHour: m.fetchMTBonusPerHour(ctx, baseURL, token),
	}, nil
}

// mtPeerStats 是 /api/tracker/myPeerStatistics 的抓取结果（做种/下载中/做种体积/发布数）。
type mtPeerStats struct {
	Seeding     int
	Leeching    int
	SeedingSize int64
	Uploads     int
}

// fetchMTPeerStats 抓 /api/tracker/myPeerStatistics 取做种统计。
// best-effort：失败返回零值，不影响主资料抓取。契约来源 PT-depiler / 旧版 PTPatronus。
func (m *MTeam) fetchMTPeerStats(ctx context.Context, baseURL, token string) mtPeerStats {
	data, err := m.mtRequest(ctx, baseURL, token, "/api/tracker/myPeerStatistics", map[string]any{})
	if err != nil {
		return mtPeerStats{}
	}
	var peer struct {
		SeederSize   any `json:"seederSize"`
		SeederCount  any `json:"seederCount"`
		LeecherCount any `json:"leecherCount"`
		UploadCount  any `json:"uploadCount"`
	}
	if err := json.Unmarshal(data, &peer); err != nil {
		return mtPeerStats{}
	}
	return mtPeerStats{
		Seeding:     int(toInt64(peer.SeederCount)),
		Leeching:    int(toInt64(peer.LeecherCount)),
		SeedingSize: toInt64(peer.SeederSize),
		Uploads:     int(toInt64(peer.UploadCount)),
	}
}

// fetchMTBonusPerHour 抓 /api/tracker/mybonus 取「每小时魔力」(finalBs)。
// best-effort：失败/缺失返回 0，不影响主资料抓取。契约来源 PT-depiler / 旧版 PTPatronus。
// 优先 formulaParams.finalBs（现行结构），回退顶层 finalBs（兼容旧形态）。
func (m *MTeam) fetchMTBonusPerHour(ctx context.Context, baseURL, token string) float64 {
	data, err := m.mtRequest(ctx, baseURL, token, "/api/tracker/mybonus", map[string]any{})
	if err != nil {
		return 0
	}
	var bonus struct {
		FinalBs       any `json:"finalBs"`
		FormulaParams struct {
			FinalBs any `json:"finalBs"`
		} `json:"formulaParams"`
	}
	if err := json.Unmarshal(data, &bonus); err != nil {
		return 0
	}
	if v := bonus.FormulaParams.FinalBs; v != nil {
		return toFloat64(v)
	}
	return toFloat64(bonus.FinalBs)
}

// FetchAnnouncement 馒头：POST /api/news/list（空 body 即可），解析新闻数组。
// 实测端点可用（x-api-key 即可，无需更高权限）：data[] 含 subject/context/createdDate。
// 这是馒头唯一稳定的公告 JSON 端点（其余 /api/notice* 等为 No static resource 或被 CF 挡）。
func (m *MTeam) FetchAnnouncement(ctx context.Context, def *site.SiteMetadata, baseURL, token string) (*site.SiteAnnouncement, error) {
	data, err := m.mtRequest(ctx, baseURL, token, "/api/news/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var items []struct {
		Subject     string `json:"subject"`
		Context     string `json:"context"`
		CreatedDate string `json:"createdDate"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now()}, nil
	}
	a := &site.SiteAnnouncement{FetchedAt: time.Now()}
	for _, it := range items {
		body := it.Context
		if body == "" {
			body = it.Subject
		}
		a.Items = append(a.Items, site.AnnouncementItem{
			Title:   it.Subject,
			Content: body,
			Date:    it.CreatedDate,
		})
	}
	return a, nil
}

// FetchFreeleech 馒头：POST /api/system/promotion/rules 取促销规则，找「全站 FREE」
// （discount 含 FREE 且 categories/teams 空=不限分类/组=全站），按 startTime/endTime 判
// upcoming（预告，如「【预告】全站促销 Free, 07月01日 開始」）/ active（进行中）。
func (m *MTeam) FetchFreeleech(ctx context.Context, def *site.SiteMetadata, baseURL, token string) ([]site.SiteFreeleech, error) {
	data, err := m.mtRequest(ctx, baseURL, token, "/api/system/promotion/rules", map[string]any{})
	if err != nil {
		return nil, err
	}
	var rules []struct {
		Discount   string `json:"discount"`
		Categories []any  `json:"categories"`
		Teams      []any  `json:"teams"`
		StartTime  string `json:"startTime"`
		EndTime    string `json:"endTime"`
	}
	if err := json.Unmarshal(data, &rules); err != nil {
		return []site.SiteFreeleech{{Status: site.FreeleechNone, Title: "暂无站免", FetchedAt: time.Now()}}, nil
	}
	now := time.Now()
	const layout = "2006-01-02 15:04:05"
	var out []site.SiteFreeleech
	for _, r := range rules {
		if !strings.Contains(strings.ToUpper(r.Discount), "FREE") {
			continue
		}
		if len(r.Categories) > 0 || len(r.Teams) > 0 {
			continue // 限分类/组的促销，非全站
		}
		status, title := site.FreeleechActive, "全站免费"
		start, _ := time.ParseInLocation(layout, r.StartTime, time.Local)
		end, _ := time.ParseInLocation(layout, r.EndTime, time.Local)
		content := ""
		if !start.IsZero() {
			content = r.StartTime + " 起"
			if now.Before(start) {
				status, title = site.FreeleechUpcoming, "即将全站免费"
			}
		}
		if !end.IsZero() {
			if content != "" {
				content += " ~ "
			}
			content += r.EndTime
		}
		out = append(out, site.SiteFreeleech{Status: status, Title: title, Content: content, FetchedAt: time.Now()})
	}
	if len(out) == 0 {
		return []site.SiteFreeleech{{Status: site.FreeleechNone, Title: "暂无站免", FetchedAt: time.Now()}}, nil
	}
	return out, nil
}

// mtRequest 发送 JSON POST，校验信封，返回 data 原始 JSON。
// HTTP 401/403 或鉴权类业务错误 → site.ErrNotLoggedIn；其它 code!="0" → 带消息的错误。
func (m *MTeam) mtRequest(ctx context.Context, baseURL, token, path string, body map[string]any) (json.RawMessage, error) {
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
	if env.Code != "0" {
		if isMTAuthError(env.Code, env.Message) {
			return nil, site.ErrNotLoggedIn
		}
		return nil, fmt.Errorf("mteam api error: code=%s msg=%s", env.Code, env.Message)
	}
	return env.Data, nil
}

// GenDlToken 取 MTeam 签名下载 URL：multipart/form-data POST /api/torrent/genDlToken {id}，
// 头 x-api-key。契约源 pt_mate mteam_adapter.genDlToken（data 为签名 URL 字符串）。
// URL 有时效（签名），调用方应在「添加」时现取，勿提前入库缓存。
func (m *MTeam) GenDlToken(ctx context.Context, baseURL, apiKey, torrentID string) (string, error) {
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
	if env.Code != "0" {
		return "", fmt.Errorf("genDlToken error: code=%s msg=%s", env.Code, env.Message)
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
