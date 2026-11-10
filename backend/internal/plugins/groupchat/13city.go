// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package groupchat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// thirteenCityHandler 13City：喊话前可自动购买"诸神赐福"勋章(实现 BlessingMedal)；
// PollFeedback 匹配"掌管啤酒瓶的神"回复(啤酒瓶奖励)。
type thirteenCityHandler struct{ nexusPHPBase }

func (thirteenCityHandler) Match(s SiteRef) bool {
	return strings.Contains(s.Name, "13City") || strings.Contains(s.Name, "13city")
}

// EnsureBlessing 实现 BlessingMedal：检查勋章页，若无"诸神赐福"则 POST /ajax.php 购买(medal_id=11)。
// 返回状态文本。无真实页面样本，逻辑翻译自原版 sites/13City.py:_ensure_blessing_medal，待真机校准。
func (h thirteenCityHandler) EnsureBlessing(ctx context.Context, c Ctx) (string, error) {
	// 1. 查勋章页是否已有
	has, err := h.hasBlessingMedal(ctx, c)
	if err != nil {
		return "", err
	}
	if has {
		return "已拥有诸神赐福", nil
	}
	// 2. 购买
	form := url.Values{}
	form.Set("action", "buyMedal")
	form.Set("params[medal_id]", "11")
	endpoint := joinBase(c.Site.BaseURL, "/ajax.php")
	resp, err := c.HC.PostForm(ctx, endpoint, c.Cookie, form.Encode(), true)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("buyMedal HTTP %d", resp.StatusCode)
	}
	var res struct {
		Ret    any    `json:"ret"`
		Msg    string `json:"msg"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(resp.Body, &res); err != nil {
		return string(resp.Body), nil
	}
	if msg := strings.TrimSpace(res.Msg); msg != "" {
		return "购买：" + msg, nil
	}
	return "购买请求已发送", nil
}

// hasBlessingMedal 抓勋章页判断是否拥有"诸神赐福"(贪心匹配文本，待真机校准)。
func (h thirteenCityHandler) hasBlessingMedal(ctx context.Context, c Ctx) (bool, error) {
	endpoint := joinBase(c.Site.BaseURL, "/medal.php")
	resp, err := c.HC.GetWithHeaders(ctx, endpoint, c.Cookie, map[string]string{"Referer": refererOf(c.Site.BaseURL)}, true)
	if err != nil {
		return false, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
	if err != nil {
		return false, nil
	}
	found := false
	doc.Find("body").Each(func(_ int, s *goquery.Selection) {
		if strings.Contains(s.Text(), "诸神赐福") {
			found = true
		}
	})
	return found, nil
}

// PollFeedback 覆写：匹配"掌管啤酒瓶的神"对 @user 的回复(啤酒瓶奖励)。
func (h thirteenCityHandler) PollFeedback(ctx context.Context, c Ctx, message string, initial *Feedback) (*Feedback, error) {
	fb, _ := pollShoutbox(ctx, c, 3, 2*time.Second, func(doc *goquery.Document, user string) *Feedback {
		var hit string
		doc.Find("tr").Has("td.shoutrow").Each(func(_ int, s *goquery.Selection) {
			if hit != "" {
				return
			}
			text := strings.TrimSpace(s.Text())
			if !strings.Contains(text, "掌管啤酒瓶的神") {
				return
			}
			if user != "" && !strings.Contains(text, "@"+user) {
				return
			}
			hit = text
		})
		if hit == "" {
			return nil
		}
		fb := &Feedback{Raw: hit}
		if strings.Contains(hit, "啤酒瓶") || strings.Contains(hit, "啤酒") {
			fb.Rewards = append(fb.Rewards, Reward{Type: "啤酒瓶", Text: hit})
		}
		return fb
	})
	if fb == nil {
		return initial, nil
	}
	fb.Sent = true
	return fb, nil
}
