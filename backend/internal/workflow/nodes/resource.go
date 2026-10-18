// Package nodes 实现内置工作流节点（16 个）。每个节点是无状态单例，构造期注入所需 chain/oper，
// 在 app.go 注册到 workflow.Registry。节点间通过 *workflow.Context 传递状态。
// nodes→chain 是无环的（chain 只 import workflow 引擎，不 import nodes）。
package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"
)

// FetchRSSNode 解析一条 RSS/Atom feed → ecx.Torrents。site_id 非空则带站点 cookie 鉴权抓取。
type FetchRSSNode struct {
	hc     *httpclient.Client
	siteOp *oper.SiteOper
	fp     *gofeed.Parser
}

func NewFetchRSSNode(hc *httpclient.Client, siteOp *oper.SiteOper) *FetchRSSNode {
	return &FetchRSSNode{hc: hc, siteOp: siteOp, fp: gofeed.NewParser()}
}

func (n *FetchRSSNode) Type() string     { return "fetch_rss" }
func (n *FetchRSSNode) Name() string     { return "RSS 获取" }
func (n *FetchRSSNode) Category() string { return "资源" }
func (n *FetchRSSNode) Description() string {
	return "解析 RSS/Atom 订阅源，产出种子到资源集合"
}
func (n *FetchRSSNode) Icon() string { return "rss" }
func (n *FetchRSSNode) DefaultParams() map[string]any {
	return map[string]any{"url": "", "site_id": float64(0)}
}
func (n *FetchRSSNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "url", Label: "RSS 地址", Type: "text", Required: true, Help: "完整的 RSS/Atom feed URL"},
		{Name: "site_id", Label: "关联站点", Type: "site_select", Help: "PT 站 cookie 鉴权 RSS 选填，匿名 RSS 留空"},
	}
}

func (n *FetchRSSNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	rawURL := strings.TrimSpace(workflow.ParamString(params, "url"))
	if rawURL == "" {
		return fmt.Errorf("RSS 地址为空")
	}
	cookie := ""
	siteName := ""
	if sid := workflow.ParamInt(params, "site_id"); sid > 0 {
		if s, err := n.siteOp.Get(uint(sid)); err == nil && s != nil {
			siteName = s.Name
			if c, err := n.siteOp.Cookie(s); err == nil {
				cookie = c
			}
		}
	}
	resp, err := n.hc.Get(ctx, rawURL, cookie, false)
	if err != nil {
		return fmt.Errorf("抓取 RSS 失败: %w", err)
	}
	feed, err := n.fp.ParseString(string(resp.Body))
	if err != nil {
		return fmt.Errorf("解析 RSS 失败: %w", err)
	}
	// 大 feed 限流：护栏挡的是交给下游的种子数量（工作流可能逐条订阅/下载），不是解析内存。
	// 截断必须写进节点消息——只说「获取 200 条」会让下一节点和运行日志把上限当成整个 feed。
	added := 0
	scan := len(feed.Items)
	if scan > fetchRSSItemLimit {
		scan = fetchRSSItemLimit
	}
	for i := 0; i < scan; i++ {
		torr := itemToTorrent(feed.Items[i], siteName)
		if torr.DownloadURL == "" && torr.Title == "" {
			continue
		}
		ecx.Torrents = append(ecx.Torrents, torr)
		added++
	}
	msg := fmt.Sprintf("RSS 获取 %d 条", added)
	if len(feed.Items) > fetchRSSItemLimit {
		msg = fmt.Sprintf("RSS 获取 %d 条（feed 共 %d 条，仅取前 %d 条，其余本轮未取）", added, len(feed.Items), fetchRSSItemLimit)
	}
	ecx.Messages = append(ecx.Messages, msg)
	return nil
}

// fetchRSSItemLimit fetch_rss 节点单轮从 feed 取用的条目上限。
const fetchRSSItemLimit = 200

// itemToTorrent gofeed.Item → site.Torrent（优先 enclosure 的种子链接/大小）。
func itemToTorrent(item *gofeed.Item, siteName string) site.Torrent {
	t := site.Torrent{Title: item.Title, SiteName: siteName}
	guid := item.GUID
	if guid == "" {
		guid = item.Link
	}
	t.ID = guid
	t.DetailURL = item.Link
	// 种子下载链接：优先 enclosure，其次 link（可能是磁力）
	if len(item.Enclosures) > 0 {
		enc := item.Enclosures[0]
		t.DownloadURL = enc.URL
		t.Size = enc.Length
	}
	if t.DownloadURL == "" {
		t.DownloadURL = item.Link
	}
	return t
}

// SearchTorrentsNode 跨站搜索种子 → ecx.Torrents（复用 SiteChain.SearchSites，保留来源站名）。
type SearchTorrentsNode struct {
	siteCh *chain.SiteChain
}

func NewSearchTorrentsNode(siteCh *chain.SiteChain) *SearchTorrentsNode {
	return &SearchTorrentsNode{siteCh: siteCh}
}

