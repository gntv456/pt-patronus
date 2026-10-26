package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Flarum 是 Flarum 论坛社区适配器（invites.fun 药丸 / pting.club 蜂巢 等 PT 论坛）。
//
// 与 NexusPHP（HTML 抓取）不同——数据走 Flarum JSON:API：
//   - GET /api           取当前用户 uid（data.relationships.actor.data.id）+ 用户组名表
//   - GET /api/users/{uid} 取用户 attributes（金币/发帖/加入时间/头像）
//
// 鉴权用 flarum_session cookie（chain.credential 默认返回 cookie，无需新增 case）。
// 论坛无 PT 种子，Search 返回空；UserInfo 把金币(money)→Bonus、用户组→Level、
// 发帖+回复→Posts、joinTime→JoinDate 做映射（上传/下载/分享率/做种论坛没有，留零值）。
//
// 写操作（签到）需 CSRF token：token 嵌在首页 HTML 的 JS payload（"csrfToken":"..."），
// 不在 Set-Cookie。保活 cron 会轮换 flarum_session 使旧 token 失效，故每次签到现取。
type Flarum struct {
	hc *httpclient.Client
}

// NewFlarum 构造适配器，注入共享 httpclient。
func NewFlarum(hc *httpclient.Client) *Flarum { return &Flarum{hc: hc} }

// Schema 返回平台标识。
func (f *Flarum) Schema() string { return "flarum" }

// Name / Priority 实现 module.Module（与 MTeam 同构）。
func (f *Flarum) Name() string  { return "site.flarum" }
func (f *Flarum) Priority() int { return 100 }

// Search 论坛无种子，返回空。实现 site.Adapter 必选方法以满足接口契约。
func (f *Flarum) Search(ctx context.Context, def *site.SiteMetadata, baseURL, cookie, keyword string, category string, page int) ([]site.Torrent, error) {
	return nil, nil
}

// flarumRef 是 JSON:API 关系引用（{type,id}）。
type flarumRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// flarumRelationships 是资源的 relationships 段（仅取需要的 groups/actor）。
type flarumRelationships struct {
	Groups struct {
		Data []flarumRef `json:"data"`
	} `json:"groups"`
}

// flarumResource 是 JSON:API 资源（forum document 的 data、included[]、user document 的 data）。
type flarumResource struct {
	Type          string              `json:"type"`
	ID            string              `json:"id"`
	Attributes    map[string]any      `json:"attributes"`
	Relationships flarumRelationships `json:"relationships"`
}

// flarumForumDoc 是 GET /api 的响应结构。
type flarumForumDoc struct {
	Data struct {
		Relationships struct {
			// 当前登录用户关系：JSON:API 形如 {"data":{"type":"users","id":"6457"}}。
			// 标准为 currentUser，这两个二开站用 actor；两者皆查兼容上游与二开。
			Actor *struct {
				Data *flarumRef `json:"data"`
			} `json:"actor"`
			CurrentUser *struct {
				Data *flarumRef `json:"data"`
			} `json:"currentUser"`
		} `json:"relationships"`
	} `json:"data"`
	Included []flarumResource `json:"included"`
}

// currentUserID 从 forum document 解析当前登录用户 id。
// 兼容 actor（invites.fun/pting.club 二开）与标准 currentUser；均缺失视为未登录。
func (doc *flarumForumDoc) currentUserID() string {
	if r := doc.Data.Relationships.Actor; r != nil && r.Data != nil && r.Data.ID != "" {
		return r.Data.ID
	}
	if r := doc.Data.Relationships.CurrentUser; r != nil && r.Data != nil && r.Data.ID != "" {
		return r.Data.ID
	}
	return ""
}

// groupTable 从 included[] 构建 group id → nameSingular 映射（用户组名=等级名）。
func (doc *flarumForumDoc) groupTable() map[string]string {
	m := make(map[string]string, len(doc.Included))
	for _, inc := range doc.Included {
		if inc.Type != "groups" {
			continue
		}
		if n, ok := inc.Attributes["nameSingular"].(string); ok && n != "" {
			m[inc.ID] = n
		}
	}
	return m
}

