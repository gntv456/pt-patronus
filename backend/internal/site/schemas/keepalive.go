package schemas

import (
	"bytes"
	"context"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/site"
)

// KeepAlive 实现 site.KeepAliveCapable：对用户资料页做一次认证 GET。
//   - 刷新服务端 session（防止 cookie 因长期不活动而过期）——主防御
//   - 捕获响应 Set-Cookie 轮换后的最新 cookie（RefreshedCookie）——回写 DB
//   - 返回当前是否仍处于登录态
func (n *NexusPHP) KeepAlive(ctx context.Context, def *site.SiteMetadata, baseURL, cookie string) (*site.KeepAliveResult, error) {
	def = site.EnsureDefaults(def)
	path := def.UserInfo.Path
	if path == "" {
		path = "usercp.php"
	}
	u := joinURL(baseURL, path)

	resp, err := n.hc.Get(ctx, u, cookie, true)
	if err != nil {
		return nil, err
	}
	// 5xx / CF 页面上「没有登录被踢的痕迹」不等于还在登录：报错让调用方走已知错误路径，
	// 而不是把站点故障报成「cookie 保活成功」。
	if err := guardHTMLResp(resp, baseURL, u); err != nil {
		return nil, err
	}
	doc, _ := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	loggedIn := !loggedOut(resp, doc, def.NoLogin)

	refreshed, _ := n.hc.Cookies(u)
	return &site.KeepAliveResult{LoggedIn: loggedIn, RefreshedCookie: refreshed}, nil
}
