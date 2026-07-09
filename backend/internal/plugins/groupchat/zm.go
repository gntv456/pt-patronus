package groupchat

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// zmHandler 织梦：单条 Send 复用 NexusPHP GET(引擎按 zm_interval 间隔逐条调)；
// PollFeedback 时间优先匹配(@user：+皮总+上传量/电力/没有理你，按"X分钟前"打分取最新)；
// LatestMailTime 抓 messages.php "收到来自 zmpt 赠送的"邮件时间(独立调度锚点)。
type zmHandler struct{ nexusPHPBase }

func (zmHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "织梦") }

var zmMinAgoRe = regexp.MustCompile(`(\d+)\s*分钟前`)

// PollFeedback 覆写：织梦 feedback 时间优先匹配。
func (zmHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	resp, err := c.HC.GetWithHeaders(ctx, shoutboxURL(c.Site.BaseURL), c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return initial, err
	}
	fb := parseZmFeedback(string(resp.Body), c.Username)
	fb.Sent = true
	if len(fb.Rewards) == 0 && fb.Raw == "" {
		return initial, nil
	}
	return fb, nil
}

// parseZmFeedback 扫 td.shoutrow，匹配 @username：…皮总…(上传量|电力|没有理你)，按时间优先取最新。
func parseZmFeedback(html, username string) *Feedback {
	fb := &Feedback{}
	if username == "" {
		return fb
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return fb
	}
	bestPri := -1
	var bestText string
	doc.Find("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}
		// 必须含 @username：/半角冒号
		if !strings.Contains(text, "@"+username+"：") && !strings.Contains(text, "@"+username+":") {
			return
		}
		if !strings.Contains(text, "皮总") {
			return
		}
		pri := 0
		if strings.Contains(text, "<1分钟前") || strings.Contains(text, "&lt;1分钟前") {
			pri = 100
		} else if m := zmMinAgoRe.FindStringSubmatch(text); len(m) > 1 {
			if mins, err := strconv.Atoi(m[1]); err == nil {
				pri = 100 - mins
			}
		}
		if pri > bestPri {
			bestPri = pri
			bestText = text
		}
	})
	if bestText == "" {
		return fb
	}
	fb.Raw = bestText
	if strings.Contains(bestText, "上传量") || strings.Contains(bestText, "上传") {
		fb.Rewards = append(fb.Rewards, Reward{Type: "上传量", Text: bestText})
	}
	if strings.Contains(bestText, "电力") {
		fb.Rewards = append(fb.Rewards, Reward{Type: "电力", Text: bestText})
	}
	// "没有理你" → 皮总未回应，无奖励(保留 Raw 供调试)
	return fb
}

// zmMailTimeRe 抓取织梦邮件时间戳(多种格式)。
var zmMailTimeRe = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}(?::\d{2})?)`)

// LatestMailTime 实现 MailTimeFetcher：抓 messages.php 找"收到来自 zmpt 赠送的"附近时间戳。
func (zmHandler) LatestMailTime(ctx context.Context, c Ctx) (time.Time, error) {
	u := joinBase(c.Site.BaseURL, "/messages.php")
	resp, err := c.HC.GetWithHeaders(ctx, u, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return time.Time{}, err
	}
	return parseZmMailTime(string(resp.Body))
}

// parseZmMailTime 在 HTML 里定位"收到来自 zmpt 赠送的"关键字，取其附近窗口内的时间戳。
func parseZmMailTime(html string) (time.Time, error) {
	idx := strings.Index(html, "收到来自 zmpt 赠送的")
	window := html
	if idx < 0 {
		// 退化：整个文档搜 zmpt
		idx = strings.Index(html, "zmpt")
	}
	if idx >= 0 {
		start := idx - 2000
		if start < 0 {
			start = 0
		}
		end := idx + 1000
		if end > len(html) {
			end = len(html)
		}
		window = html[start:end]
	}
	m := zmMailTimeRe.FindString(window)
	if m == "" {
		return time.Time{}, fmt.Errorf("zm mail time not found")
	}
	layouts := []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05"}
	for _, l := range layouts {
		if t, err := time.ParseInLocation(l, m, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("zm mail time unparseable: %s", m)
}
