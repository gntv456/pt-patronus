package site

import (
	"testing"
)

// TestParseAnnouncements 现代皮肤 li.index-news-item：解析出标题/日期/正文三件套。
func TestParseAnnouncements(t *testing.T) {
	doc := loadFixture(t, "testdata/hxpt_announcement.html")
	a := ParseAnnouncements(doc)
	if a == nil {
		t.Fatal("nil announcement")
	}
	if len(a.Items) != 2 {
		t.Fatalf("items: got %d want 2", len(a.Items))
	}
	first := a.Items[0]
	if first.Title != "关于元旦全站免费的通知" {
		t.Errorf("item0 title: got %q want 关于元旦全站免费的通知", first.Title)
	}
	if first.Date != "2026-01-01" {
		t.Errorf("item0 date: got %q want 2026-01-01", first.Date)
	}
	if first.Content == "" {
		t.Error("item0 content empty")
	}
	if a.Items[1].Title != "服务器迁移公告" {
		t.Errorf("item1 title: got %q want 服务器迁移公告", a.Items[1].Title)
	}
}

// TestParseAnnouncements_Classic 经典皮肤 [id^=ka] 公告块 + 前导 <a> 标题。
func TestParseAnnouncements_Classic(t *testing.T) {
	html := `<div>
		<a name="ka0">公告标题</a>
		<div id="ka0">这是第一条公告正文内容，足够长以便被解析。</div>
	</div>`
	doc := docFromHTML(t, html)
	a := ParseAnnouncements(doc)
	if len(a.Items) == 0 {
		t.Fatal("classic: expected items, got 0")
	}
}

// TestAnnTitle 标题裁剪：去前缀 + 截断到标点/36 字。
func TestAnnTitle(t *testing.T) {
	if got := annTitle("", "公告：关于元旦活动安排的通知，请查收"); got != "关于元旦活动安排的通知" {
		t.Errorf("annTitle: got %q want 关于元旦活动安排的通知", got)
	}
}
