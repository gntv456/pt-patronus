package schemas

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// zhuqueRecorder 记录 fake server 收到的写操作，供断言。
type zhuqueRecorder struct {
	csrf       string
	deletePath string // delete / deleteSystem
	deleteIDs  string // ids query 值
	replyBody  string // replyAdmin 请求体
}

// zhuqueFakeServer 模拟朱雀首页 csrf 注入 + 公告/消息 JSON 接口。
// 公告/消息路径必须带 x-csrf-token 头，否则 400（复刻真实站行为）。
func zhuqueFakeServer(t *testing.T, rec *zhuqueRecorder) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><head><meta name="x-csrf-token" content="test-csrf-xyz"></head><body>spa</body></html>`))
		case "/api/announcement/list":
			if r.Header.Get("x-csrf-token") != "test-csrf-xyz" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rec.csrf = r.Header.Get("x-csrf-token")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":[
				{"id":30,"title":"关于站点升级","content":"升级详情","create_time":1735287153,"update_time":1735287153,"enabled":1},
				{"id":29,"title":"新年公告","content":"新年好","create_time":1720000000,"enabled":1}
			]}`))
		case "/api/message/listSystem":
			if r.Header.Get("x-csrf-token") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":{"messages":[
				{"id":276536,"user_id":2792,"title":"系统通知","content":"通知正文","read":0,"create_time":1750133220},
				{"id":276535,"user_id":2792,"title":"已读通知","content":"ok","read":1,"create_time":1750000000}
			],"total":2}}`))
		case "/api/message/getSystem", "/api/message/get", "/api/message/getAdmin":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":{"content":"完整正文（详情）"}}`))
		case "/api/message/delete", "/api/message/deleteSystem":
			if r.Header.Get("x-csrf-token") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			rec.deletePath = strings.TrimPrefix(r.URL.Path, "/api/message/")
			rec.deleteIDs = r.URL.Query().Get("ids")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":null,"code":"success"}`))
		case "/api/message/replyAdmin":
			if r.Header.Get("x-csrf-token") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			bd, _ := io.ReadAll(r.Body)
			rec.replyBody = string(bd)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":200,"data":null,"code":"success"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestZhuqueFetchAnnouncement(t *testing.T) {
	rec := &zhuqueRecorder{}
	srv := zhuqueFakeServer(t, rec)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}

	ann, err := g.FetchAnnouncement(context.Background(), def, srv.URL, "socute=abc")
	if err != nil {
		t.Fatalf("FetchAnnouncement: %v", err)
	}
	if rec.csrf != "test-csrf-xyz" {
		t.Errorf("announcement 端点未收到 csrf 头, got=%q", rec.csrf)
	}
	if len(ann.Items) != 2 {
		t.Fatalf("want 2 items, got %d", len(ann.Items))
	}
	if ann.Items[0].Title != "关于站点升级" || ann.Items[0].Content != "升级详情" {
		t.Errorf("item0 解析错: %+v", ann.Items[0])
	}
	for _, it := range ann.Items {
		// create_time 是秒级 → 应格式化为 YYYY-MM-DD HH:MM（验证非空且格式正确，不绑死时区数值）
		if !regexpLike(it.Date) {
			t.Errorf("item date 格式错: %q", it.Date)
		}
	}
}

func TestZhuqueFetchMessages(t *testing.T) {
	srv := zhuqueFakeServer(t, &zhuqueRecorder{})
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}

	msgs, err := g.FetchMessages(context.Background(), def, srv.URL, "socute=abc", site.MailboxSystem)
	if err != nil {
		t.Fatalf("FetchMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	first := msgs[0]
	if first.Title != "系统通知" || first.Content != "通知正文" {
		t.Errorf("msg0 解析错: %+v", first)
	}
	if first.IsRead {
		t.Errorf("msg0 应未读, IsRead=true")
	}
	if first.Sender != "系统消息" {
		t.Errorf("system sender = %q", first.Sender)
	}
	if !strings.HasPrefix(first.URL, "/message/system?id=") {
		t.Errorf("msg URL = %q", first.URL)
	}
	if first.AdminActions == nil || first.AdminActions.ItemID != "276536" {
		t.Errorf("AdminActions.ItemID 解析错: %+v", first.AdminActions)
	}
	// 第二条 read=1 → IsRead=true
	if !msgs[1].IsRead {
		t.Errorf("msg1 应已读")
	}
}

func TestZhuqueFetchMessageContent(t *testing.T) {
	srv := zhuqueFakeServer(t, &zhuqueRecorder{})
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}

	content, err := g.FetchMessageContent(context.Background(), def, srv.URL, "socute=abc", "/message/system?id=276536")
	if err != nil {
		t.Fatalf("FetchMessageContent: %v", err)
	}
	if content != "完整正文（详情）" {
		t.Errorf("content = %q", content)
	}
	// 无法解析的 URL → 空串不报错
	if c, _ := g.FetchMessageContent(context.Background(), def, srv.URL, "", "/bad"); c != "" {
		t.Errorf("bad url should yield empty, got %q", c)
	}
}

