package chain

import (
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// 自动下载失败的条目已回滚、下轮重试，cron 里只有一行 Warn。若不发通知，
// 界面恒显示「已检查、新增 N 条」而条目列表为空——失败被渲染成「没有内容」。
func TestRssDownloadFailNoticePayload(t *testing.T) {
	sub := &models.RssSubscription{ID: 51, Name: "电影源"}
	res := &RssCheckResult{New: 3, DownloadFailed: 2, DownloadErrors: []string{"下载器暂时连不上：请确认客户端已启动"}}
	in := rssDownloadFailNotice(sub, res)

	if in.Source != notice.SourceRssDownloadFail {
		t.Errorf("要用本域新枚举, got %q", in.Source)
	}
	if in.Source != "rss_download_fail" {
		t.Errorf("Source 串要和前端 notices.source 的键一致, got %q", in.Source)
	}
	if !strings.Contains(in.Title, "电影源") {
		t.Errorf("标题要点名订阅, got %q", in.Title)
	}
	if !strings.Contains(in.Body, "2 条未能推送到下载器") || !strings.Contains(in.Body, "下载器暂时连不上") {
		t.Errorf("正文要带失败条数与原因, got %q", in.Body)
	}
	if !strings.Contains(in.Body, "下轮会自动重试") {
		t.Errorf("要说明条目已回滚、会自动重试（否则用户以为种子丢了）, got %q", in.Body)
	}
	if in.Level != "warning" {
		t.Errorf("可重试的失败用 warning, got %q", in.Level)
	}
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(in.Key, "51") || !strings.Contains(in.Key, today) {
		t.Errorf("去重键要按「订阅+当天」（每轮重试不刷屏）, got %q", in.Key)
	}
	other := rssDownloadFailNotice(&models.RssSubscription{ID: 52, Name: "剧集源"}, res)
	if other.Key == in.Key {
		t.Errorf("不同订阅不许共用一个去重键")
	}
}
