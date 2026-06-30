package schemas

import (
	"context"
	"fmt"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// GenericJSON 是通用 JSON-API 适配器，服务命名 JSON schema（tnode/yemapt/f3nix/tbdev）。
// schema 参数化实例。选择器（JSON 点分路径）由 definition 驱动，走 site.ParseJSONSearch。
// cookie 鉴权（bearer-token 站需 per-schema credential() 扩展，留 M6）。
type GenericJSON struct {
	hc     *httpclient.Client
	schema string
}

func NewGenericJSON(hc *httpclient.Client, schema string) *GenericJSON {
	return &GenericJSON{hc: hc, schema: schema}
}

func (g *GenericJSON) Schema() string { return g.schema }
func (g *GenericJSON) Name() string   { return "site.generic_json." + g.schema }
func (g *GenericJSON) Priority() int  { return 200 }

// Search GET 搜索端点（BuildJSONSearchURL 处理路径占位/query 参数）→ ParseJSONSearch。
func (g *GenericJSON) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def)
	target := site.BuildJSONSearchURL(baseURL, def, keyword, page)
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
	return site.ParseJSONSearch(resp.Body, def, baseURL), nil
}

// FetchUserInfo JSON 站 YAML 普遍无 user_info → stub（不报错）。
func (g *GenericJSON) FetchUserInfo(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}
