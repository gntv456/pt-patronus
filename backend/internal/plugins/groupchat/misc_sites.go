// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package groupchat

import (
	"context"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// pollShoutbox 轮询 shoutbox 页面 attempts 次(间隔 wait)，用 scan 回调判断是否命中反馈。
// 命中返回 *Feedback；全部未命中返回 nil。供轮询站(藏宝阁/PTLGS)与单次精解析站共用。
func pollShoutbox(ctx context.Context, c Ctx, attempts int, wait time.Duration, scan func(*goquery.Document, string) *Feedback) (*Feedback, error) {
	for i := 0; i < attempts; i++ {
		if i > 0 {
			if cancelled := sleepCtx(ctx, wait); cancelled {
				return nil, nil
			}
		}
		resp, err := c.HC.GetWithHeaders(ctx, shoutboxURL(c.Site.BaseURL), c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
		if err != nil {
			continue
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
		if err != nil {
			continue
		}
		if fb := scan(doc, c.Username); fb != nil {
			return fb, nil
		}
	}
	return nil, nil
}

func rewardFromText(text string) *Feedback {
	fb := &Feedback{Raw: text}
	fb.Rewards = classifyRewards(text)
	return fb
}

// —— 藏宝阁：轮询 5×3s，匹配"系统:"/"系统："前缀 + @user 行 ——

type cangbaoHandler struct{ nexusPHPBase }

func (cangbaoHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "藏宝阁") }

func (h cangbaoHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 5, 3*time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if user != "" && !strings.Contains(text, "@"+user) {
				return
			}
			if strings.HasPrefix(text, "系统:") || strings.HasPrefix(text, "系统：") {
				hit = text
			}
		})
		if hit == "" {
			return nil
		}
		return rewardFromText(hit)
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}

// —— PTLGS：轮询 5×3s，匹配"黑丝娘"前缀 + @user，支持负奖励("损失") ——

type ptlgsHandler struct{ nexusPHPBase }

func (ptlgsHandler) Match(s SiteRef) bool {
	lname := strings.ToLower(s.Name)
	return strings.Contains(strings.ToUpper(s.Name), "PTLGS") || strings.Contains(lname, "ptlgs")
}

func (h ptlgsHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 5, 3*time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if user != "" && !strings.Contains(text, "@"+user) {
				return
			}
			if strings.HasPrefix(text, "黑丝娘") {
				hit = text
			}
		})
		if hit == "" {
			return nil
		}
		return rewardFromText(hit)
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}

// —— 天枢：sender 在 span.nowrap 与内容分离；匹配 @user + 求魔力/求上传 关键词 ——

type dubheHandler struct{ nexusPHPBase }

func (dubheHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "天枢") }

func (h dubheHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 2, time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("tr").Has("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if user == "" || !strings.Contains(text, "@"+user) {
				return
			}
			sender := strings.TrimSpace(s.Find("span.nowrap").Text())
			// sender 与内容分离：内容里含 @user 但 sender 不含(避免匹配到自己发的)
			if sender != "" && strings.Contains(sender, user) {
				return
			}
			hit = text
		})
		if hit == "" {
			return nil
		}
		return rewardFromText(hit)
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}

// —— Moment：匹配 【username的女友】 行 ——

type momentHandler struct{ nexusPHPBase }

func (momentHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "Moment") }

func (h momentHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 2, time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if strings.Contains(text, "【"+user+"的女友】") {
				hit = text
			}
		})
		if hit == "" {
			return nil
		}
		return rewardFromText(hit)
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}

// —— PTS：3 层匹配 div.magic-reward-top.system-msg → shoutrow td → form 字段 ——

type ptskitHandler struct{ nexusPHPBase }

func (ptskitHandler) Match(s SiteRef) bool { return strings.Contains(s.Name, "PTS") }

func (h ptskitHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 3, time.Second, func(doc *goquery.Document, user string) *Feedback {
		// 层 1：magic-reward-top system-msg
		var hit string
		doc.Find("div.magic-reward-top.system-msg").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if strings.Contains(text, "用户「"+user+"」") || user == "" {
				hit = text
			}
		})
		if hit == "" {
			doc.Find("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
				if hit != "" {
					return
				}
				text := strings.TrimSpace(s.Text())
				if user == "" || strings.Contains(text, user) {
					if strings.Contains(text, "魔力值") || strings.Contains(text, "获得") {
						hit = text
					}
				}
			})
		}
		if hit == "" {
			return nil
		}
		fb := rewardFromText(hit)
		// PTS 主奖励是魔力值，强制 type
		if len(fb.Rewards) == 0 {
			fb.Rewards = []Reward{{Type: "魔力值", Text: hit}}
		}
		return fb
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}

// —— 幸运：DIV 布局 wish-bubble-system>wish-content 或 chat-message-container>chat-content ——

type luckptHandler struct{ nexusPHPBase }

func (luckptHandler) Match(s SiteRef) bool {
	return strings.Contains(s.Name, "LuckPT") || strings.Contains(s.Name, "幸运")
}

func (h luckptHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 2, time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("div.wish-bubble-system div.wish-content").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if user == "" || strings.Contains(text, "@"+user) {
				hit = text
			}
		})
		if hit == "" {
			doc.Find("div.chat-message-container div.chat-content").Each(func(_ int, s *goquery.Selection) {
				if hit != "" {
					return
				}
				text := strings.TrimSpace(s.Text())
				if user == "" || strings.Contains(text, "@"+user) {
					hit = text
				}
			})
		}
		if hit == "" {
			return nil
		}
		return rewardFromText(hit)
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}
