// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"bytes"
	"context"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// InspectResult 是诊断抓取的结果。
type InspectResult struct {
	StatusCode int               `json:"status_code"`
	FinalURL   string            `json:"final_url"`
	Location   string            `json:"location,omitempty"` // 重定向目标（未跟随时）
	Cloudflare bool              `json:"cloudflare"`
	LoggedOut  bool              `json:"logged_out"`
	Fields     map[string]string `json:"fields"`    // 每个待试选择器抓到的文本
	HTMLHead   string            `json:"html_head"` // 页面前 2KB，便于肉眼定位元素
}

// Inspect 用给定 cookie 抓取 URL，返回状态/登录态/各选择器提取结果/页面片段。
// 用于校准 definitions/*.yaml 的 selectors（对照真实 HTML 快速试错）。
func Inspect(ctx context.Context, hc *httpclient.Client, url, cookie string,
	fields map[string]ElementQuery, nologin NoLoginDef) (*InspectResult, error) {

	resp, err := hc.Get(ctx, url, cookie, true)
	if err != nil {
		return nil, err
	}
	doc, derr := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))

	res := &InspectResult{
		StatusCode: resp.StatusCode,
		FinalURL:   resp.FinalURL,
		Location:   resp.Header.Get("Location"),
		Cloudflare: resp.IsCloudflare(),
		Fields:     map[string]string{},
	}
	// 登录态判定：优先 Location 头，否则用 FinalURL
	checkURL := res.Location
	if checkURL == "" {
		checkURL = resp.FinalURL
	}
	if derr == nil {
		res.LoggedOut = IsLoggedOut(resp.StatusCode, checkURL, doc, nologin)
		for name, q := range fields {
			res.Fields[name] = Extract(doc.Selection, q)
		}
	}
	head := string(resp.Body)
	if len(head) > 2048 {
		head = head[:2048]
	}
	res.HTMLHead = head
	return res, nil
}
