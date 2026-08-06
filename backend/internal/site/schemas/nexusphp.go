package schemas

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// HTML 抓取，选择器完全由 definition（YAML）驱动；内嵌 BaseHTMLAdapter 复用 fetchDoc。
type NexusPHP struct {
	BaseHTMLAdapter
	utlsOnce sync.Once
	utlsHC   *httpclient.Client
}

// NewNexusPHP 构造适配器，注入共享 httpclient。
func NewNexusPHP(hc *httpclient.Client) *NexusPHP {
	return &NexusPHP{BaseHTMLAdapter: BaseHTMLAdapter{hc: hc}}
}

// utlsClient 返回一个 uTLS（Chrome 指纹）httpclient，sync.Once lazy 构造。
// 供启用 checkin.utls 的 WAF 站签到 opt-in：标准 TLS 被 SafeLine 等按 TLS 指纹拦截（468），
// uTLS 伪装 Chrome ClientHello 绕过。构造失败返回 nil，调用方回退标准 hc。
func (n *NexusPHP) utlsClient() *httpclient.Client {
	n.utlsOnce.Do(func() {
		if c, err := httpclient.New(httpclient.Options{FollowRedirect: true, MaxRetries: 2, UTLS: true}); err == nil {
			n.utlsHC = c
		}
	})
	return n.utlsHC
}

// Schema 返回平台标识。
func (n *NexusPHP) Schema() string { return "nexusphp" }

// Name 实现 module.Module。
func (n *NexusPHP) Name() string { return "site.nexusphp" }

// Priority 实现 module.Module（数值越小越先）。
func (n *NexusPHP) Priority() int { return 100 }

// Search 抓取搜索结果页并解析为种子列表。
// category 非空时 BuildSearchURL 据站点 search_categories 追加 cat[]（服务端精准过滤）；
// 行级后置过滤（丢弃音乐等异类）由 chain 层 FilterByCategory 统一处理。
func (n *NexusPHP) Search(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie, keyword, category string, page int) ([]site.Torrent, error) {

	def = site.EnsureDefaults(def) // 选择器为空时自动填 NexusPHP 标准默认值
	u := site.BuildSearchURL(baseURL, def, keyword, category, page)
	resp, err := n.hc.Get(ctx, u, cookie, true)
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
	return site.ParseSearch(doc, def, baseURL), nil
}

// TestConnection 用用户页探测连接与登录态，返回用户等级。
func (n *NexusPHP) TestConnection(ctx context.Context, def *site.SiteMetadata,
	baseURL, cookie string) (bool, string, error) {
	ui, err := n.FetchUserInfo(ctx, def, baseURL, cookie)
	if err != nil {
		return false, "", err
	}
	return true, ui.Level, nil
}
