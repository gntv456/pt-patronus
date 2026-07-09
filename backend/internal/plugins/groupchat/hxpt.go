package groupchat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// hxptHandler 好学：feedback 需带 ajax_chat=1&t=<ms> 重拉，找用户消息【上一行】的"系统提示："。
// 奖励类型：火花。
type hxptHandler struct{ nexusPHPBase }

func (hxptHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "好学") }

// PollFeedback 覆写：ajax_chat 重拉 → 找用户消息行 → 取前一行系统提示 → 分类火花。
func (hxptHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	u := fmt.Sprintf("%s?ajax_chat=1&type=&t=%d", shoutboxURL(c.Site.BaseURL), time.Now().UnixMilli())
	resp, err := c.HC.GetWithHeaders(ctx, u, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return initial, err
	}
	fb := parseHxptFeedback(string(resp.Body), c.Username)
	fb.Sent = true
	if len(fb.Rewards) == 0 && fb.Raw == "" {
		return initial, nil
	}
	return fb, nil
}

// parseHxptFeedback 扫 shoutrow 行，定位含 @username 的行，取其前一行若以"系统提示："开头则分类。
func parseHxptFeedback(html, username string) *Feedback {
	fb := &Feedback{}
	if username == "" {
		return fb
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return fb
	}
	rows := doc.Find("tr").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return s.Find("td.shoutrow").Length() > 0
	}).Nodes
	// goquery 的 Each 用索引，前一行 = rows[i-1]
	for i := 1; i < len(rows); i++ {
		cur := strings.TrimSpace(goquery.NewDocumentFromNode(rows[i]).Text())
		if !strings.Contains(cur, "@"+username) {
			continue
		}
		prev := strings.TrimSpace(goquery.NewDocumentFromNode(rows[i-1]).Text())
		if strings.HasPrefix(prev, "系统提示：") || strings.Contains(prev, "系统提示:") {
			fb.Raw = prev
			if strings.Contains(prev, "火花") {
				fb.Rewards = append(fb.Rewards, Reward{Type: "火花", Text: prev})
			}
			return fb
		}
	}
	return fb
}
