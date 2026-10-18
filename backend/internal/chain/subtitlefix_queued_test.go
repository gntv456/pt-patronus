package chain

import (
	"context"
	"testing"
)

// #300：queued 必须在「受理」这一刻定下来——界面靠它决定能不能说「已受理」。
// 目录型条目被整条剔除后可能一部都不剩：那种情况不起 goroutine、也就不会有
// 「字幕补齐完成」的通知；回执若仍写成受理成功，用户会去等一条永远不来的下文。
func TestFixMissingSubtitlesQueuedCountedAtAcceptance(t *testing.T) {
	ch, _, _, _ := newMediaServerTestChain(t)
	fixer := &fakeSubtitleFixer{}
	ch.SetSubtitleFixer(fixer)

	dirOnly := &HealthReport{Issues: []HealthIssue{
		{ServerID: 1, Type: "missing_subtitle", ItemID: "s1", Title: "Show", Path: `X:\库\Show (2008)`},
	}}
	out, err := ch.FixMissingSubtitles(context.Background(), dirOnly, "chs")
	if err != nil {
		t.Fatal(err)
	}
	if out.Queued != 0 {
		t.Fatalf("Queued=%d，期望 0（目录条目应整条剔除）", out.Queued)
	}
	if fixer.hits != 0 {
		t.Fatalf("字幕源被调用 %d 次，期望 0（一部都没排进队列就不该起后台任务）", fixer.hits)
	}

	mixed := &HealthReport{Issues: []HealthIssue{
		{ServerID: 1, Type: "missing_subtitle", ItemID: "m1", Title: "Movie", Path: `X:\库\Movie.2020\Movie.2020.1080p.mkv`},
		{ServerID: 1, Type: "missing_subtitle", ItemID: "s2", Title: "Show", Path: `X:\库\Show (2008)`},
		{ServerID: 1, Type: "unmatched", ItemID: "u1", Title: "Other", Path: `X:\库\Other.mkv`},
	}}
	out2, err := ch.FixMissingSubtitles(context.Background(), mixed, "chs")
	if err != nil {
		t.Fatal(err)
	}
	// 正控：计数只认「类型=缺字幕 且 路径是媒体文件」，非字幕类型与目录都不进账。
	if out2.Queued != 1 {
		t.Fatalf("Queued=%d，期望 1（两条里只有一条可用）", out2.Queued)
	}
	waitSubtitleFix(t, ch)
}
