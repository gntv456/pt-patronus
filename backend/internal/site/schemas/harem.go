package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchInviteStatus 实现 site.HaremCapable：邀请库存。
func (n *NexusPHP) FetchInviteStatus(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.InviteStatus, error) {

	def = site.EnsureDefaults(def)
	uid := site.ExtractUserIDFromCookie(cookie)
	path := def.Harem.InvitePath
	if path == "" {
		path = "invite.php"
	}
	u := path
	if uid != "" {
		u = path + "?id=" + uid
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, u, cookie)
	if err != nil {
		return &site.InviteStatus{Reason: "网络异常: " + err.Error()}, nil
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseInviteStatus(doc), nil
}

// FetchInvitees 实现 site.HaremCapable：被邀请人花名册（翻页，硬上限 20 页，空页停）。
func (n *NexusPHP) FetchInvitees(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) ([]site.InviteeInfo, error) {

	def = site.EnsureDefaults(def)
	uid := site.ExtractUserIDFromCookie(cookie)
	if uid == "" {
		return nil, nil
	}
	base := def.Harem.InviteePath
	if base == "" {
		base = "invite.php"
	}
	maxPages := def.Harem.MaxPages
	if maxPages <= 0 {
		maxPages = 20
	}
	var out []site.InviteeInfo
	var prevSig string
	for page := 1; page <= maxPages; page++ {
		u := fmt.Sprintf("%s?id=%s&menu=invitee", base, uid)
		if page > 1 {
			u += fmt.Sprintf("&page=%d", page)
		}
		doc, resp, err := n.fetchDoc(ctx, baseURL, u, cookie)
		if err != nil {
			break
		}
		if loggedOut(resp, doc, def.NoLogin) {
			break
		}
		rows := site.ParseInvitees(doc)
		if len(rows) == 0 {
			break // 空页 = 结束
		}
		// 无进展检测：部分 NexusPHP 皮肤不识别 &page= 分页，每页返回相同第 1 页；
		// 本页 username 集合签名与上页相同 → 停止，避免重复累加与无谓网络请求。
		sig := inviteeRowsSig(rows)
		if sig == prevSig {
			break
		}
		prevSig = sig
		out = append(out, rows...)
	}
	// 排除"自己"：invite.php?menu=invitee 是"我邀请的人"，列表里若混入邀请人自己
	//（页面顶栏用户信息块被误抓，其 userdetails id == 当前 uid），剔除。
	if uid != "" && len(out) > 0 {
		filtered := make([]site.InviteeInfo, 0, len(out))
		for _, iv := range out {
			if iv.UserID != "" && iv.UserID == uid {
				continue
			}
			filtered = append(filtered, iv)
		}
		out = filtered
	}
	return out, nil
}

// inviteeRowsSig 生成一页被邀请人的去序签名（排序后的 username 集合），
// 用于 FetchInvitees 翻页无进展检测：两页签名相同即视为分页无效/循环。
func inviteeRowsSig(rows []site.InviteeInfo) string {
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, strings.ToLower(strings.TrimSpace(r.Username)))
	}
	sort.Strings(names)
	return strings.Join(names, "|")
}

// ── MTeam ──

// mtInvitesDivisor 馒头临时邀请固定价（魔力/个）。
const mtInvitesDivisor = 80000.0

