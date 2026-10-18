// Package schemas 实现各平台 Schema 适配器。每个适配器服务该 schema 下所有站点定义。
//
// common.go 提供 HTML 抓取系适配器（NexusPHP/Unit3D/Gazelle...）的公共基类与 helper：
//   - BaseHTMLAdapter：内嵌共享 httpclient，提供 fetchDoc（GET + goquery 解析 + CF 判定）
//   - joinURL / loggedOut：URL 拼接与登录态判定的通用薄封装
//
// 各 HTML schema 适配器内嵌 BaseHTMLAdapter 复用抓取管道，无需各自重写 HTTP 编排。
// 非 HTML（JSON API）适配器（如 mteam）不内嵌，自行组织请求。
package schemas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// BaseHTMLAdapter 是 HTML 抓取系 schema 适配器的公共基类。
// 内嵌共享 httpclient；fetchDoc 为其方法，子适配器内嵌后经方法提升直接调用 a.fetchDoc / n.fetchDoc。
type BaseHTMLAdapter struct {
	hc *httpclient.Client
}

// fetchDoc GET 一个相对路径并解析为 goquery 文档。返回文档与原始响应（供登录态/CF 判定）。
// baseURL + path 经 joinURL 拼接；cookie 注入鉴权；命中 Cloudflare 时返回错误。
func (a *BaseHTMLAdapter) fetchDoc(ctx context.Context, baseURL, path, cookie string) (*goquery.Document, *httpclient.Response, error) {
	return fetchHTMLDoc(ctx, a.hc, baseURL, path, cookie)
}

// fetchHTMLDoc 是 HTML 抓取的单一判定管道：CF 盾、5xx 错误页、解析失败三道都会报错。
// BaseHTMLAdapter.fetchDoc 与不走基类的站点自建 GET helper（torrentleech 系等）共用，
// 否则旁路的那些站点会漏判——错误页被解析成空文档就成了「成功的空数据」。
func fetchHTMLDoc(ctx context.Context, hc *httpclient.Client, baseURL, path, cookie string) (*goquery.Document, *httpclient.Response, error) {
	full := joinURL(baseURL, path)
	resp, err := hc.Get(ctx, full, cookie, true)
	if err != nil {
		return nil, resp, err
	}
	if err := guardHTMLResp(resp, baseURL, full); err != nil {
		return nil, resp, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, resp, fmt.Errorf("parse html: %w", err)
	}
	return doc, resp, nil
}

// guardHTMLResp 两道响应判定：CF 人机验证页、5xx 服务器错误页。
// 两者都不可能当数据用——硬解析只会得到「所有选择器落空」的空文档，
// 上层于是拿到「什么都没解析到但没有错误」的结果，全 0 被当真实数据写进快照。
func guardHTMLResp(resp *httpclient.Response, baseURL, fullURL string) error {
	if resp.IsCloudflare() {
		return fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站；cf_clearance cookie 需一并包含）", baseURL)
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("站点返回 HTTP %d（服务器错误页，未作为数据解析）: %s", resp.StatusCode, fullURL)
	}
	return nil
}

// userInfoParsesEmpty 一次用户信息抓取是否什么都没解析到。
// 有用户名 / uid / 等级 / 任一数值信号即认为读到东西——只有这些全空时，
// 拿到的其实是「空结构」，与真实的 0 上传新号无法区分，只能按失败处理。
func userInfoParsesEmpty(u *site.UserInfo) bool {
	return u != nil && u.Name == "" && u.ID == "" && u.Level == "" &&
		u.Uploaded == 0 && u.Downloaded == 0 && u.Ratio == 0 &&
		u.Seeding == 0 && u.SeedingSize == 0 && u.Bonus == 0
}

// emptyUserInfoErr 「什么都没解析到」的统一错误。
// 不能当成功返回：调用方（聚合快照/仪表盘/考核）会把全 0 的 UserInfo 当真实数据
// 写进 site_stats 并标成 connected，用户看到「分享率一夜跌到 0」的假事故，
// 真因（页面没解析出来）反而被埋。
func emptyUserInfoErr(baseURL, page string) error {
	return fmt.Errorf("未从 %s 的%s解析到任何用户信息（站点改版或返回了错误页），已按失败处理以免写入全 0 快照", baseURL, page)
}

// loggedOut 判定响应是否处于未登录态：兼顾最终 URL 与 Location 重定向头。
// 对 site.IsLoggedOut 的薄封装，所有 HTML 适配器共用。
func loggedOut(resp *httpclient.Response, doc *goquery.Document, def site.NoLoginDef) bool {
	final := resp.FinalURL
	if l := resp.Header.Get("Location"); l != "" {
		final = l
	}
	return site.IsLoggedOut(resp.StatusCode, final, doc, def)
}

// joinURL 把 baseURL 与相对 path 拼成绝对 URL，归一双方的多余斜杠。
func joinURL(base, path string) string {
	for len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	for len(base) > 0 && base[len(base)-1] == '/' {
		base = base[:len(base)-1]
	}
	return base + "/" + path
}

// keepAliveIndex 对首页（"/"）做一次认证 GET 作为 cookie 保活探针，实现 site.KeepAliveCapable。
// unit3d / gazelle / luminance 的会话探针都是首页（各自 FetchUserInfo 已验证「GET / + loggedOut」），
// 故三站共用此实现——不引入任何新选择器，登出检测复用既有已校准逻辑。
//   - 刷新服务端 session（防 cookie 长期不活动过期）——主防御
//   - 捕获响应 Set-Cookie 轮换后的最新 cookie（RefreshedCookie）——回写 DB
//   - 报告当前是否仍处于登录态（LoggedIn）
func (a *BaseHTMLAdapter) keepAliveIndex(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.KeepAliveResult, error) {
	def = site.EnsureDefaults(def)
	doc, resp, err := a.fetchDoc(ctx, baseURL, "/", cookie)
	if err != nil {
		return nil, err
	}
	loggedIn := !loggedOut(resp, doc, def.NoLogin)
	refreshed, _ := a.hc.Cookies(joinURL(baseURL, "/"))
	return &site.KeepAliveResult{LoggedIn: loggedIn, RefreshedCookie: refreshed}, nil
}