func (n *SearchTorrentsNode) Type() string     { return "search_torrents" }
func (n *SearchTorrentsNode) Name() string     { return "搜索资源" }
func (n *SearchTorrentsNode) Category() string { return "资源" }
func (n *SearchTorrentsNode) Description() string {
	return "跨站关键字搜索种子，产出到资源集合"
}
func (n *SearchTorrentsNode) Icon() string { return "search" }
func (n *SearchTorrentsNode) DefaultParams() map[string]any {
	return map[string]any{"keyword": "", "category": ""}
}
func (n *SearchTorrentsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "keyword", Label: "关键字", Type: "text", Required: true},
		{Name: "category", Label: "分类", Type: "select", Options: []workflow.ParamOption{
			{Label: "全部", Value: ""},
			{Label: "电影", Value: "movie"},
			{Label: "剧集", Value: "tv"},
		}},
	}
}

func (n *SearchTorrentsNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	kw := strings.TrimSpace(workflow.ParamString(params, "keyword"))
	if kw == "" {
		return fmt.Errorf("搜索关键字为空")
	}
	results, err := n.siteCh.SearchSites(ctx, kw, workflow.ParamString(params, "category"), nil)
	if err != nil {
		return fmt.Errorf("跨站搜索失败: %w", err)
	}
	// 零站点参与 = 整轮什么都没搜（模板最常见的首次运行态），记成「搜索到 0 条」会被
	// 读成「搜过了、确实没有」，必须让节点失败出声。
	if len(results) == 0 {
		return fmt.Errorf("没有可搜索的站点（请先在「站点」页添加并登录）")
	}
	failed := 0
	var firstErr string
	for _, sr := range results {
		if sr.Error != "" {
			failed++
			if firstErr == "" {
				firstErr = sr.SiteName + "：" + sr.Error
			}
		}
	}
	if failed > 0 && failed == len(results) {
		return fmt.Errorf("%d 个站点搜索全部失败，未取得任何结果（首个：%s）", failed, firstErr)
	}
	total := 0
	for _, sr := range results {
		if sr.Error != "" {
			continue
		}
		for _, t := range sr.Torrents {
			if t.DownloadURL == "" {
				continue
			}
			if t.SiteName == "" {
				t.SiteName = sr.SiteName
			}
			t.SiteID = sr.SiteID
			ecx.Torrents = append(ecx.Torrents, t)
			total++
		}
	}
	msg := fmt.Sprintf("搜索到 %d 条资源", total)
	if failed > 0 {
		msg += fmt.Sprintf("（%d/%d 个站点搜索失败，首个：%s）", failed, len(results), firstErr)
	}
	ecx.Messages = append(ecx.Messages, msg)
	return nil
}

// FetchSiteStatsNode 读取站点最新统计 → ecx.Variables（上传/下载/分享率/魔力）。
type FetchSiteStatsNode struct {
	db *gorm.DB
}

func NewFetchSiteStatsNode(db *gorm.DB) *FetchSiteStatsNode {
	return &FetchSiteStatsNode{db: db}
}

func (n *FetchSiteStatsNode) Type() string        { return "fetch_site_stats" }
func (n *FetchSiteStatsNode) Name() string        { return "站点数据" }
func (n *FetchSiteStatsNode) Category() string    { return "资源" }
func (n *FetchSiteStatsNode) Description() string { return "读取站点最新统计快照到变量" }
func (n *FetchSiteStatsNode) Icon() string        { return "sites" }
func (n *FetchSiteStatsNode) DefaultParams() map[string]any {
	return map[string]any{"site_id": float64(0)}
}
func (n *FetchSiteStatsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "site_id", Label: "站点", Type: "site_select", Required: true},
	}
}

func (n *FetchSiteStatsNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	sid := workflow.ParamInt(params, "site_id")
	if sid <= 0 {
		return fmt.Errorf("未选择站点")
	}
	type statRow struct {
		Uploaded   int64   `gorm:"column:uploaded"`
		Downloaded int64   `gorm:"column:downloaded"`
		Ratio      float64 `gorm:"column:ratio"`
		Bonus      float64 `gorm:"column:bonus"`
	}
	var s statRow
	// Scan 在「零行」时不返回错误（ErrRecordNotFound 只有 First/Take 会给），
	// 于是没采过样的站点会带着全 0 变量「成功」下去：播报文案说成「上传 0 / 分享率 0.00」，
	// 下游条件分支也按这个假 0 决策。没有数据就是没有数据，必须报出来。
	res := n.db.Table("site_stats").Where("site_id = ?", sid).
		Order("snap_at DESC").Limit(1).Scan(&s)
	if res.Error != nil {
		return fmt.Errorf("读取站点统计失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("站点 %d 还没有采集到统计数据（先跑一次聚合快照）", sid)
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables["uploaded"] = s.Uploaded
	ecx.Variables["downloaded"] = s.Downloaded
	ecx.Variables["ratio"] = s.Ratio
	ecx.Variables["bonus"] = s.Bonus
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("站点统计：上传 %d / 下载 %d / 分享率 %.2f", s.Uploaded, s.Downloaded, s.Ratio))
	return nil
}