// FetchInviteStatus 馒头：POST /api/member/profile。
func (m *MTeam) FetchInviteStatus(ctx context.Context, def *site.SiteMetadata,
	baseURL, token string) (*site.InviteStatus, error) {

	data, err := m.mtRequest(ctx, baseURL, token, "/api/member/profile", map[string]any{})
	if err != nil {
		return &site.InviteStatus{Reason: "API 请求失败"}, err
	}
	var prof struct {
		Invites      json.Number `json:"invites"`
		LimitInvites json.Number `json:"limitInvites"`
		Role         json.Number `json:"role"`
		MemberCount  struct {
			Bonus json.Number `json:"bonus"`
		} `json:"memberCount"`
	}
	if err := json.Unmarshal(data, &prof); err != nil {
		return &site.InviteStatus{Reason: "解析失败"}, nil
	}
	perm := toInt(jsonNumberString(prof.Invites))
	temp := toInt(jsonNumberString(prof.LimitInvites))
	role := toInt(jsonNumberString(prof.Role))
	bonus := toFloat(jsonNumberString(prof.MemberCount.Bonus))
	hasPerm := role >= 3
	buyable := 0
	if bonus > 0 {
		buyable = int(bonus / mtInvitesDivisor)
	}
	canInvite := hasPerm && (perm > 0 || temp > 0 || buyable > 0)
	reason := mtInviteReason(hasPerm, perm, temp, buyable, bonus)
	return &site.InviteStatus{
		CanInvite:            canInvite,
		Reason:               reason,
		PermanentCount:       perm,
		TemporaryCount:       temp,
		Bonus:                bonus,
		PermanentInvitePrice: -1,
		TemporaryInvitePrice: mtInvitesDivisor,
	}, nil
}

// FetchInvitees 馒头：取 uid → POST /api/invite/getUserInviteHistory?uid=。
func (m *MTeam) FetchInvitees(ctx context.Context, def *site.SiteMetadata,
	baseURL, token string) ([]site.InviteeInfo, error) {

	profData, err := m.mtRequest(ctx, baseURL, token, "/api/member/profile", map[string]any{})
	if err != nil {
		return nil, err
	}
	var prof struct {
		ID json.Number `json:"id"`
	}
	if err := json.Unmarshal(profData, &prof); err != nil {
		return nil, nil
	}
	uid := jsonNumberString(prof.ID)
	if uid == "" {
		return nil, nil
	}
	data, err := m.mtRequest(ctx, baseURL, token, "/api/invite/getUserInviteHistory?uid="+uid, map[string]any{})
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, nil
	}
	out := make([]site.InviteeInfo, 0, len(list))
	for _, it := range list {
		raw := strings.ToLower(fmt.Sprint(it["status"]))
		status := raw
		switch strings.ToUpper(fmt.Sprint(it["status"])) {
		case "CONFIRMED":
			status = "confirmed"
		case "PENDING":
			status = "pending"
		case "DISABLED", "BANNED":
			status = "disabled"
		}
		out = append(out, site.InviteeInfo{
			Username:   getString(it, "username"),
			Email:      getString(it, "email"),
			Uploaded:   toInt64(it["uploaded"]),
			Downloaded: toInt64(it["downloaded"]),
			Ratio:      getString(it, "shareRate"),
			Status:     status,
			Enabled:    status != "disabled" && status != "banned",
		})
	}
	return out, nil
}

func mtInviteReason(hasPerm bool, perm, temp, buyable int, bonus float64) string {
	if !hasPerm {
		return "等级不足（需 Elite User 及以上）"
	}
	if perm > 0 || temp > 0 {
		if buyable > 0 {
			return fmt.Sprintf("永久=%d 临时=%d 可购买%d个", perm, temp, buyable)
		}
		return fmt.Sprintf("永久=%d 临时=%d", perm, temp)
	}
	if buyable > 0 {
		return fmt.Sprintf("无名额，魔力%.0f 可购买%d个临时邀请", bonus, buyable)
	}
	return "无可用邀请名额且魔力不足"
}

// jsonNumberString 把 json.Number（或任意值）安全转字符串。
func jsonNumberString(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case json.Number:
		return t.String()
	case string:
		return t
	default:
		return fmt.Sprint(v)
	}
}

func toInt(s string) int {
	n := 0
	neg := false
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r == '-' && i == 0:
			neg = true
		case r >= '0' && r <= '9':
			n = n*10 + int(r-'0')
		}
	}
	if neg {
		return -n
	}
	return n
}

func toFloat(s string) float64 {
	f := 0.0
	neg := false
	seenDot := false
	div := 1.0
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r == '-' && i == 0:
			neg = true
		case r >= '0' && r <= '9':
			if seenDot {
				div *= 10
				f += float64(r-'0') / div
			} else {
				f = f*10 + float64(r-'0')
			}
		case r == '.' && !seenDot:
			seenDot = true
		}
	}
	if neg {
		return -f
	}
	return f
}
