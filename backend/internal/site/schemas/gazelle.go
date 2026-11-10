// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Gazelle 是 Gazelle 框架系站点适配器（WhatCD/Gazelle 系）。
// HTML 抓取，内嵌 BaseHTMLAdapter；group→torrent 扁平化由 site.ParseGazelleSearch 处理。
// 用户资料两步：/ 发现 uid → /user.php?id=N（best-effort）。
// bespoke 站（PTP 内联 JS / AnimeBytes JSON / Anthelion 自定义）待 M6 per-site 覆盖。
type Gazelle struct {
	BaseHTMLAdapter
}

// NewGazelle 构造适配器，注入共享 httpclient。
func NewGazelle(hc *httpclient.Client) *Gazelle {
	return &Gazelle{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}}
}

func (g *Gazelle) Schema() string { return "gazelle" }
func (g *Gazelle) Name() string   { return "site.gazelle" }
func (g *Gazelle) Priority() int  { return 100 }

// Search 抓取 /torrents.php?searchstr=… 并按 group→torrent 扁平化解析。
func (g *Gazelle) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def)
	u := site.BuildSearchURL(baseURL, def, keyword, category, page)
	resp, err := g.hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, err
	}
	// 5xx / CF 页解析出来是「零条种子」，与「这个关键词没结果」在返回值上无法区分——
	// 跨站搜索会把故障站渲染成「该站无匹配」。
	if err := guardHTMLResp(resp, baseURL, u); err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseGazelleSearch(doc, def, baseURL), nil
}

// FetchUserInfo 两步：/ 发现 uid（兼登录探针）→ /user.php?id=N 解析 ul.stats（best-effort）。
func (g *Gazelle) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	def = site.EnsureDefaults(def)
	// 步骤 1：首页（兼登录探针）→ 发现 uid
	doc, resp, err := g.fetchDoc(ctx, baseURL, "/", cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	uid := site.ParseGazelleUserID(doc, def)
	out := &site.UserInfo{ID: uid}
	// 步骤 2：用户资料页 user.php?id=N（best-effort，失败保留 step1 的 id）
	if uid != "" {
		if udoc, _, e := g.fetchDoc(ctx, baseURL, "user.php?id="+uid, cookie); e == nil {
			if p := site.ParseUserInfo(udoc, def); p != nil {
				site.ParseUserDetail(udoc, def, p)
				*out = *p
				if out.ID == "" {
					out.ID = uid
				}
			}
		}
	}
	if len(def.UserInfo.Pages) > 0 {
		if err := fetchUserInfoPages(ctx, &g.BaseHTMLAdapter, def, baseURL, cookie, out); err != nil {
			return nil, err
		}
	}
	if userInfoParsesEmpty(out) {
		// uid 没解析出来（首页改版 / 选择器落空）且资料页也没补到任何东西 → 全 0 空结构，
		// 当成功返回会让上层写进 site_stats 并标 connected。
		return nil, emptyUserInfoErr(baseURL, "用户页")
	}
	return out, nil
}

// TestConnection 用两步用户流程探测连接与登录态，返回用户等级。
func (g *Gazelle) TestConnection(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (bool, string, error) {
	ui, err := g.FetchUserInfo(ctx, def, baseURL, cookie)
	if err != nil {
		return false, "", err
	}
	return true, ui.Level, nil
}

// KeepAlive 实现 site.KeepAliveCapable：对首页做认证 GET 刷新 session，保活 cookie。
// 复用 keepAliveIndex（与 FetchUserInfo 同一「GET / + loggedOut」探针）。
func (g *Gazelle) KeepAlive(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.KeepAliveResult, error) {
	return g.keepAliveIndex(ctx, def, baseURL, cookie)
}
