// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"github.com/hxpt/ptpatronus/internal/site"
)

// FetchMessages 实现 site.MessageCapable：抓取私信列表。
func (n *NexusPHP) FetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, mailbox site.MailboxType) ([]site.Message, error) {

	def = site.EnsureDefaults(def)
	listPath := def.Messages.ListPath
	if listPath == "" {
		listPath = "messages.php"
	}
	u := listPath
	if mailbox == site.MailboxStaff {
		u = "staffbox.php"
	} else if box := site.MailboxBoxParam(mailbox); box != "" {
		u = listPath + "&box=" + box
		if !strings.Contains(listPath, "?") {
			u = listPath + "?box=" + box
		}
	}
	doc, resp, err := n.fetchDoc(ctx, baseURL, u, cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseMessages(doc, mailbox), nil
}

// FetchMessageContent 抓取消息正文。
func (n *NexusPHP) FetchMessageContent(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL string) (string, error) {

	doc, _, err := n.fetchDoc(ctx, baseURL, msgURL, cookie)
	if err != nil {
		return "", err
	}
	return site.ParseMessageContent(doc), nil
}

// ReplyMessage 回复消息（CSRF 可选：def.Messages.CSRFField 为空则不带 token）。
func (n *NexusPHP) ReplyMessage(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL, content string) (*site.MessageReplyResult, error) {

	def = site.EnsureDefaults(def)
	form := url.Values{}
	form.Set("reply", content)
	if csrf := def.Messages.CSRFField; csrf != "" {
		token, err := n.fetchCSRFToken(ctx, baseURL, cookie, msgURL, csrf)
		if err != nil {
			return &site.MessageReplyResult{Success: false, Message: err.Error()}, nil
		}
		form.Set(csrf, token)
	}
	resp, err := n.hc.PostForm(ctx, absMsgURL(baseURL, msgURL), cookie, form.Encode(), true)
	if err != nil {
		return &site.MessageReplyResult{Success: false, Message: "网络异常: " + err.Error()}, nil
	}
	body := string(resp.Body)
	if strings.Contains(body, "成功") || strings.Contains(body, "sent") {
		return &site.MessageReplyResult{Success: true, Message: "已回复"}, nil
	}
	return &site.MessageReplyResult{Success: false, Message: "回复失败"}, nil
}

// DeleteMessage 删除消息（空 body POST）。
func (n *NexusPHP) DeleteMessage(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, deleteURL string) error {

	resp, err := n.hc.PostForm(ctx, absMsgURL(baseURL, deleteURL), cookie, "", true)
	if err != nil {
		return err
	}
	body := string(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.TrimSpace(body) == "" {
		return nil
	}
	if strings.Contains(body, "成功") || strings.Contains(body, "deleted") || strings.Contains(body, "删除") {
		return nil
	}
	if strings.Contains(body, "没有权限") || strings.Contains(body, "permission") || strings.Contains(body, "denied") {
		return fmt.Errorf("无权限操作")
	}
	return fmt.Errorf("删除失败(HTTP %d)", resp.StatusCode)
}

// MarkRead 标记已读（批量 ids，空 body POST）。按 HTTP 响应判定成败而非盲目乐观：
//   - 命中未登录（重定向到登录页 / 状态码命中）→ ErrNotLoggedIn，避免把登录页当成功；
//   - 2xx 且正文含成功标记 → 全部成功；
//   - 2xx 且正文为空（站点忽略 ids 参数）→ 逐条重试，每条按其响应状态码计成功/失败；
//   - 其余（非 2xx）→ 按失败计。
func (n *NexusPHP) MarkRead(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, itemIDs []string) (*site.BatchResult, error) {

	if len(itemIDs) == 0 {
		return &site.BatchResult{Failed: 0}, fmt.Errorf("无有效消息ID")
	}
	def = site.EnsureDefaults(def)
	u := "messages.php?action=markread&ids=" + url.QueryEscape(strings.Join(itemIDs, ","))
	resp, err := n.hc.PostForm(ctx, joinURL(baseURL, u), cookie, "", true)
	if err != nil {
		// 逐条结果面的原因要有中文读者（界面直接展示 errors[0]），别把 "context deadline exceeded" 弹给用户。
		return &site.BatchResult{Failed: len(itemIDs), Errors: []string{errtext.Text(err)}}, nil
	}
	if loggedOut(resp, nil, def.NoLogin) {
		return &site.BatchResult{Failed: len(itemIDs), Errors: []string{"未登录"}}, site.ErrNotLoggedIn
	}
	body := string(resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 &&
		(strings.Contains(body, "成功") || strings.Contains(body, "ok") || strings.Contains(body, "marked")) {
		return &site.BatchResult{Success: len(itemIDs)}, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.TrimSpace(body) == "" {
		// 站点可能忽略 ids 参数；逐条尝试（每条按响应判定成败）。
		return n.markReadOneByOne(ctx, baseURL, cookie, itemIDs), nil
	}
	return &site.BatchResult{Failed: len(itemIDs), Errors: []string{fmt.Sprintf("标记失败(HTTP %d)", resp.StatusCode)}}, nil
}

// markReadOneByOne 批量端点不可用时逐条标记；按每条响应状态码（2xx）计成功/失败，
// 而非盲目全部计成功。
func (n *NexusPHP) markReadOneByOne(ctx context.Context, baseURL, cookie string, ids []string) *site.BatchResult {

	r := &site.BatchResult{}
	for _, id := range ids {
		u := "messages.php?action=markread&id=" + url.QueryEscape(id)
		resp, err := n.hc.PostForm(ctx, joinURL(baseURL, u), cookie, "", true)
		if err != nil {
			r.Failed++
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %s", id, errtext.Text(err)))
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			r.Success++
		} else {
			r.Failed++
			r.Errors = append(r.Errors, fmt.Sprintf("%s: HTTP %d", id, resp.StatusCode))
		}
	}
	return r
}

// fetchCSRFToken 从详情页取 input[name=field] 的值。
func (n *NexusPHP) fetchCSRFToken(ctx context.Context, baseURL, cookie, msgURL, field string) (string, error) {
	resp, err := n.hc.Get(ctx, absMsgURL(baseURL, msgURL), cookie, true)
	if err != nil {
		return "", fmt.Errorf("无法获取CSRF令牌")
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return "", fmt.Errorf("无法获取CSRF令牌")
	}
	val, _ := doc.Find(`input[name="` + field + `"]`).First().Attr("value")
	if val == "" {
		return "", fmt.Errorf("无法获取CSRF令牌")
	}
	return val, nil
}

// absMsgURL 把可能相对的 href 解析为绝对 URL。
func absMsgURL(baseURL, href string) string {
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	return joinURL(baseURL, strings.TrimPrefix(href, "/"))
}

// ── MTeam：只读（通知列表 / 统计伪消息；写操作 graceful）──

// FetchMessages 馒头：通知列表 + 统计伪消息（仅 inbox/system 有效）。
func (m *MTeam) FetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, token string, mailbox site.MailboxType) ([]site.Message, error) {

	if mailbox != site.MailboxInbox && mailbox != site.MailboxSystem {
		return nil, nil
	}
	// 优先通知列表
	for _, ep := range []string{"/api/msg/notify/list", "/api/msg/notify/page", "/api/msg/notify"} {
		data, err := m.mtRequest(ctx, baseURL, token, ep, map[string]any{"pageNumber": 1, "pageSize": 50})
		if err == nil {
			if msgs := parseMTNotifications(data); len(msgs) > 0 {
				return msgs, nil
			}
		}
	}
	// 回退统计伪消息
	data, err := m.mtRequest(ctx, baseURL, token, "/api/msg/notify/statistic", map[string]any{})
	if err != nil {
		return nil, nil
	}
	return mtStatsPseudoMessage(data), nil
}

// FetchMessageContent 馒头无详情页。
func (m *MTeam) FetchMessageContent(context.Context, *site.SiteMetadata, string, string, string) (string, error) {
	return "", nil
}

// ReplyMessage 馒头只读。
func (m *MTeam) ReplyMessage(context.Context, *site.SiteMetadata, string, string, string, string) (*site.MessageReplyResult, error) {
	return &site.MessageReplyResult{Success: false, Message: "MTeam API 仅提供通知计数，不支持回复"}, nil
}

// DeleteMessage 馒头只读。
func (m *MTeam) DeleteMessage(context.Context, *site.SiteMetadata, string, string, string) error {
	return fmt.Errorf("MTeam API 仅提供通知计数，不支持远程删除")
}

// MarkRead 馒头只读。
func (m *MTeam) MarkRead(context.Context, *site.SiteMetadata, string, string, []string) (*site.BatchResult, error) {
	return &site.BatchResult{Failed: 0}, fmt.Errorf("MTeam API 仅提供通知计数，不支持远程标记已读")
}

// parseMTNotifications 从通知 JSON 数组解析（容忍多种字段名）。
func parseMTNotifications(data []byte) []site.Message {
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err != nil {
		// 可能包在 data 字段里
		var wrap struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(data, &wrap); err == nil && len(wrap.Data) > 0 {
			arr = wrap.Data
		}
	}
	out := make([]site.Message, 0, len(arr))
	for _, it := range arr {
		title := mtFirstStr(it, "title", "subject", "name")
		content := mtFirstStr(it, "content", "message", "msg", "body")
		if title == "" {
			title = truncateStr(content, 80)
		}
		out = append(out, site.Message{
			Sender:   mtFirstStr(it, "sender", "from", "author", "type"),
			Title:    title,
			Content:  content,
			Date:     mtFirstStr(it, "createdDate", "createdAt", "createTime", "sendTime", "time", "date"),
			URL:      "/msg/notify/" + fmt.Sprint(getAny(it, "id")),
			IsRead:   false,
			Mailbox:  site.MailboxSystem,
			Subtitle: mtFirstStr(it, "subtitle", "typeName", "category"),
		})
	}
	return out
}

// mtFirstStr 取首个非空字符串字段。
func mtFirstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// mtStatsPseudoMessage 从 /api/msg/notify/statistic 合成计数伪消息。
func mtStatsPseudoMessage(data []byte) []site.Message {
	var s struct {
		Count  json.Number `json:"count"`
		Unread json.Number `json:"unMake"`
	}
	_ = json.Unmarshal(data, &s)
	count := toInt(s.Count.String())
	if count == 0 {
		count = toInt(s.Unread.String())
	}
	if count <= 0 {
		return nil
	}
	return []site.Message{{
		Sender:  "M-Team",
		Title:   fmt.Sprintf("消息/通知统计 %d 条", count),
		Date:    "未读 " + s.Unread.String() + " / 总数 " + s.Count.String(),
		IsRead:  toInt(s.Unread.String()) == 0,
		Mailbox: site.MailboxSystem,
		Content: fmt.Sprintf("MTeam API 仅返回通知计数：未读 %s 条，总数 %s 条。", s.Unread.String(), s.Count.String()),
	}}
}

func truncateStr(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
