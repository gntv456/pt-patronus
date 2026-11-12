// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// GenericHTML 是通用选择器驱动适配器，兜底无专属适配器的 schema
// （""、xbtit、tcg、tbsource、filelist、discuz、rartracker、fsm、cgbtsource、meantorrent、aidoruonline、smallhorse）。
// schema 参数化——每个 schema 注册一个实例（RegisterAdapter 按 Schema() 去重）。
// 选择器完全由 definition（YAML）驱动：HTML 走 site.ParseSearch，JSON 走 site.ParseJSONSearch。
// schema ""（通用私有站系，含 nyaa/rutracker/hdbits/torrentleech 等）的
// HTML/JSON 分发：def.Format 显式 > 响应体首字节嗅探。
type GenericHTML struct {
	BaseHTMLAdapter
	schema string
}

// NewGenericHTML 构造绑定特定 schema 的通用适配器实例。
func NewGenericHTML(hc *httpclient.Client, schema string) *GenericHTML {
	return &GenericHTML{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}, schema: schema}
}

func (g *GenericHTML) Schema() string { return g.schema }
func (g *GenericHTML) Name() string   { return "site.generic." + g.schema }
func (g *GenericHTML) Priority() int  { return 200 } // 兜底，低于专属适配器(100)

// Search 抓取并按 def.Format / 响应体嗅探分发 HTML 或 JSON 解析。
// URL：路径占位符（torrentleech /query 形）→ BuildJSONSearchURL；否则 BuildSearchURL。
func (g *GenericHTML) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def)
	if def.ID == "gtnet" {
		return g.searchGTNet(ctx, def, baseURL, cookie, keyword)
	}
	var target string
	if site.PathPlaceholderUsed(def.SearchEntry) {
		target = site.BuildJSONSearchURL(baseURL, def, keyword, page)
	} else {
		target = site.BuildSearchURL(baseURL, def, keyword, category, page)
	}
	resp, err := g.hc.Get(ctx, target, cookie, true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	// 分发：Format 显式 json/html > 响应体首字节嗅探（{/[ → JSON，否则 HTML）。
	if def.Format == "json" || (def.Format == "" && site.LooksLikeJSON(resp.Body)) {
		return site.ParseJSONSearch(resp.Body, def, baseURL)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseSearch(doc, def, baseURL), nil
}

var gtnetSecurityTokenRe = regexp.MustCompile(`SECURITYTOKEN\s*=\s*"([^"]+)"`)

func (g *GenericHTML) searchGTNet(ctx context.Context, def *site.SiteMetadata, baseURL, cookie, keyword string) ([]site.Torrent, error) {
	doc, resp, err := g.fetchDoc(ctx, baseURL, "/torrentslist.php", cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}

	form := url.Values{}
	form.Set("do", "process")
	form.Set("securitytoken", extractGTNetSecurityToken(doc))
	form.Set("query", keyword)

	resp, err = g.hc.PostForm(ctx, joinURL(baseURL, def.SearchEntry), cookie, form.Encode(), true)
	if err != nil {
		return nil, err
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, site.ErrNotLoggedIn
	}
	doc, err = goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	return site.ParseSearch(doc, def, baseURL), nil
}

func extractGTNetSecurityToken(doc *goquery.Document) string {
	if doc == nil {
		return ""
	}
	if v, ok := doc.Find(`input[name="securitytoken"]`).First().Attr("value"); ok {
		if token := strings.TrimSpace(v); token != "" {
			return token
		}
	}
	var token string
	doc.Find("script").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		m := gtnetSecurityTokenRe.FindStringSubmatch(s.Text())
		if len(m) > 1 {
			token = strings.TrimSpace(m[1])
			return false
		}
		return true
	})
	return token
}

// FetchUserInfo best-effort。schema "" 经 EnsureDefaults 注入了 NexusPHP usercp.php 默认
// （非站点真实 user_info），直接返 stub；命名 schema（xbtit/tcg/...）走 default 分支不注入，
// def.UserInfo.Path 反映 YAML——有则抓+解析（失败降级 stub 不报错），无则 stub。
// 绝不因缺 user_info 阻塞 TestConnection。
func (g *GenericHTML) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	def = site.EnsureDefaults(def)
	if def.ID == "torrentleech" {
		return fetchTorrentLeechUserInfo(ctx, g.hc, def, baseURL, cookie)
	}
	if def.ID == "pussytorrents" {
		return fetchPussyTorrentsUserInfo(ctx, g.hc, def, baseURL, cookie)
	}
	if def.ID == "aidoruonline" {
		return fetchAidoruOnlineUserInfo(ctx, g.hc, def, baseURL, cookie)
	}
	if def.ID == "starspace" {
		return fetchStarSpaceUserInfo(ctx, g.hc, def, baseURL, cookie)
	}
	if def.ID == "skyeysnow" {
		return fetchSkyeySnowUserInfo(ctx, &g.BaseHTMLAdapter, def, baseURL, cookie)
	}
	if def.ID == "zhixing" {
		return fetchZhixingUserInfo(ctx, g.hc, def, baseURL, cookie)
	}
	if len(def.UserInfo.Pages) > 0 {
		u := &site.UserInfo{}
		if err := fetchUserInfoPages(ctx, &g.BaseHTMLAdapter, def, baseURL, cookie, u); err != nil {
			return nil, err
		}
		if userInfoParsesEmpty(u) {
			// 站点配了 user_info.pages 却一页都没读到 → 改版或错误页；
			// 返空结构会被聚合任务当真实数据写进 site_stats 并标 connected。
			return nil, emptyUserInfoErr(baseURL, "用户信息页")
		}
		return u, nil
	}
	if g.schema == "" || def.UserInfo.Path == "" || len(def.UserInfo.Fields) == 0 {
		return &site.UserInfo{}, nil
	}
	doc, resp, err := g.fetchDoc(ctx, baseURL, def.UserInfo.Path, cookie)
	if err != nil {
		return &site.UserInfo{}, nil // best-effort 降级
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	parsed := site.ParseUserInfo(doc, def)
	if userInfoParsesEmpty(parsed) {
		// 路径与字段都配了却一个都没落到位 = 站点改版，不是「用户数据全 0」。
		return nil, emptyUserInfoErr(baseURL, "用户信息页")
	}
	return parsed, nil
}
