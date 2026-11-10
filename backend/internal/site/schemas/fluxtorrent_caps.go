// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// ── 可选能力：全部走开放 API /open/me/*（只读，Token 鉴权）──

// fluxGet 是带 Token 鉴权的 GET + 信封解包（能力端点共用管道）。
func (f *FluxTorrent) fluxGet(ctx context.Context, baseURL, credRaw, path string) (json.RawMessage, error) {
	cred := parseFluxTorrentCredential(credRaw)
	if cred.Token == "" {
		return nil, fmt.Errorf("fluxtorrent api token required（网页端 我的 → API Token 签发，fxo_ 开头）")
	}
	resp, err := f.hc.GetWithHeaders(ctx, joinURL(baseURL, path), "", withFluxTorrentAuth(nil, cred.Token), true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, site.ErrNotLoggedIn
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fluxtorrent %s: status %d", path, resp.StatusCode)
	}
	var env fluxEnvelope
	if err := json.Unmarshal(resp.Body, &env); err != nil {
		return nil, fmt.Errorf("fluxtorrent %s: parse envelope: %w", path, err)
	}
	if env.Code != 0 {
		return nil, fluxBizErr(strings.TrimPrefix(path, "/api/v1/open/me/"), env)
	}
	return env.Data, nil
}

// fluxHRItem 是 /open/me/hr 的单条违约。
type fluxHRItem struct {
	TorrentID        int    `json:"torrent_id"`
	Name             string `json:"name"`
	SeededSeconds    int64  `json:"seeded_seconds"`
	RequiredSeconds  int64  `json:"required_seconds"`
	ShortfallSeconds int64  `json:"shortfall_seconds"`
	DetectedAt       int64  `json:"detected_at"`
	ResolvedAt       *int64 `json:"resolved_at"`
	Open             bool   `json:"open"`
}

// FetchHR 走 GET /open/me/hr?status=open（HRCapable）。
// 字段映射：Ratio 无来源留 0；SeedTime/Status/DeadlineLeft 以原始文本呈现
// （做种时长/所需时长与状态、剩余缺口），DetailURL 指向站内详情页。
func (f *FluxTorrent) FetchHR(ctx context.Context, def *site.SiteMetadata,
	baseURL, credRaw string) ([]site.HRTorrent, error) {
	data, err := f.fluxGet(ctx, baseURL, credRaw, "/api/v1/open/me/hr?status=open&limit=100")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []fluxHRItem `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("fluxtorrent hr: parse items: %w", err)
	}
	out := make([]site.HRTorrent, 0, len(payload.Items))
	for _, it := range payload.Items {
		out = append(out, site.HRTorrent{
			TorrentID:    strconv.Itoa(it.TorrentID),
			Title:        it.Name,
			SeedTime:     fmt.Sprintf("已做种 %s / 需 %s", fluxDuration(it.SeededSeconds), fluxDuration(it.RequiredSeconds)),
			Status:       fluxHRStatus(it),
			DeadlineLeft: fmt.Sprintf("差 %s", fluxDuration(it.ShortfallSeconds)),
			DetailURL:    joinURL(baseURL, fmt.Sprintf("/torrent/%d", it.TorrentID)),
		})
	}
	return out, nil
}

// fluxHRStatus 状态文本：未解决=监控中（差量>0）/已达标（差量=0），已解决=已豁免。
func fluxHRStatus(it fluxHRItem) string {
	if !it.Open {
		return "已解决"
	}
	if it.ShortfallSeconds > 0 {
		return "未达标"
	}
	return "达标"
}

// fluxDuration 秒数 → "X天Y时Z分" 人类可读（H&R 考核时长粒度到分钟足够）。
func fluxDuration(seconds int64) string {
	if seconds <= 0 {
		return "0分钟"
	}
	d := seconds / 86400
	h := seconds % 86400 / 3600
	m := seconds % 3600 / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%d天%d时%d分", d, h, m)
	case h > 0:
		return fmt.Sprintf("%d时%d分", h, m)
	default:
		return fmt.Sprintf("%d分钟", m)
	}
}

// fluxSeedingItem 是 /open/me/seeding 的单条做种。
type fluxSeedingItem struct {
	TorrentID int    `json:"torrent_id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
}

// FetchSeedingList 走 GET /open/me/seeding?limit=100（SeedingListCapable，保种契约逐条聚合）。
func (f *FluxTorrent) FetchSeedingList(ctx context.Context, def *site.SiteMetadata,
	baseURL, credRaw string) ([]site.SeedingItem, error) {
	data, err := f.fluxGet(ctx, baseURL, credRaw, "/api/v1/open/me/seeding?limit=100")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []fluxSeedingItem `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("fluxtorrent seeding: parse items: %w", err)
	}
	out := make([]site.SeedingItem, 0, len(payload.Items))
	for _, it := range payload.Items {
		out = append(out, site.SeedingItem{Title: it.Name, Size: it.Size})
	}
	return out, nil
}

// fluxMessageItem 是 /open/me/messages 的单条站内信。
type fluxMessageItem struct {
	ID        int    `json:"id"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
	ReadAt    *int64 `json:"read_at"`
	Unread    bool   `json:"unread"`
}

// FetchMessages 走 GET /open/me/messages（MessageCapable，只读——仅收件箱有效，
// 其余信箱返回空）。FluxTorrent 无消息详情页（body 随列表返回），FetchMessageContent
// 直接回 body；写操作（回复/删除/标记已读）开放 API 未开放，graceful 报不支持。
func (f *FluxTorrent) FetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, credRaw string, mailbox site.MailboxType) ([]site.Message, error) {
	if mailbox != site.MailboxInbox && mailbox != site.MailboxSystem {
		return nil, nil
	}
	data, err := f.fluxGet(ctx, baseURL, credRaw, "/api/v1/open/me/messages?limit=50")
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []fluxMessageItem `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("fluxtorrent messages: parse items: %w", err)
	}
	out := make([]site.Message, 0, len(payload.Items))
	for _, it := range payload.Items {
		out = append(out, site.Message{
			ID:      uint64(it.ID),
			Sender:  it.From,
			Title:   it.Subject,
			Content: it.Body,
			Date:    time.Unix(it.CreatedAt, 0).Format("2006-01-02 15:04"),
			IsRead:  !it.Unread,
		})
	}
	return out, nil
}

// FetchMessageContent：body 已随列表返回，无需二次抓取（回空由前端展示列表 title/content）。
func (f *FluxTorrent) FetchMessageContent(context.Context, *site.SiteMetadata, string, string, string) (string, error) {
	return "", nil
}

// ReplyMessage / DeleteMessage / MarkRead：FluxTorrent 开放 API 站内信只读，graceful 不支持。
func (f *FluxTorrent) ReplyMessage(context.Context, *site.SiteMetadata, string, string, string, string) (*site.MessageReplyResult, error) {
	return &site.MessageReplyResult{Success: false, Message: "FluxTorrent 开放 API 站内信仅只读，不支持回复"}, nil
}

func (f *FluxTorrent) DeleteMessage(context.Context, *site.SiteMetadata, string, string, string) error {
	return fmt.Errorf("FluxTorrent 开放 API 站内信仅只读，不支持删除")
}

func (f *FluxTorrent) MarkRead(context.Context, *site.SiteMetadata, string, string, []string) (*site.BatchResult, error) {
	return &site.BatchResult{Failed: 0}, fmt.Errorf("FluxTorrent 开放 API 站内信仅只读，不支持远程标记已读")
}
