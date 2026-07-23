package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

// 朱雀（zhuque.in，schema=tnode）是 SPA + JSON-API 站点。公告与私信走 JSON 接口；
// 除 /api/plugins/ptppUserInfo 外，所有接口的 GET 均要求 x-csrf-token 请求头（从首页
// /index 的 <meta name="x-csrf-token" content="..."> 提取）。时间戳（joinTime /
// create_time 等）均为秒级。
//
// 端点契约（GET + csrf）：
//
//	/api/announcement/list                          → data:[{id,title,content,create_time,update_time,enabled}]
//	/api/message/listSystem?page=1&size=100         → data:{messages:[{id,user_id,title,content,read,create_time}],total}
//	/api/message/listInbox ?page=1&size=100         → 同上
//	/api/message/listOutbox?page=1&size=100         → 同上
//	/api/message/listAdmin ?type=0&page=1&size=100  → 同上
//	/api/message/getSystem?id=<id> / get?id= / getAdmin?id=  → 详情
//
// 本文件为 *GenericJSON 实现 AnnouncementCapable 与 MessageCapable，仅对 def.ID=="zhuque"
// 生效；其余 tnode 站返回空（chain 的 !Enabled 守卫已先行拦截）。

var zhuqueCSRFRe = regexp.MustCompile(`<meta\s+name="x-csrf-token"\s+content="([^"]+)"`)

// zhuqueCST 朱雀站点时区（timezone_offset=+0800）。
var zhuqueCST = time.FixedZone("CST", 8*3600)

// zhuqueCSRFToken GET /index 提取 csrf（仅配合会话 cookie 用于本次抓取，不入库不外泄）。
func (g *GenericJSON) zhuqueCSRFToken(ctx context.Context, baseURL, cookie string) (string, error) {
	resp, err := g.hc.Get(ctx, joinURL(baseURL, "index"), cookie, true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", site.ErrNotLoggedIn
	}
	if m := zhuqueCSRFRe.FindSubmatch(resp.Body); len(m) >= 2 {
		return string(m[1]), nil
	}
	return "", fmt.Errorf("zhuque: csrf token not found on /index")
}

// zhuqueFmtTime 秒级时间戳 → 「2006-01-02 15:04」（站点时区 +0800）。
func zhuqueFmtTime(sec int64) string {
	if sec <= 0 {
		return ""
	}
	return time.Unix(sec, 0).In(zhuqueCST).Format("2006-01-02 15:04")
}

// zhuqueGet 带 csrf 头的 JSON GET；401/403 → ErrNotLoggedIn。
func (g *GenericJSON) zhuqueGet(ctx context.Context, baseURL, cookie, path string) ([]byte, error) {
	csrf, err := g.zhuqueCSRFToken(ctx, baseURL, cookie)
	if err != nil {
		return nil, err
	}
	resp, err := g.hc.GetWithHeaders(ctx, joinURL(baseURL, path), cookie, map[string]string{
		"x-csrf-token": csrf,
		"Accept":       "application/json",
	}, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	return resp.Body, nil
}

// FetchAnnouncement 实现 site.AnnouncementCapable。
func (g *GenericJSON) FetchAnnouncement(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.SiteAnnouncement, error) {

	if def.ID == "sunnypt" {
		return g.sunnyptFetchAnnouncement(ctx, def, baseURL, cookie)
	}
	if def.ID != "zhuque" {
		return &site.SiteAnnouncement{Items: []site.AnnouncementItem{}}, nil
	}
	body, err := g.zhuqueGet(ctx, baseURL, cookie, "/api/announcement/list")
	if err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now(), Error: err.Error()}, nil
	}
	var wrap struct {
		Data []struct {
			ID         int64  `json:"id"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			CreateTime int64  `json:"create_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &site.SiteAnnouncement{FetchedAt: time.Now(), Error: err.Error()}, nil
	}
	items := make([]site.AnnouncementItem, 0, len(wrap.Data))
	for _, it := range wrap.Data {
		items = append(items, site.AnnouncementItem{
			Title:   it.Title,
			Date:    zhuqueFmtTime(it.CreateTime),
			Content: it.Content,
		})
	}
	return &site.SiteAnnouncement{Items: items, FetchedAt: time.Now()}, nil
}