func TestZhuqueDeleteMessage(t *testing.T) {
	rec := &zhuqueRecorder{}
	srv := zhuqueFakeServer(t, rec)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}

	// system → deleteSystem，ids=[id]
	if err := g.DeleteMessage(context.Background(), def, srv.URL, "", "/message/system?id=276536"); err != nil {
		t.Fatalf("delete system: %v", err)
	}
	if rec.deletePath != "deleteSystem" {
		t.Errorf("system 应命中 deleteSystem, got %q", rec.deletePath)
	}
	if rec.deleteIDs != "[276536]" {
		t.Errorf("ids 应为 [276536]（朱雀 JSON 数组串）, got %q", rec.deleteIDs)
	}
	// inbox → delete
	rec.deletePath, rec.deleteIDs = "", ""
	if err := g.DeleteMessage(context.Background(), def, srv.URL, "", "/message/inbox?id=100"); err != nil {
		t.Fatalf("delete inbox: %v", err)
	}
	if rec.deletePath != "delete" {
		t.Errorf("inbox 应命中 delete, got %q", rec.deletePath)
	}
	if rec.deleteIDs != "[100]" {
		t.Errorf("inbox ids 应为 [100], got %q", rec.deleteIDs)
	}
}

func TestZhuqueReplyMessage(t *testing.T) {
	rec := &zhuqueRecorder{}
	srv := zhuqueFakeServer(t, rec)
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}

	// admin → replyAdmin {content,id,type:0}
	r, err := g.ReplyMessage(context.Background(), def, srv.URL, "", "/message/admin?id=55", "你好")
	if err != nil || !r.Success {
		t.Fatalf("reply admin: %+v err=%v", r, err)
	}
	for _, want := range []string{`"content"`, `"id":55`, `"type":0`} {
		if !strings.Contains(rec.replyBody, want) {
			t.Errorf("replyAdmin body 缺 %s: %q", want, rec.replyBody)
		}
	}
	// inbox → 不支持（仅管理员对话可回复）
	r2, err := g.ReplyMessage(context.Background(), def, srv.URL, "", "/message/inbox?id=1", "x")
	if err != nil || r2.Success {
		t.Errorf("inbox 回复应失败: %+v err=%v", r2, err)
	}
}

func TestZhuqueMarkRead(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "zhuque", Schema: "tnode"}
	// no-op：朱雀单条已读靠 get 详情副作用（FetchMessageContent 触发），
	// 此处仅返回 Success 让前端 .then(load) 刷新；不发请求。
	r, err := g.MarkRead(context.Background(), def, "http://nonexistent.invalid", "", []string{"1", "2"})
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if r.Success != 2 {
		t.Errorf("MarkRead Success=%d want 2", r.Success)
	}
}

func TestZhuqueMailboxMapping(t *testing.T) {
	cases := []struct {
		mb      site.MailboxType
		wantBox string
		wantOk  bool
	}{
		{site.MailboxSystem, "system", true},
		{site.MailboxInbox, "inbox", true},
		{site.MailboxSent, "outbox", true},
		{site.MailboxStaff, "admin", true},
		{site.MailboxCheater, "", false},
		{site.MailboxReports, "", false},
	}
	for _, c := range cases {
		box, path, ok := zhuqueMailbox(c.mb)
		if ok != c.wantOk {
			t.Errorf("%s: ok=%v want %v", c.mb, ok, c.wantOk)
			continue
		}
		if !ok {
			continue
		}
		if box != c.wantBox {
			t.Errorf("%s: box=%q want %q", c.mb, box, c.wantBox)
		}
		if !strings.HasPrefix(path, "/api/message/") {
			t.Errorf("%s: path=%q", c.mb, path)
		}
	}
	// admin 必须带 type=0（否则 400 INVALID_TYPE）
	_, adminPath, _ := zhuqueMailbox(site.MailboxStaff)
	if !strings.Contains(adminPath, "type=0") {
		t.Errorf("admin path 缺 type=0: %s", adminPath)
	}
}

func TestZhuqueParseMsgURL(t *testing.T) {
	box, id, ok := zhuqueParseMsgURL("/message/system?id=276536")
	if !ok || box != "system" || id != "276536" {
		t.Errorf("parse 失败: box=%q id=%q ok=%v", box, id, ok)
	}
	if _, _, ok := zhuqueParseMsgURL("/other/abc"); ok {
		t.Errorf("不应解析非法 URL")
	}
}

// TestZhuqueNonZhuqueReturnsEmpty 非 zhuque 的 tnode 站 → 公告/消息空，不发请求。
func TestZhuqueNonZhuqueReturnsEmpty(t *testing.T) {
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericJSON(hc, "tnode")
	def := &site.SiteMetadata{ID: "othertnode", Schema: "tnode"}

	ann, err := g.FetchAnnouncement(context.Background(), def, "http://nonexistent.invalid", "")
	if err != nil || len(ann.Items) != 0 {
		t.Errorf("非朱雀公告应空: ann=%+v err=%v", ann, err)
	}
	msgs, err := g.FetchMessages(context.Background(), def, "http://nonexistent.invalid", "", site.MailboxSystem)
	if err != nil || len(msgs) != 0 {
		t.Errorf("非朱雀消息应空: msgs=%d err=%v", len(msgs), err)
	}
}

func TestZhuqueCSRFRegex(t *testing.T) {
	m := zhuqueCSRFRe.FindSubmatch([]byte(`<meta name="x-csrf-token" content="abc-123-xyz">`))
	if len(m) < 2 || string(m[1]) != "abc-123-xyz" {
		t.Errorf("csrf 正则提取失败: %v", m)
	}
}

// regexpLike 校验日期为 YYYY-MM-DD HH:MM。
func regexpLike(s string) bool {
	if len(s) != 16 {
		return false
	}
	return s[4] == '-' && s[7] == '-' && s[10] == ' ' && s[13] == ':'
}
