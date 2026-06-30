package schemas

import (
	"bytes"
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// Unit3D 是 UNIT3D（Laravel 框架）系站点的适配器。HTML 抓取，内嵌 BaseHTMLAdapter 复用 fetchDoc。
// 选择器由 EnsureDefaults 注入的 UNIT3D 框架默认（DefaultUnit3D*）驱动；站点 YAML 可覆盖。
// 用户资料走「三步流程」（首页发现用户名 → /users/{name} → /users/{name}/earnings）。
type Unit3D struct {
	BaseHTMLAdapter
}

// NewUnit3D 构造适配器，注入共享 httpclient。
func NewUnit3D(hc *httpclient.Client) *Unit3D {
	return &Unit3D{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}}
}

// Schema 返回平台标识。
func (u *Unit3D) Schema() string { return "unit3d" }

// Name 实现 module.Module。
func (u *Unit3D) Name() string { return "site.unit3d" }

// Priority 实现 module.Module（数值越小越先）。
func (u *Unit3D) Priority() int { return 100 }

// Search 抓取 UNIT3D 种子列表页（/torrents?name=…）并解析。
// category 当前不发服务端 categoryIds[]（per-site 分类 ID 映射缺失）；行级后置过滤
// （丢弃音乐异类）由 chain 层 FilterByCategory 统一处理，非种子行（表头/广告）由 ParseUnit3DSearch 丢弃。
func (u *Unit3D) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def) // 选择器为空时填 UNIT3D 标准默认
	target := site.BuildUnit3DSearchURL(baseURL, def, keyword, page)
	resp, err := u.hc.Get(ctx, target, cookie, true)
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(resp.Body))
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	if resp.IsCloudflare() {
		return nil, fmt.Errorf("blocked by cloudflare on %s", baseURL)
	}
	return site.ParseUnit3DSearch(doc, def, baseURL), nil
}

// FetchUserInfo 抓取 UNIT3D 用户资料（三步流程，best-effort）：
//  1. GET /（兼登录探针）→ 发现用户名（顶部导航 settings 链接 href）
//  2. GET /users/{name} → ratio-bar + i18n dt/dd 统计（uploaded/downloaded/ratio/bonus/seeding/leeching/level/join_date）
//  3. GET /users/{name}/earnings → 每小时积分 bonus_per_hour
//
// 任一子页失败不致命：首页失败才整体失败；资料页/收益页失败仅对应字段留空/留 0。
func (u *Unit3D) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {

	def = site.EnsureDefaults(def)

	// 1. 首页：登录探针 + 发现用户名
	doc, resp, err := u.fetchDoc(ctx, baseURL, "/", cookie)
	if err != nil {
		return nil, err
	}
	if loggedOut(resp, doc, def.NoLogin) {
		return nil, site.ErrNotLoggedIn
	}
	name := site.ParseUnit3DUsername(doc, def)
	if name == "" {
		// root 已过 loggedOut，却没找到用户名 → 站点主题差异，非登录失败。
		return nil, fmt.Errorf("unit3d: username not found on index page")
	}
	out := &site.UserInfo{Name: name}

	// 2. 资料页 /users/{name}：ratio-bar + dt/dd 统计（best-effort，失败保留 step1 的 name）
	if udoc, _, e := u.fetchDoc(ctx, baseURL, "users/"+name, cookie); e == nil {
		if p := site.ParseUserInfo(udoc, def); p != nil {
			*out = *p
			if out.Name == "" {
				out.Name = name
			}
		}
	}

	// 3. 每小时积分 /users/{name}/earnings（best-effort，>0 才覆写）
	if edoc, _, e := u.fetchDoc(ctx, baseURL, "users/"+name+"/earnings", cookie); e == nil {
		if bph := site.ParseUnit3DBonusPerHour(edoc, def); bph > 0 {
			out.BonusPerHour = bph
		}
	}
	return out, nil
}

// TestConnection 用三步用户流程探测连接与登录态，返回用户等级。
func (u *Unit3D) TestConnection(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (bool, string, error) {
	ui, err := u.FetchUserInfo(ctx, def, baseURL, cookie)
	if err != nil {
		return false, "", err
	}
	return true, ui.Level, nil
}

// KeepAlive 实现 site.KeepAliveCapable：对首页做认证 GET 刷新 session，保活 cookie。
// 复用 keepAliveIndex（与 FetchUserInfo 同一「GET / + loggedOut」探针）。
func (u *Unit3D) KeepAlive(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.KeepAliveResult, error) {
	return u.keepAliveIndex(ctx, def, baseURL, cookie)
}