// FetchMessages 实现 site.MessageCapable。朱雀邮箱映射：
//
//	MailboxSystem → listSystem / MailboxInbox → listInbox /
//	MailboxSent → listOutbox / MailboxStaff → listAdmin(type=0)
//
// cheater/reports 朱雀无对应端点 → 空列表。
func (g *GenericJSON) FetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, mailbox site.MailboxType) ([]site.Message, error) {

	if def.ID == "sunnypt" {
		return g.sunnyptFetchMessages(ctx, def, baseURL, cookie, mailbox)
	}
	if def.ID != "zhuque" {
		return []site.Message{}, nil
	}
	box, path, ok := zhuqueMailbox(mailbox)
	if !ok {
		return []site.Message{}, nil
	}
	body, err := g.zhuqueGet(ctx, baseURL, cookie, path)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Data struct {
			Messages []struct {
				ID         int64  `json:"id"`
				Title      string `json:"title"`
				Content    string `json:"content"`
				Read       int    `json:"read"`
				CreateTime int64  `json:"create_time"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, err
	}
	out := make([]site.Message, 0, len(wrap.Data.Messages))
	for _, m := range wrap.Data.Messages {
		out = append(out, site.Message{
			ID:           uint64(m.ID),
			Sender:       zhuqueSender(mailbox),
			Title:        m.Title,
			Content:      m.Content,
			Date:         zhuqueFmtTime(m.CreateTime),
			URL:          fmt.Sprintf("/message/%s?id=%d", box, m.ID),
			IsRead:       m.Read != 0,
			AdminActions: &site.AdminActions{ItemID: strconv.FormatInt(m.ID, 10)},
		})
	}
	return out, nil
}

// zhuquePost 带 csrf 头的 JSON POST；401/403 → ErrNotLoggedIn。
func (g *GenericJSON) zhuquePost(ctx context.Context, baseURL, cookie, path string, body []byte) ([]byte, error) {
	csrf, err := g.zhuqueCSRFToken(ctx, baseURL, cookie)
	if err != nil {
		return nil, err
	}
	resp, err := g.hc.Do(ctx, http.MethodPost, joinURL(baseURL, path), body, map[string]string{
		"x-csrf-token": csrf,
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}, cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	return resp.Body, nil
}

// FetchMessageContent 调详情端点取正文。朱雀 get/getSystem/getAdmin 为 GET + query（F 风格），
// 且服务端有「标记已读」副作用——打开详情即标记该条已读（实测 getSystem 验证 read 0→1）。
func (g *GenericJSON) FetchMessageContent(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL string) (string, error) {

	if def.ID == "sunnypt" {
		return g.sunnyptFetchMessageContent(ctx, def, baseURL, cookie, msgURL)
	}
	if def.ID != "zhuque" {
		return "", nil
	}
	box, id, ok := zhuqueParseMsgURL(msgURL)
	if !ok {
		return "", nil
	}
	q := url.Values{}
	q.Set("id", id)
	endpoint := "/api/message/get"
	switch box {
	case "system":
		endpoint = "/api/message/getSystem"
	case "admin":
		endpoint = "/api/message/getAdmin"
		q.Set("type", "0") // getAdmin 需 type（0=原消息；listAdmin 亦用 type=0）
	}
	body, err := g.zhuqueGet(ctx, baseURL, cookie, endpoint+"?"+q.Encode())
	if err != nil {
		return "", nil
	}
	var wrap struct {
		Data map[string]any `json:"data"`
	}
	if json.Unmarshal(body, &wrap) == nil && wrap.Data != nil {
		for _, k := range []string{"content", "body", "message", "text"} {
			if v, ok := wrap.Data[k].(string); ok && v != "" {
				return v, nil
			}
		}
	}
	return "", nil
}

// ReplyMessage 仅管理员对话（admin 邮箱）可回复：POST /api/message/replyAdmin {content,id,type:0}。
// 普通私信（inbox/outbox/system）朱雀无回复端点 → 优雅失败（前端禁用回复即可）。
func (g *GenericJSON) ReplyMessage(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL, content string) (*site.MessageReplyResult, error) {

	if def.ID == "sunnypt" {
		return &site.MessageReplyResult{Success: false, Message: "sunnypt 暂不支持站内回复（请到站点聊天页操作）"}, nil
	}
	if def.ID != "zhuque" {
		return &site.MessageReplyResult{Success: false, Message: "不支持"}, nil
	}
	box, id, ok := zhuqueParseMsgURL(msgURL)
	if !ok || box != "admin" {
		return &site.MessageReplyResult{Success: false, Message: "朱雀该邮箱暂不支持回复（仅管理员对话可回复）"}, nil
	}
	body, _ := json.Marshal(map[string]any{"content": content, "id": zhuqueAtoi(id), "type": 0})
	resp, err := g.zhuquePost(ctx, baseURL, cookie, "/api/message/replyAdmin", body)
	if err != nil {
		return &site.MessageReplyResult{Success: false, Message: "网络异常: " + err.Error()}, nil
	}
	var wrap struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
	}
	if json.Unmarshal(resp, &wrap) == nil {
		if wrap.Status == 401 || wrap.Status == 403 {
			return &site.MessageReplyResult{Success: false, Message: "未登录"}, site.ErrNotLoggedIn
		}
		if wrap.Status != 0 && wrap.Status != 200 {
			return &site.MessageReplyResult{Success: false, Message: fmt.Sprintf("回复失败 status=%d", wrap.Status)}, nil
		}
	}
	return &site.MessageReplyResult{Success: true, Message: "已回复"}, nil
}

// DeleteMessage GET /api/message/delete（inbox/outbox/admin）或 deleteSystem（system）。
// 朱雀 query 参数 ids 是 JSON 数组字符串 [id]（G 序列化），单条也带括号。
func (g *GenericJSON) DeleteMessage(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, deleteURL string) error {

	if def.ID == "sunnypt" {
		return fmt.Errorf("sunnypt 暂不支持删除消息")
	}
	if def.ID != "zhuque" {
		return nil
	}
	box, id, ok := zhuqueParseMsgURL(deleteURL)
	if !ok {
		return fmt.Errorf("zhuque: 无法解析消息地址 %s", deleteURL)
	}
	endpoint := "/api/message/delete"
	if box == "system" {
		endpoint = "/api/message/deleteSystem"
	}
	q := url.Values{}
	q.Set("ids", "["+id+"]") // 朱雀格式 ids=[id]
	body, err := g.zhuqueGet(ctx, baseURL, cookie, endpoint+"?"+q.Encode())
	if err != nil {
		return err
	}
	var wrap struct {
		Status int `json:"status"`
	}
	if json.Unmarshal(body, &wrap) == nil && wrap.Status != 0 && wrap.Status != 200 {
		return fmt.Errorf("zhuque: 删除失败 status=%d", wrap.Status)
	}
	return nil
}

// MarkRead 朱雀无单条标记已读 API：read 仅支持按邮箱全部标记 {all,type}，强行调用会误标整箱。
// 单条已读靠 get 详情副作用——FetchMessageContent 调 getSystem/get/getAdmin 时服务端已标记
// （实测验证）。前端 openDetail 先调 content（已标记）再调 markRead，此处 no-op 返回成功，
// 让前端 .then(load) 正常刷新列表即可展示已读态。
func (g *GenericJSON) MarkRead(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, itemIDs []string) (*site.BatchResult, error) {

	if def.ID == "sunnypt" {
		return g.sunnyptMarkRead(ctx, def, baseURL, cookie, itemIDs)
	}
	if def.ID != "zhuque" {
		return &site.BatchResult{Failed: 0}, nil
	}
	return &site.BatchResult{Success: len(itemIDs)}, nil
}

// zhuqueAtoi 字符串 id → int64（reply body 的 id 字段需数字）。
func zhuqueAtoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// zhuqueMailbox 把 PTPatronus 邮箱类型映射到朱雀 list 端点路径与前端 box 名。
func zhuqueMailbox(mb site.MailboxType) (box, path string, ok bool) {
	switch mb {
	case site.MailboxSystem:
		return "system", "/api/message/listSystem?page=1&size=100", true
	case site.MailboxInbox:
		return "inbox", "/api/message/listInbox?page=1&size=100", true
	case site.MailboxSent:
		return "outbox", "/api/message/listOutbox?page=1&size=100", true
	case site.MailboxStaff:
		return "admin", "/api/message/listAdmin?type=0&page=1&size=100", true
	}
	return "", "", false
}

func zhuqueSender(mb site.MailboxType) string {
	switch mb {
	case site.MailboxSystem:
		return "系统消息"
	case site.MailboxStaff:
		return "管理员"
	}
	return ""
}

var zhuqueMsgURLRe = regexp.MustCompile(`/message/(system|inbox|outbox|admin)\?id=(\d+)`)

func zhuqueParseMsgURL(msgURL string) (box, id string, ok bool) {
	m := zhuqueMsgURLRe.FindStringSubmatch(msgURL)
	if len(m) < 3 {
		return "", "", false
	}
	return m[1], m[2], true
}
