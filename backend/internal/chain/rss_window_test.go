package chain

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/mmcdole/gofeed"
)

// rssBigFeed 造 n 条目的假 feed：标题序号 = 条目在 feed 里的位置（便于断言窗口取到哪一段）。
//   - ascending=false 新在前（RSS 惯例），true 旧在前（不少站点/聚合器的实际顺序）
//   - withDates=false 时不带 pubDate（解析不出时间 → 只能按 feed 原序处理）
func rssBigFeed(n int, ascending, withDates bool) string {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>Big</title>`)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `<item><title>Item-%03d</title><link>https://site/details.php?id=%d</link><guid>big-%d</guid>`, i, i, i)
		if withDates {
			age := i
			if !ascending {
				age = n - 1 - i // 降序：排在第一位的是最新
			}
			fmt.Fprintf(&b, `<pubDate>%s</pubDate>`, base.Add(time.Duration(age)*time.Minute).Format(time.RFC1123Z))
		}
		b.WriteString(`</item>`)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

// TestRssCheckReportsTruncatedWindow feed 条目数超过单轮窗口时，账面必须把「只看了窗口」说出来：
// 只有 New 时，「源里共 250 条只扫了 200 条」和「整个源都扫过了」在接口与界面里同形。
func TestRssCheckReportsTruncatedWindow(t *testing.T) {
	const total = 250
	ch, itemOp, feedURL := newRssTestChain(t, rssBigFeed(total, false, true))
	sub := &models.RssSubscription{ID: 401, Name: "window", URL: feedURL}
	res, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Total != total || res.Scanned != RssCheckWindow || !res.Truncated {
		t.Fatalf("截断账面错误：total=%d scanned=%d truncated=%v（期望 %d/%d/true）",
			res.Total, res.Scanned, res.Truncated, total, RssCheckWindow)
	}
	if res.New != RssCheckWindow {
		t.Fatalf("窗口内应发现 %d 条，got %d", RssCheckWindow, res.New)
	}
	items, _ := itemOp.ListBySubscription(sub.ID, 1000)
	// created_at 只到秒，同批写入的先后不可靠 → 按集合断言「取到了哪一段」。
	got := titleSet(items)
	if len(items) != RssCheckWindow || !got["Item-000"] || got["Item-249"] {
		t.Fatalf("窗口应覆盖 feed 前 %d 条，got %d 条 Item-000=%v Item-249=%v",
			RssCheckWindow, len(items), got["Item-000"], got["Item-249"])
	}
}

// TestRssCheckUntruncatedAccount 未超窗口的正常源：truncated 必须为假且 scanned==total。
// 与上一条成对——截断标志恒真同样是说谎（界面天天喊「没扫全」）。
func TestRssCheckUntruncatedAccount(t *testing.T) {
	ch, _, feedURL := newRssTestChain(t, rssBigFeed(20, false, true))
	sub := &models.RssSubscription{ID: 402, Name: "small", URL: feedURL}
	res, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Total != 20 || res.Scanned != 20 || res.Truncated {
		t.Errorf("未截断却报截断：total=%d scanned=%d truncated=%v", res.Total, res.Scanned, res.Truncated)
	}
	if res.New != 20 {
		t.Errorf("20 条应全收，got %d", res.New)
	}
}

// TestRssCheckAscendingFeedReachesNewestItems 升序（旧→新）feed 里最新一条排在末尾：
// 按 feed 位置取前 200 条会让它永远落在窗口之外——开了自动下载也永不触发，
// 而每轮都说「没有新条目」。窗口必须从「最新」那一端数起。
func TestRssCheckAscendingFeedReachesNewestItems(t *testing.T) {
	const total = 250
	ch, itemOp, feedURL := newRssTestChain(t, rssBigFeed(total, true, true))
	sub := &models.RssSubscription{ID: 403, Name: "asc", URL: feedURL, TitleInclude: "Item-249"}
	res, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.New != 1 {
		t.Fatalf("升序 feed 的最新条目必须被检查到，got new=%d（截断=%v scanned=%d/%d）",
			res.New, res.Truncated, res.Scanned, res.Total)
	}
	items, _ := itemOp.ListBySubscription(sub.ID, 10)
	if len(items) != 1 || items[0].Title != "Item-249" {
		t.Fatalf("应入库最新那一条，got %+v", items)
	}
}

// TestRssCheckNoPubDateKeepsFeedOrder 缺 pubDate 时判不出方向，保守沿用 feed 原序
// （不改变既有订阅的取序），但截断账面仍要如实上报。
func TestRssCheckNoPubDateKeepsFeedOrder(t *testing.T) {
	const total = 250
	ch, itemOp, feedURL := newRssTestChain(t, rssBigFeed(total, false, false))
	sub := &models.RssSubscription{ID: 404, Name: "nodate", URL: feedURL}
	res, err := ch.Check(context.Background(), sub)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Scanned != RssCheckWindow || !res.Truncated {
		t.Fatalf("无 pubDate 也应报截断：scanned=%d truncated=%v", res.Scanned, res.Truncated)
	}
	items, _ := itemOp.ListBySubscription(sub.ID, 1000)
	got := titleSet(items)
	if len(items) != RssCheckWindow || !got["Item-000"] || got["Item-249"] {
		t.Fatalf("无 pubDate 时应保持 feed 原序（前 %d 条），got %d 条 Item-000=%v Item-249=%v",
			RssCheckWindow, len(items), got["Item-000"], got["Item-249"])
	}
}

// titleSet created_at 只到秒，同批写入的行顺序不可靠，断言一律按集合做。
func titleSet(items []models.RssItem) map[string]bool {
	got := make(map[string]bool, len(items))
	for _, it := range items {
		got[it.Title] = true
	}
	return got
}

// TestRssFeedAscending 方向判定的纯函数清单（与链级用例分头覆盖各出口）。
func TestRssFeedAscending(t *testing.T) {
	item := func(pub string) *gofeed.Item {
		if pub == "" {
			return &gofeed.Item{}
		}
		ts, err := time.Parse(time.RFC1123Z, pub)
		if err != nil {
			t.Fatal(err)
		}
		return &gofeed.Item{PublishedParsed: &ts}
	}
	const newer = "Mon, 12 Oct 2026 10:00:00 +0000"
	const older = "Mon, 12 Oct 2026 09:00:00 +0000"

	cases := []struct {
		name  string
		items []*gofeed.Item
		want  bool
	}{
		{"末条更新=升序", []*gofeed.Item{item(older), item(newer)}, true},
		{"末条更旧=降序（惯例）", []*gofeed.Item{item(newer), item(older)}, false},
		{"同刻不算升序（不动原序）", []*gofeed.Item{item(newer), item(newer)}, false},
		{"首条缺时间=不动原序", []*gofeed.Item{item(""), item(newer)}, false},
		{"末条缺时间=不动原序", []*gofeed.Item{item(newer), item("")}, false},
		{"单条无从判定", []*gofeed.Item{item(newer)}, false},
		{"空 feed", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rssFeedAscending(tc.items); got != tc.want {
				t.Errorf("rssFeedAscending=%v want %v", got, tc.want)
			}
		})
	}
}
