// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package groupchat

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// nexusPHPBase 是 NexusPHP 系喊话的默认实现，14 站共享底座。
// 特殊站通过嵌入本结构体 + 覆写 Send/FetchFeedback 实现差异化。
//
// 默认协议(对标原版 ISiteHandler.send_messagebox)：
//
//	Send:         GET /shoutbox.php?shbox_text=<msg>&shout=我喊&sent=yes&type=shoutbox
//	FetchFeedback: GET /shoutbox.php → goquery 解析 //tr[td.shoutrow] 找含 @username 的行
type nexusPHPBase struct{}

func shoutboxURL(base string) string { return joinBase(base, "/shoutbox.php") }

func joinBase(base, path string) string {
	b := strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return b + path
}

// referer 站点根(很多 NexusPHP 喊话要 Referer 才认)。
func refererOf(base string) string {
	return strings.TrimRight(base, "/") + "/"
}

// shoutboxQuery 构造 NexusPHP 喊话的固定四参查询串(shbox_text/shout=我喊/sent=yes/type=shoutbox)。
func shoutboxQuery(message string) string {
	q := url.Values{}
	q.Set("shbox_text", message)
	q.Set("shout", "我喊")
	q.Set("sent", "yes")
	q.Set("type", "shoutbox")
	return q.Encode()
}

// Send 默认 NexusPHP 喊话(GET shoutbox.php 带固定四参)，响应页粗解析反馈。
func (nexusPHPBase) Send(ctx context.Context, c Ctx, message string) (*Feedback, error) {
	full := shoutboxURL(c.Site.BaseURL) + "?" + shoutboxQuery(message)
	resp, err := c.HC.GetWithHeaders(ctx, full, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("shoutbox HTTP %d", resp.StatusCode)
	}
	fb := parseNexusFeedback(string(resp.Body), c.Username)
	fb.Sent = true
	return fb, nil
}

// PollFeedback 重新拉 shoutbox 页面精解析(找含 @username 的最新行)。
// nexusPHPHandler 通过嵌入继承此方法从而实现 Poller。
func (nexusPHPBase) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	resp, err := c.HC.GetWithHeaders(ctx, shoutboxURL(c.Site.BaseURL), c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return initial, err
	}
	fb := parseNexusFeedback(string(resp.Body), c.Username)
	fb.Sent = true
	if len(fb.Rewards) == 0 && fb.Raw == "" {
		return initial, nil // 二次拉没拿到更好结果，保留 initial
	}
	return fb, nil
}

// parseNexusFeedback 解析 NexusPHP shoutbox HTML，匹配含 @username 的行，按关键词分类奖励。
func parseNexusFeedback(html, username string) *Feedback {
	fb := &Feedback{}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return fb
	}
	var matched string
	doc.Find("tr").Each(func(_ int, s *goquery.Selection) {
		if matched != "" {
			return
		}
		if s.Find("td.shoutrow").Length() == 0 {
			return
		}
		text := strings.TrimSpace(s.Text())
		if text == "" {
			return
		}
		if username != "" && strings.Contains(text, "@"+username) {
			matched = text
		}
	})
	if matched == "" {
		return fb
	}
	fb.Raw = matched
	fb.Rewards = classifyRewards(matched)
	return fb
}

// rewardKeywords 奖励类型 → 触发关键词(对标原版各 handler 的分类逻辑)。
var rewardKeywords = []struct {
	Type string
	Kws  []string
}{
	{"上传量", []string{"上传量", "上传"}},
	{"下载量", []string{"下载量", "下载"}},
	{"魔力值", []string{"魔力值", "魔力"}},
	{"工分", []string{"工分"}},
	{"VIP", []string{"VIP", "vip"}},
	{"彩虹ID", []string{"彩虹ID", "彩虹"}},
	{"电力", []string{"电力"}},
	{"象草", []string{"象草"}},
	{"火花", []string{"火花"}},
	{"啤酒瓶", []string{"啤酒瓶", "啤酒"}},
}

// classifyRewards 按关键词从文本提取奖励；含"损失"标记为负向。
func classifyRewards(text string) []Reward {
	negative := strings.Contains(text, "损失")
	var rewards []Reward
	for _, kw := range rewardKeywords {
		hit := false
		for _, k := range kw.Kws {
			if strings.Contains(text, k) {
				hit = true
				break
			}
		}
		if hit {
			rewards = append(rewards, Reward{Type: kw.Type, Text: text, Negative: negative})
		}
	}
	return rewards
}

// specialSiteKeywords 被 NexusPHP 兜底排除的站名子串(复刻原版 sites/NexusPHP.py:28)。
// 命中任一即视为特殊站，由对应的特化 handler 处理。
var specialSiteKeywords = []string{
	"织梦", "象站", "青蛙", "LongPT", "longpt",
	"藏宝阁", "Moment", "moment", "13City", "13city",
	"好学", "PTS", "天枢", "LuckPT", "幸运", "PTLGS", "ptlgs",
}

func nameMatchesAny(name string, keywords []string) bool {
	lname := strings.ToLower(name)
	for _, k := range keywords {
		if strings.Contains(lname, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// nexusPHPHandler 是 NexusPHP 兜底 handler：匹配所有未被特殊站认领的 NexusPHP 站。
type nexusPHPHandler struct{ nexusPHPBase }

func (nexusPHPHandler) Match(s SiteRef) bool {
	return !nameMatchesAny(s.Name, specialSiteKeywords)
}
