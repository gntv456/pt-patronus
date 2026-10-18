package oper

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// #318 RSS 订阅地址此前在写入面完全不校验 scheme（同包的 downloader.go / mediaserver.go
// 都调 validateURLScheme，唯独 rss 没调）。于是「忘了写 https://」这个最常见的笔误会被原样存进库：
// 列表里这条订阅看着正常（enabled、有间隔），只有到抓取那一刻才失败，
// 而失败文本是 chain 拼的「抓取 RSS 失败: Get "example.com/feed": unsupported protocol scheme ""」
// —— 英文尾巴进界面，cron 每轮再撞一次。写入边界拒掉才有人话可读。

func badURLs() []string {
	return []string{
		"example.com/feed",    // 忘写协议（真实用户最常犯的）
		"file:///etc/passwd",  // 非 http(s)
		"ftp://10.0.0.9/feed", // 非 http(s)
		"javascript:alert(1)", // 非 http(s)
		"https://",            // 有 scheme 无主机
		"http://[::1",         // 根本解析不了
	}
}

func TestRssCreateRejectsUnusableURL(t *testing.T) {
	for _, raw := range badURLs() {
		db := newRssDB(t)
		op := NewRssOper(db)
		err := op.Create(&models.RssSubscription{Name: "坏地址", URL: raw, IntervalMin: 30, Enabled: true})
		reason := InputReasonErr(err)
		if reason == nil {
			t.Errorf("URL %q 必须在写入面被拒并给出人话原因，实际 err=%v", raw, err)
			continue
		}
		// 关键后果断言：不许留下一条「看着正常但永远抓不动」的订阅
		var n int64
		if e := db.Model(&models.RssSubscription{}).Count(&n).Error; e != nil {
			t.Fatalf("计数失败：%v", e)
		}
		if n != 0 {
			t.Errorf("URL %q 被拒后库里不该有残留行，实际 %d 行", raw, n)
		}
		if reason.Error() == "" {
			t.Errorf("原因不能是空串（界面要说得出话）")
		}
	}
}

func TestRssUpdateRejectsUnusableURL(t *testing.T) {
	db := newRssDB(t)
	op := NewRssOper(db)
	sub := &models.RssSubscription{Name: "好源", URL: "https://rss.example/feed", IntervalMin: 30, Enabled: true}
	if err := op.Create(sub); err != nil {
		t.Fatalf("正控：合法 https 地址要写得进去，%v", err)
	}
	if sub.ID == 0 {
		t.Fatalf("正控：Create 要回填 ID")
	}
	before := sub.URL

	sub.URL = "example.com/feed"
	if e := op.Update(sub); InputReasonErr(e) == nil {
		t.Errorf("编辑成坏地址同样要被拒并给人话原因，实际 err=%v", e)
	}
	var got models.RssSubscription
	if e := db.First(&got, sub.ID).Error; e != nil {
		t.Fatalf("回读失败：%v", e)
	}
	if got.URL != before {
		t.Errorf("被拒的编辑不许落库：期望 %q，实际 %q", before, got.URL)
	}
}

// 正控：http(s) 完整地址照常读写（校验不能顺手把正常路径拦了）。
func TestRssWriteAcceptsValidURL(t *testing.T) {
	db := newRssDB(t)
	op := NewRssOper(db)
	for _, raw := range []string{"https://rss.example/feed", "http://10.0.0.9:8080/rss"} {
		if e := op.Create(&models.RssSubscription{Name: "正常源", URL: raw, IntervalMin: 30, Enabled: true}); e != nil {
			t.Errorf("合法地址 %q 不该被拦：%v", raw, e)
		}
	}
}
