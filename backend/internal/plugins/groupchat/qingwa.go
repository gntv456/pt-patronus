package groupchat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// qingwaHandler 青蛙：Send 响应解析 //ul[1]/li/text()(非 tr/td)；"发了!"→"一般为10G!"。
// 额外实现 DailyBonusBuyer(POST /api/bonus-shop/exchange {id:28,amount:1} 兑换 1000蝌蚪)。
type qingwaHandler struct{ nexusPHPBase }

func (qingwaHandler) Match(s SiteRef) bool { return strings.Contains(strings.ToLower(s.Name), "青蛙") }

// Send 覆写：用基类的 GET 发送，但响应按 ul/li 解析(青蛙 shoutbox 结构不同)。
func (h qingwaHandler) Send(ctx context.Context, c Ctx, message string) (*Feedback, error) {
	// 复用基类发送逻辑(同样的 GET 四参)，但自己解析响应。
	q := shoutboxQuery(message)
	full := shoutboxURL(c.Site.BaseURL) + "?" + q
	resp, err := c.HC.GetWithHeaders(ctx, full, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qingwa shoutbox HTTP %d", resp.StatusCode)
	}
	fb := parseQingwaFeedback(string(resp.Body))
	fb.Sent = true
	return fb, nil
}

// parseQingwaFeedback 解析青蛙响应：取首个 ul 的 li 文本拼接，"发了!" 视为蝌蚪奖励(约10G)。
func parseQingwaFeedback(html string) *Feedback {
	fb := &Feedback{}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return fb
	}
	var parts []string
	doc.Find("ul").First().Find("li").Each(func(_ int, s *goquery.Selection) {
		if t := strings.TrimSpace(s.Text()); t != "" {
			parts = append(parts, t)
		}
	})
	if len(parts) == 0 {
		return fb
	}
	text := strings.Join(parts, " ")
	fb.Raw = text
	if strings.Contains(text, "发了") {
		fb.Rewards = append(fb.Rewards, Reward{Type: "青蛙", Text: text + "(一般为10G!)"})
	}
	return fb
}

// BuyDailyBonus 实现 DailyBonusBuyer：POST /api/bonus-shop/exchange 兑换每日福利(item id=28)。
func (qingwaHandler) BuyDailyBonus(ctx context.Context, c Ctx) (string, error) {
	endpoint := joinBase(c.Site.BaseURL, "/api/bonus-shop/exchange")
	body, _ := json.Marshal(map[string]any{"id": 28, "amount": 1})
	resp, err := c.HC.Do(ctx, "POST", endpoint, body, map[string]string{
		"Content-Type": "application/json",
		"Referer":      refererOf(c.Site.BaseURL),
	}, c.Cookie, true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("bonus HTTP %d", resp.StatusCode)
	}
	var res struct {
		Success bool   `json:"success"`
		Msg     string `json:"msg"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(resp.Body), &res); err != nil {
		return string(resp.Body), nil
	}
	if res.Success {
		return "兑换成功(1000蝌蚪)", nil
	}
	return strings.TrimSpace(res.Msg), nil
}
