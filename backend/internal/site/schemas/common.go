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
	resp, err := a.hc.Get(ctx, joinURL(baseURL, path), cookie, true)
	if err != nil {
		return nil, nil, err
	}
	if resp.IsCloudflare() {
		return nil, nil, fmt.Errorf("blocked by cloudflare on %s（站点开了 CF 人机验证：用浏览器访问该站通过验证后，重新导出完整 cookie 再更新到本站；cf_clearance cookie 需一并包含）", baseURL)
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, nil, fmt.Errorf("parse html: %w", err)
	}
	return doc, resp, nil
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
