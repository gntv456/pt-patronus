// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"fmt"
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

// mtEnvelope 是 MTeam 统一响应信封：Code 兼容字符串 "0" 与数字 0。
type mtEnvelope struct {
	Code    json.Number     `json:"code"`
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
	// 合并站点定义的额外参数；跳过两类键：mode（已单独解析，避免把上面的映射又冲回
	// normal）、web_detail（适配器内部用：搜索结果 DetailURL 的 web 前缀，非站点请求参数，
	// 误入请求体会被 M-Team 兼容面按未知字段拒绝）。
	for k, v := range def.ExtraParam {
		if k == "mode" || k == "web_detail" {
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
		out = append(out, parseMTTorrent(it, def.ExtraParam["web_detail"]))
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
