package site

import (
	"os"
	"testing"
)

// TestParseMessages table tr:has(td.rowfollow)，title 链接 + sender + date + unread + itemId。
func TestParseMessages(t *testing.T) {
	html := `<html><body><table>
		<tr class="colhead"><td class="rowfollow">状态</td><td class="rowfollow">主题</td><td class="rowfollow">发件人</td><td class="rowfollow">日期</td><td class="rowfollow">操作</td></tr>
		<tr>
			<td class="rowfollow"><img class="unreadpm" src="x"></td>
			<td class="rowfollow"><a href="viewmessage.php?id=42">欢迎加入</a></td>
			<td class="rowfollow">管理员</td>
			<td class="rowfollow">2026-01-01 12:00</td>
			<td class="rowfollow"><input name="id[]" value="42"><a href="messages.php?action=delete&id=42">删</a></td>
		</tr>
		<tr>
			<td class="rowfollow">读</td>
			<td class="rowfollow"><a href="viewmessage.php?id=7">系统通知</a></td>
			<td class="rowfollow">系统</td>
			<td class="rowfollow">3天前</td>
			<td class="rowfollow"><input name="id[]" value="7"></td>
		</tr>
	</table></body></html>`
	got := ParseMessages(docFromHTML(t, html), MailboxInbox)
	if len(got) != 2 {
		t.Fatalf("messages: got %d want 2 (header skipped)", len(got))
	}
	if got[0].Title != "欢迎加入" {
		t.Errorf("msg0 title: got %q", got[0].Title)
	}
	if got[0].IsRead {
		t.Error("msg0 should be unread")
	}
	if got[0].URL != "viewmessage.php?id=42" {
		t.Errorf("msg0 url: got %q", got[0].URL)
	}
	if got[0].AdminActions == nil || got[0].AdminActions.ItemID != "42" {
		t.Errorf("msg0 itemId: got %+v", got[0].AdminActions)
	}
	if got[1].IsRead != true {
		t.Error("msg1 should be read")
	}
}

// TestParseStaffboxMessages 好学 staffbox.php（管理组信箱）真实结构：
// 链接 staffbox.php?action=viewpm&pmid=、发讯者 <span.nowrap><a><b>、
// 时间 <span title="精确时间">、回复列含「是」、itemId 来自 setanswered[]。
func TestParseStaffboxMessages(t *testing.T) {
	data, err := os.ReadFile("testdata/hxpt_staffbox.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	got := ParseMessages(docFromHTML(t, string(data)), MailboxStaff)
	if len(got) != 2 {
		t.Fatalf("staffbox: got %d want 2 (header + footer op row skipped)", len(got))
	}
	if got[0].Title != "审核拒绝种子已更新" {
		t.Errorf("staff0 title: got %q", got[0].Title)
	}
	if got[0].URL != "https://www.hxpt.org/staffbox.php?action=viewpm&pmid=537&return=" {
		t.Errorf("staff0 url: got %q", got[0].URL)
	}
	if got[0].Sender != "ElJose" {
		t.Errorf("staff0 sender: got %q", got[0].Sender)
	}
	if got[0].Date != "2026-06-24 21:05:24" {
		t.Errorf("staff0 date (span title): got %q", got[0].Date)
	}
	if got[0].AdminActions == nil || got[0].AdminActions.ItemID != "537" {
		t.Errorf("staff0 itemId: got %+v", got[0].AdminActions)
	}
	if got[0].AdminActions.DeleteURL != "staffbox.php?action=delete&id=537" {
		t.Errorf("staff0 deleteUrl: got %q", got[0].AdminActions.DeleteURL)
	}
	if !got[0].IsRead { // 回复列非空 → 已回复 → 已读
		t.Error("staff0 should be read (answered)")
	}
	if got[1].Sender != "jiangbkvir" {
		t.Errorf("staff1 sender: got %q", got[1].Sender)
	}
	if got[1].Date != "2026-06-18 07:59:47" {
		t.Errorf("staff1 date: got %q", got[1].Date)
	}
}

// TestMailboxBoxParam 各邮箱 box 参数。
func TestMailboxBoxParam(t *testing.T) {
	cases := map[MailboxType]string{
		MailboxInbox:   "",
		MailboxSent:    "-1",
		MailboxSystem:  "-2",
		MailboxCheater: "-3",
		MailboxReports: "-5",
	}
	for mb, want := range cases {
		if got := MailboxBoxParam(mb); got != want {
			t.Errorf("box(%v): got %q want %q", mb, got, want)
		}
	}
}

// TestSynthesizeMsgID siteID*100000 + mailboxOrd*10000 + index。
func TestSynthesizeMsgID(t *testing.T) {
	if got := SynthesizeMsgID(5, MailboxInbox, 3); got != 500003 {
		t.Errorf("synthesize(inbox): got %d want 500003", got)
	}
	if got := SynthesizeMsgID(5, MailboxStaff, 1); got != 550001 { // staff ord=5
		t.Errorf("synthesize(staff): got %d want 550001", got)
	}
}

// TestParseMessageContent 正文提取（硬编码选择器 + 兜底）。
func TestParseMessageContent(t *testing.T) {
	html := `<table width="737"><tr><td colspan="2">这是消息正文内容，足够长以便被解析提取出来展示。</td></tr></table>`
	got := ParseMessageContent(docFromHTML(t, html))
	if got == "" {
		t.Error("content empty")
	}
}
