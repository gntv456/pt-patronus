package schemas

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/site"
)

var sunnyNewsHeadRe = regexp.MustCompile(`(?m)^##\s*([0-9]{4}[./][0-9]{1,2}[./][0-9]{1,2})\s*[-–—]\s*(.+)$`)

func sunnyParseNewsMD(md string) []site.AnnouncementItem {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	idxs := sunnyNewsHeadRe.FindAllStringSubmatchIndex(md, -1)
	if len(idxs) == 0 {
		if strings.TrimSpace(md) == "" {
			return nil
		}
		return []site.AnnouncementItem{{Title: "站点公告", Content: strings.TrimSpace(md)}}
	}
	items := make([]site.AnnouncementItem, 0, len(idxs))
	for i, loc := range idxs {
		date := md[loc[2]:loc[3]]
		title := strings.TrimSpace(md[loc[4]:loc[5]])
		start := loc[1]
		end := len(md)
		if i+1 < len(idxs) {
			end = idxs[i+1][0]
		}
		body := strings.TrimSpace(md[start:end])
		date = strings.ReplaceAll(date, ".", "-")
		date = strings.ReplaceAll(date, "/", "-")
		items = append(items, site.AnnouncementItem{Title: title, Date: date, Content: body})
	}
	return items
}

func (g *GenericJSON) sunnyptFetchMessages(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, mailbox site.MailboxType) ([]site.Message, error) {

	apiBase := sunnyAPIBase(baseURL)
	switch mailbox {
	case site.MailboxSystem:
		return g.sunnyptListNotifications(ctx, apiBase, cookie)
	case site.MailboxInbox:
		return g.sunnyptListChatRooms(ctx, apiBase, cookie, "/api/v1/chat/private/list", site.MailboxInbox)
	case site.MailboxStaff:
		return g.sunnyptListChatRooms(ctx, apiBase, cookie, "/api/v1/chat/admin/list", site.MailboxStaff)
	default:
		return []site.Message{}, nil
	}
}

func (g *GenericJSON) sunnyptListNotifications(ctx context.Context, apiBase, cookie string) ([]site.Message, error) {
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, "/api/v1/notifications?limit=100&offset=0")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Code int `json:"code"`
		Data struct {
			List []struct {
				ID      any    `json:"id"`
				Title   string `json:"title"`
				Body    string `json:"body"`
				Type    string `json:"type"`
				ReadAt  any    `json:"read_at"`
				Created any    `json:"created_at"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("sunnypt: 解析通知: %w", err)
	}
	if wrap.Code != 0 {
		return []site.Message{}, nil
	}
	out := make([]site.Message, 0, len(wrap.Data.List))
	for _, it := range wrap.Data.List {
		id := sunnyAnyID(it.ID)
		out = append(out, site.Message{
			Sender:       "系统通知",
			Title:        it.Title,
			Content:      it.Body,
			Date:         sunnyAnyDate(it.Created),
			URL:          "/notifications?id=" + id,
			IsRead:       sunnyIsRead(it.ReadAt),
			Mailbox:      site.MailboxSystem,
			AdminActions: &site.AdminActions{ItemID: id},
			FetchedAt:    time.Now(),
		})
	}
	return out, nil
}

func (g *GenericJSON) sunnyptListChatRooms(ctx context.Context, apiBase, cookie, path string, mb site.MailboxType) ([]site.Message, error) {
	body, _, err := g.sunnyGet(ctx, apiBase, cookie, path)
	if err != nil {
		return nil, err
	}
	type room struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		State int    `json:"state"`
	}
	var rooms []room
	var wrap struct {
		Code int    `json:"code"`
		Data []room `json:"data"`
	}
	if json.Unmarshal(body, &wrap) == nil && wrap.Data != nil {
		rooms = wrap.Data
	} else {
		var wrap2 struct {
			Code int `json:"code"`
			Data struct {
				List []room `json:"list"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &wrap2) == nil {
			rooms = wrap2.Data.List
			wrap.Code = wrap2.Code
		}
	}
	if wrap.Code != 0 && len(rooms) == 0 {
		return []site.Message{}, nil
	}
	sender := "私信"
	if mb == site.MailboxStaff {
		sender = "管理员"
	}
	out := make([]site.Message, 0, len(rooms))
	for _, it := range rooms {
		out = append(out, site.Message{
			Sender:  sender,
			Title:   it.Title,
			URL:     "/chat?roomId=" + url.QueryEscape(it.ID),
			IsRead:  it.State != 0,
			Mailbox: mb,
			AdminActions: &site.AdminActions{
				ItemID:   it.ID,
				ReplyURL: "/chat?roomId=" + url.QueryEscape(it.ID),
			},
			FetchedAt: time.Now(),
		})
	}
	return out, nil
}

