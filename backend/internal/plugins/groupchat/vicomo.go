package groupchat

import (
	"context"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// vicomoHandler 象站：feedback 走站内信(/messages.php 收件箱)，不扫 shoutbox。
// 发送喊话仍用 NexusPHP GET(基类 Send)；PollFeedback 抓收件箱第一条"象草"奖励私信。
// 标记已读逻辑待真机校准(原版 POST messages.php?action=moveordel&markread)，本轮先只读。
type vicomoHandler struct{ nexusPHPBase }

func (vicomoHandler) Match(s SiteRef) bool { return strings.Contains(strings.ToLower(s.Name), "象站") }

func (h vicomoHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	u := joinBase(c.Site.BaseURL, "/messages.php")
	resp, err := c.HC.GetWithHeaders(ctx, u, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return initial, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
	if err != nil {
		return initial, nil
	}
	// 找第一条消息行的主题文本(选择器待真机校准：messages 表格第一个非表头行)
	var topic string
	doc.Find("table tr").Each(func(_ int, s *goquery.Selection) {
		if topic != "" {
			return
		}
		tds := s.Find("td")
		if tds.Length() == 0 {
			return // 表头
		}
		text := strings.TrimSpace(tds.First().Text())
		if text == "" {
			text = strings.TrimSpace(s.Text())
		}
		if strings.Contains(text, "象草") || strings.Contains(text, "电力") || strings.Contains(text, "赠送") {
			topic = text
		}
	})
	if topic == "" {
		return initial, nil
	}
	fb := &Feedback{Sent: true, Raw: topic}
	if strings.Contains(topic, "象草") {
		fb.Rewards = append(fb.Rewards, Reward{Type: "象草", Text: topic})
	}
	return fb, nil
}
