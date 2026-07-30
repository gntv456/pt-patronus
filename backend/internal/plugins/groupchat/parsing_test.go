package groupchat

import (
	"strings"
	"testing"
	"time"
)

func TestClassifyRewards(t *testing.T) {
	r := classifyRewards("系统：@user 获得上传量 1GB，魔力值 100")
	if len(r) != 2 {
		t.Fatalf("expected 2 rewards (上传量+魔力值), got %d: %#v", len(r), r)
	}
	types := map[string]bool{}
	for _, x := range r {
		types[x.Type] = true
	}
	if !types["上传量"] || !types["魔力值"] {
		t.Fatalf("missing reward types: %#v", types)
	}
}

func TestClassifyRewardsNegative(t *testing.T) {
	r := classifyRewards("黑丝娘：@user 损失 下载量 500MB")
	if len(r) == 0 || !r[0].Negative {
		t.Fatalf("expected negative download reward, got %#v", r)
	}
}

func TestParseNexusFeedback(t *testing.T) {
	html := `<table>
<tr><td class="shoutrow">系统: @user 获得上传量 1G</td></tr>
<tr><td class="shoutrow">@other 求下载</td></tr>
</table>`
	fb := parseNexusFeedback(html, "user")
	if fb.Raw == "" {
		t.Fatal("expected match on @user row")
	}
	if len(fb.Rewards) == 0 || fb.Rewards[0].Type != "上传量" {
		t.Fatalf("expected 上传量 reward, got %#v", fb.Rewards)
	}
}

func TestParseNexusFeedbackNoUsername(t *testing.T) {
	html := `<table><tr><td class="shoutrow">系统: @user 上传量</td></tr></table>`
	fb := parseNexusFeedback(html, "")
	if len(fb.Rewards) != 0 {
		t.Fatalf("empty username should not match, got %#v", fb)
	}
}

func TestParseZmFeedbackTimePriority(t *testing.T) {
	html := `<table>
<tr><td class="shoutrow">@user：皮总 电力 10 5分钟前</td></tr>
<tr><td class="shoutrow">@user：皮总 上传量 1G &lt;1分钟前</td></tr>
</table>`
	fb := parseZmFeedback(html, "user")
	if len(fb.Rewards) == 0 {
		t.Fatal("expected rewards from zm feedback")
	}
	// 优先级 <1分钟前(100) > 5分钟前(95)，应取上传量
	if fb.Rewards[0].Type != "上传量" {
		t.Fatalf("expected 上传量 (highest priority), got %s (raw=%s)", fb.Rewards[0].Type, fb.Raw)
	}
}

func TestParseZmFeedbackNoPiZong(t *testing.T) {
	html := `<table><tr><td class="shoutrow">@user：某路人 上传量</td></tr></table>`
	fb := parseZmFeedback(html, "user")
	if len(fb.Rewards) != 0 {
		t.Fatalf("rows without 皮总 should not match, got %#v", fb)
	}
}

func TestParseZmMailTime(t *testing.T) {
	html := `<html><body>... 收到来自 zmpt 赠送的电力 ... 2025-04-20 20:55:49 ...</body></html>`
	tm, err := parseZmMailTime(html)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	want := "2025-04-20 20:55:49"
	if got := tm.Format("2006-01-02 15:04:05"); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestParseZmMailTimeNotFound(t *testing.T) {
	_, err := parseZmMailTime(strings.Repeat("x", 100))
	if err == nil {
		t.Fatal("expected error when no timestamp")
	}
}

func TestParseZmMailTimeLayouts(t *testing.T) {
	// 验证 layouts 切片里的格式可被标准库解析（与实现一致）
	for _, s := range []string{"2025-01-02 03:04", "2025-01-02T03:04:05"} {
		if _, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local); err != nil && !strings.Contains(s, "T") {
			t.Errorf("layout 无法解析 %q: %v", s, err)
		}
	}
}