func (g *GenericJSON) sunnyptFetchMessageContent(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, msgURL string) (string, error) {

	apiBase := sunnyAPIBase(baseURL)
	if strings.Contains(msgURL, "notifications") {
		id := sunnyQueryID(msgURL, "id")
		if id == "" {
			return "", nil
		}
		msgs, err := g.sunnyptListNotifications(ctx, apiBase, cookie)
		if err != nil {
			return "", err
		}
		for _, m := range msgs {
			if m.AdminActions != nil && m.AdminActions.ItemID == id {
				return m.Content, nil
			}
		}
		return "", nil
	}
	roomID := sunnyQueryID(msgURL, "roomId")
	if roomID == "" {
		return "", nil
	}
	path := "/api/v1/chat/history?roomId=" + url.QueryEscape(roomID) + "&count=200"
	body, st, err := g.sunnyGet(ctx, apiBase, cookie, path)
	if err != nil {
		return "", err
	}
	data, err := sunnyData(body, st)
	if err != nil {
		return "", err
	}
	return strings.Join(sunnyChatHistoryLines(data), "\n"), nil
}

func sunnyChatHistoryLines(data json.RawMessage) []string {
	var arr []map[string]any
	if json.Unmarshal(data, &arr) != nil {
		var obj map[string]json.RawMessage
		if json.Unmarshal(data, &obj) == nil {
			for _, k := range []string{"messages", "list", "history", "items", "data"} {
				if v, ok := obj[k]; ok {
					_ = json.Unmarshal(v, &arr)
					if len(arr) > 0 {
						break
					}
				}
			}
		}
	}
	out := make([]string, 0, len(arr))
	for _, it := range arr {
		text := sunnyMapStr(it, "content", "text", "body", "message", "words")
		if text == "" {
			continue
		}
		who := sunnyMapStr(it, "username", "user_name", "sender", "from", "admin_name")
		if who != "" {
			out = append(out, who+": "+text)
		} else {
			out = append(out, text)
		}
	}
	return out
}

func (g *GenericJSON) sunnyptMarkRead(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string, itemIDs []string) (*site.BatchResult, error) {

	apiBase := sunnyAPIBase(baseURL)
	ok, fail := 0, 0
	for _, id := range itemIDs {
		if id == "" || strings.HasPrefix(id, "private_") {
			ok++
			continue
		}
		_, st, err := g.sunnyPost(ctx, apiBase, cookie, "/api/v1/notifications/"+url.PathEscape(id)+"/read", []byte("{}"))
		if err != nil || (st != 0 && st != 200) {
			fail++
			continue
		}
		ok++
	}
	return &site.BatchResult{Success: ok, Failed: fail}, nil
}

func sunnyAnyID(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case json.Number:
		return t.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func sunnyAnyDate(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t > 1e12 {
			return sunnyFmtDate(int64(t) / 1000)
		}
		if t > 1e9 {
			return sunnyFmtDate(int64(t))
		}
	}
	return ""
}

func sunnyIsRead(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	s := fmt.Sprint(v)
	return s != "" && s != "0" && s != "<nil>" && s != "null" && s != "false"
}

func sunnyQueryID(raw, key string) string {
	if i := strings.Index(raw, key+"="); i >= 0 {
		s := raw[i+len(key)+1:]
		if j := strings.IndexAny(s, "&#"); j >= 0 {
			s = s[:j]
		}
		s, _ = url.QueryUnescape(s)
		return s
	}
	return ""
}
