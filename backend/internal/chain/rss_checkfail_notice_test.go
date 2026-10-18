package chain

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// #319 RSS 订阅在 cron 里每轮抓取失败时，此前只有一行 slog.Warn：
// 失败分支照样调 MarkChecked 推进 last_checked_at，于是列表里这条订阅始终「已检查、
// 新条目 0」，用户学到的是「这个源就是没东西」，而不是「一直抓不到」。
// 手动点「检查」有 200+{ok:false,error:中文} 这条通道，cron 没有 ⇒ 必须自曝。

func TestRssCheckFailNoticePayload(t *testing.T) {
	sub := &models.RssSubscription{ID: 42, Name: "电影源"}
	in := rssCheckFailNotice(sub, errors.New(`抓取 RSS 失败: Get "https://x/feed": dial tcp: refused`))

	if in.Source != notice.SourceRssCheckFail {
		t.Errorf("要用本域 Source（前端据此给图标/文案），实际 %q", in.Source)
	}
	if in.Source != "rss_check_fail" {
		t.Errorf("Source 串要和前端 notices.source 的键一致，实际 %q", in.Source)
	}
	if !strings.Contains(in.Title, "电影源") {
		t.Errorf("标题要点名是哪个订阅（多个源同时坏时靠这个区分），实际 %q", in.Title)
	}
	if !strings.Contains(in.Body, "抓取 RSS 失败") {
		t.Errorf("正文要带后端那句原因（英文尾巴也在里面，供排障），实际 %q", in.Body)
	}
	// 走文案单一来源：可归类的传输失败必须在通知里就翻成人话（真机第一轮暴露这里贴的是原文）
	transport := rssCheckFailNotice(sub, errors.New(`抓取 RSS 失败: Get "http://127.0.0.1:9/feed": dial tcp: connectex: No connection could be made`))
	if !strings.Contains(transport.Body, "连不上") || strings.Contains(transport.Body, "connectex") {
		t.Errorf("通知正文要走 errtext 翻成人话，实际 %q", transport.Body)
	}
	if in.Level != "warning" {
		t.Errorf("每轮都坏但不是全站故障，用 warning，实际 %q", in.Level)
	}
	// 去重键必须同时含订阅号与当天日期：同一天同一源只吵一次，不同源各自分开。
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(in.Key, "42") || !strings.Contains(in.Key, today) {
		t.Errorf("Key 要按「订阅+当天」去重，实际 %q", in.Key)
	}
	other := rssCheckFailNotice(&models.RssSubscription{ID: 43, Name: "剧集源"}, errors.New("解析 RSS 失败: x"))
	if other.Key == in.Key {
		t.Errorf("两个不同订阅的失败不许共用一个去重键（后者会被前者的去重吞掉）")
	}
}

// 接线证明：CheckAll 的失败分支必须既有日志也有通知（只留日志＝没吭声）。
func TestRssCheckAllNotifiesOnFailure(t *testing.T) {
	src, err := os.ReadFile("rss.go")
	if err != nil {
		t.Fatalf("读不到 rss.go（go test 工作目录是包目录）：%v", err)
	}
	code := stripRSSComments(string(src))
	i := strings.Index(code, "func (c *RssChain) CheckAll")
	if i < 0 {
		t.Fatalf("找不到 CheckAll")
	}
	j := strings.Index(code[i+1:], "\nfunc ")
	body := code[i : i+1+j]
	if !strings.Contains(body, `slog.Warn("rss check failed"`) {
		t.Errorf("失败要留日志（这条不能因为加了通知就被删）")
	}
	if !strings.Contains(body, "c.notice.Notice(sctx, rssCheckFailNotice(sub, err), true)") {
		t.Errorf("CheckAll 必须把失败自曝成站内通知；只 Warn 的话界面上这条订阅永远显示「已检查」")
	}
	if !strings.Contains(body, "if c.notice != nil") {
		t.Errorf("notice 可空（测试未装配），必须判空后再发")
	}
}

// stripRSSComments 剥掉整行 // 注释：我自己的补丁注释里就写着这些串，不剥会造假红。
func stripRSSComments(s string) string {
	out := make([]string, 0, 96)
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