// FetchUserInfo GET /api 取 uid+组名表，再 GET /api/users/{uid} 取资料，映射到 UserInfo。
func (f *Flarum) FetchUserInfo(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.UserInfo, error) {

	// Step1: /api 取当前 uid + groups 表。
	resp, err := f.hc.Get(ctx, joinURL(baseURL, "/api"), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	var forum flarumForumDoc
	if err := json.Unmarshal(resp.Body, &forum); err != nil {
		return nil, fmt.Errorf("parse flarum /api: %w", err)
	}
	uid := forum.currentUserID()
	if uid == "" {
		return nil, site.ErrNotLoggedIn
	}
	groupNames := forum.groupTable()

	// Step2: /api/users/{uid} 取用户 attributes。
	resp2, err := f.hc.Get(ctx, joinURL(baseURL, "/api/users/"+uid), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp2.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	if resp2.StatusCode == 401 || resp2.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	var userDoc struct {
		Data flarumResource `json:"data"`
	}
	if err := json.Unmarshal(resp2.Body, &userDoc); err != nil {
		return nil, fmt.Errorf("parse flarum user: %w", err)
	}
	a := userDoc.Data.Attributes

	// Level：用户组 nameSingular 拼接（如「PT用户, 认证会员」）；LevelID 取首个组 id。
	var levels []string
	firstGroupID := 0
	for i, g := range userDoc.Data.Relationships.Groups.Data {
		if n, ok := groupNames[g.ID]; ok && n != "" {
			levels = append(levels, n)
		}
		if i == 0 {
			firstGroupID, _ = strconv.Atoi(g.ID)
		}
	}

	name := strAttr(a, "username")
	if name == "" {
		name = strAttr(a, "displayName")
	}
	numID, _ := strconv.Atoi(uid)

	return &site.UserInfo{
		ID:            uid,
		NumericUserID: numID,
		Name:          name,
		Avatar:        site.ApplyFilters(strAttr(a, "avatarUrl"), []site.Filter{{Name: "clean_avatar"}}),
		Level:         strings.Join(levels, ", "),
		LevelID:       firstGroupID,
		Bonus:         toFloat64(a["money"]),
		Posts:         int(toInt64(a["discussionCount"]) + toInt64(a["commentCount"])),
		JoinDate:      strAttr(a, "joinTime"),
		LastAccessAt:  strAttr(a, "lastCheckinTime"),
	}, nil
}

// strAttr 从 attributes map 取字符串属性（容忍 nil/非字符串）。
func strAttr(a map[string]any, key string) string {
	if a == nil {
		return ""
	}
	if v, ok := a[key].(string); ok {
		return v
	}
	return ""
}

// KeepAlive 实现 site.KeepAliveCapable：GET /api，actor/currentUser 关系存在即登录态；
// 捕获 flarum_session 轮换后的最新 cookie 回写 DB。自动接入 cookie-keepalive cron。
func (f *Flarum) KeepAlive(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.KeepAliveResult, error) {
	resp, err := f.hc.Get(ctx, joinURL(baseURL, "/api"), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	loggedIn := false
	var forum flarumForumDoc
	if json.Unmarshal(resp.Body, &forum) == nil {
		loggedIn = forum.currentUserID() != ""
	}
	refreshed, _ := f.hc.Cookies(joinURL(baseURL, "/"))
	return &site.KeepAliveResult{LoggedIn: loggedIn, RefreshedCookie: refreshed}, nil
}

// flarumCSRFRe 从首页 HTML 的 JS payload 提取 CSRF token。
// 实测两站均把 token 嵌在 "csrfToken":"<40字符>" 而非 Set-Cookie。
var flarumCSRFRe = regexp.MustCompile(`"csrfToken"\s*:\s*"([^"]+)"`)

// fetchCSRFToken GET 首页 HTML 提取 CSRF token（Flarum 写操作必需）。
// 每次签到现取：保活 cron 轮换 flarum_session 会使缓存的 token 失效。
func (f *Flarum) fetchCSRFToken(ctx context.Context, baseURL, cookie string) (string, error) {
	resp, err := f.hc.Get(ctx, joinURL(baseURL, "/"), cookie, true)
	if err != nil {
		return "", err
	}
	m := flarumCSRFRe.FindSubmatch(resp.Body)
	if len(m) < 2 {
		return "", fmt.Errorf("flarum csrf token not found on %s homepage", baseURL)
	}
	return string(m[1]), nil
}

// flarumCheckinPath 从定义取签到端点路径，默认 /api/checkin（invites.fun 已确认）。
// pting.club 端点非标准、待运行时校准；可在 YAML extra_param.checkin_path 覆盖。
func flarumCheckinPath(def *site.SiteMetadata) string {
	if p := strings.TrimSpace(def.ExtraParam["checkin_path"]); p != "" {
		return p
	}
	return "/api/checkin"
}

// flarumErrorEnvelope 是 Flarum JSON:API 错误信封。
type flarumErrorEnvelope struct {
	Errors []struct {
		Status string `json:"status"`
		Code   string `json:"code"`
		Detail string `json:"detail"`
		Source struct {
			Pointer string `json:"pointer"`
		} `json:"source"`
	} `json:"errors"`
}

// Checkin 实现 site.CheckinCapable。两种签到模式（extra_param.checkin_mode）：
//   - "patch_user"（pting.club，Ziiven/flarum-daily-check-in 扩展）：PATCH /api/users/{uid}
//     {canCheckin:false}，后端监听 Flarum Saving 事件触发签到。注意：该请求对 TLS 指纹
//     敏感——curl 的指纹会被 Cloudflare 降级致 attributes 静默丢失，须用本 httpclient。
//   - 默认 POST（invites.fun）：POST {checkin_path}。
//
// 两者都需先 GET 首页提 CSRF token；自动接入每日签到 cron（10 8 * * *）+ 一键签到。
func (f *Flarum) Checkin(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.CheckinResult, error) {
	def = site.EnsureDefaults(def)
	tok, err := f.fetchCSRFToken(ctx, baseURL, cookie)
	if err != nil {
		return &site.CheckinResult{Success: false, Message: err.Error()}, nil
	}
	if def.ExtraParam["checkin_mode"] == "patch_user" {
		return f.checkinPatchUser(ctx, baseURL, cookie, tok)
	}
	return f.checkinPost(ctx, def, baseURL, cookie, tok)
}

// flarumWriteHeaders 是 Flarum 写操作（签到）的公共请求头。
func flarumWriteHeaders(tok string) map[string]string {
	return map[string]string{
		"X-CSRF-Token": tok,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}
}

// fetchCurrentUserID GET /api 取当前登录用户 id（actor/currentUser），未登录或出错返回空。
func (f *Flarum) fetchCurrentUserID(ctx context.Context, baseURL, cookie string) string {
	resp, err := f.hc.Get(ctx, joinURL(baseURL, "/api"), cookie, true)
	if err != nil || resp.IsCloudflare() {
		return ""
	}
	var forum flarumForumDoc
	if json.Unmarshal(resp.Body, &forum) != nil {
		return ""
	}
	return forum.currentUserID()
}

// checkinPost POST {checkin_path} 签到（invites.fun 风格）。
func (f *Flarum) checkinPost(ctx context.Context, def *site.SiteMetadata, baseURL, cookie, tok string) (*site.CheckinResult, error) {
	path := flarumCheckinPath(def)
	resp, err := f.hc.Do(ctx, "POST", joinURL(baseURL, path), []byte(`{}`), flarumWriteHeaders(tok), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	return f.parseCheckinResponse(resp), nil
}

// checkinPatchUser PATCH /api/users/{uid} {canCheckin:false} 签到（pting.club，Ziiven 扩展）。
// Ziiven 扩展监听 Flarum Saving 事件，检测到 canCheckin attribute 触发签到；totalContinuousCheckIn
// 由后端重算（客户端值被覆盖），故恒传 1 即可。
func (f *Flarum) checkinPatchUser(ctx context.Context, baseURL, cookie, tok string) (*site.CheckinResult, error) {
	uid := f.fetchCurrentUserID(ctx, baseURL, cookie)
	if uid == "" {
		return &site.CheckinResult{Success: false, Message: site.ErrNotLoggedInText}, nil
	}
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{
			"type": "users",
			"id":   uid,
			"attributes": map[string]any{
				"canCheckin":             false,
				"totalContinuousCheckIn": 1,
			},
		},
	})
	resp, err := f.hc.Do(ctx, "PATCH", joinURL(baseURL, "/api/users/"+uid), body, flarumWriteHeaders(tok), cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站）", baseURL)
	}
	return f.parseCheckinResponse(resp), nil
}

// parseCheckinResponse 解析签到响应（POST/PATCH 通用）：
// 401/403 未登录；422 今日已签；csrf_mismatch 失败；无 errors 成功。
func (f *Flarum) parseCheckinResponse(resp *httpclient.Response) *site.CheckinResult {
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return &site.CheckinResult{Success: false, Message: site.ErrNotLoggedInText}
	}
	var env flarumErrorEnvelope
	if json.Unmarshal(resp.Body, &env) == nil && len(env.Errors) > 0 {
		e := env.Errors[0]
		switch {
		case e.Code == "csrf_token_mismatch":
			// 理论上每次现取 token 不应发生；兜底提示重试。
			return &site.CheckinResult{Success: false, Message: "CSRF token 失效，请重试"}
		case e.Status == "422":
			// 签到扩展对「今日已签」返回 422（实测 invites.fun: source /data/attributes/message）。
			return &site.CheckinResult{Success: true, Message: "今天已签到"}
		default:
			detail := e.Detail
			if detail == "" {
				detail = "HTTP " + e.Status
			}
			return &site.CheckinResult{Success: false, Message: fmt.Sprintf("签到失败: %s", detail)}
		}
	}
	// 无 errors = 成功；best-effort 从响应抽取奖励金币（reward/bonus/money 字段）。
	bonus := parseFlarumCheckinBonus(string(resp.Body))
	msg := "签到成功"
	if bonus > 0 {
		msg = fmt.Sprintf("签到成功，获得 %.0f 金币", bonus)
	}
	return &site.CheckinResult{Success: true, Message: msg, Bonus: bonus}
}

// flarumCheckinBonusRe 从签到成功响应抽取奖励数额（兼容 money/reward/bonus 多种字段名）。
var flarumCheckinBonusRe = regexp.MustCompile(`(?i)"(?:reward|bonus|money|gain)[A-Za-z]*"\s*:\s*(\d+(?:\.\d+)?)`)

func parseFlarumCheckinBonus(body string) float64 {
	m := flarumCheckinBonusRe.FindStringSubmatch(body)
	if len(m) >= 2 {
		v, _ := strconv.ParseFloat(m[1], 64)
		return v
	}
	return 0
}
