package nodes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// 本文件新增资源类节点：跨站搜索、种子详情，以及对 ecx.Torrents 的排序/去重/择优。

// SearchAllSitesNode 跨所有启用站并发搜索，聚合结果进 ecx.Torrents。
type SearchAllSitesNode struct {
	siteCh *chain.SiteChain
}

func NewSearchAllSitesNode(siteCh *chain.SiteChain) *SearchAllSitesNode {
	return &SearchAllSitesNode{siteCh: siteCh}
}

func (n *SearchAllSitesNode) Type() string     { return "search_all_sites" }
func (n *SearchAllSitesNode) Name() string     { return "跨站搜索" }
func (n *SearchAllSitesNode) Category() string { return "资源" }
func (n *SearchAllSitesNode) Description() string {
	return "跨所有启用站点并发搜索，聚合进资源集合"
}
func (n *SearchAllSitesNode) Icon() string { return "search" }
func (n *SearchAllSitesNode) DefaultParams() map[string]any {
	return map[string]any{"keyword": "", "category": ""}
}
func (n *SearchAllSitesNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "keyword", Label: "关键词", Type: "text", Required: true},
		{Name: "category", Label: "类型", Type: "select", Options: []workflow.ParamOption{
			{Label: "综合", Value: ""}, {Label: "电影", Value: "movie"}, {Label: "剧集", Value: "tv"}, {Label: "音乐", Value: "music"},
		}},
	}
}

func (n *SearchAllSitesNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	kw := strings.TrimSpace(workflow.ParamString(params, "keyword"))
	if kw == "" {
		return fmt.Errorf("关键词为空")
	}
	category := workflow.ParamString(params, "category")
	results := n.siteCh.SearchSites(ctx, kw, category, nil)
	total := 0
	for _, r := range results {
		for i := range r.Torrents {
			if r.Torrents[i].SiteName == "" {
				r.Torrents[i].SiteName = r.SiteName
			}
		}
		ecx.Torrents = append(ecx.Torrents, r.Torrents...)
		total += len(r.Torrents)
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("跨站搜索「%s」：%d 条", kw, total))
	return nil
}

// TorrentDetailNode 抓取单条种子详情页，写入文本载荷。
type TorrentDetailNode struct {
	siteCh *chain.SiteChain
	siteOp *oper.SiteOper
}

func NewTorrentDetailNode(siteCh *chain.SiteChain, siteOp *oper.SiteOper) *TorrentDetailNode {
	return &TorrentDetailNode{siteCh: siteCh, siteOp: siteOp}
}

func (n *TorrentDetailNode) Type() string     { return "torrent_detail" }
func (n *TorrentDetailNode) Name() string     { return "种子详情" }
func (n *TorrentDetailNode) Category() string { return "资源" }
func (n *TorrentDetailNode) Description() string {
	return "抓取单条种子详情页（写入文本载荷）"
}
func (n *TorrentDetailNode) Icon() string { return "detail" }
func (n *TorrentDetailNode) DefaultParams() map[string]any {
	return map[string]any{"site_id": float64(0), "detail_url": ""}
}
func (n *TorrentDetailNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "site_id", Label: "站点", Type: "site_select", Required: true},
		{Name: "detail_url", Label: "详情页 URL", Type: "text", Required: true,
			Help: "支持 {{torrents.0.detail_url}}"},
	}
}

func (n *TorrentDetailNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	s, err := resolveSite(n.siteOp, params)
	if err != nil {
		return err
	}
	detailURL := strings.TrimSpace(render(workflow.ParamString(params, "detail_url"), ecx))
	if detailURL == "" {
		return fmt.Errorf("详情页 URL 为空")
	}
	detail, err := n.siteCh.TorrentDetail(ctx, s, detailURL)
	if err != nil {
		return fmt.Errorf("获取详情失败: %w", err)
	}
	writeJSON(ecx, detail)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("种子详情：%s", detailURL))
	return nil
}

// SortTorrentsNode 按字段排序 ecx.Torrents（纯逻辑）。
type SortTorrentsNode struct{}

func NewSortTorrentsNode() *SortTorrentsNode { return &SortTorrentsNode{} }

func (n *SortTorrentsNode) Type() string     { return "sort_torrents" }
func (n *SortTorrentsNode) Name() string     { return "排序资源" }
func (n *SortTorrentsNode) Category() string { return "资源" }
func (n *SortTorrentsNode) Description() string {
	return "按做种数/体积/完成数排序资源集合"
}
func (n *SortTorrentsNode) Icon() string { return "sort" }
func (n *SortTorrentsNode) DefaultParams() map[string]any {
	return map[string]any{"field": "seeders", "order": "desc"}
}
func (n *SortTorrentsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "field", Label: "字段", Type: "select", Options: []workflow.ParamOption{
			{Label: "做种数", Value: "seeders"}, {Label: "体积", Value: "size"}, {Label: "完成数", Value: "completed"},
		}},
		{Name: "order", Label: "方向", Type: "select", Options: []workflow.ParamOption{
			{Label: "降序", Value: "desc"}, {Label: "升序", Value: "asc"},
		}},
	}
}

func (n *SortTorrentsNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	field := workflow.ParamString(params, "field")
	if field == "" {
		field = "seeders"
	}
	desc := workflow.ParamString(params, "order") != "asc"
	sort.SliceStable(ecx.Torrents, func(i, j int) bool {
		a, b := ecx.Torrents[i], ecx.Torrents[j]
		var less bool
		switch field {
		case "size":
			less = site.ParseBytes(a.Size) < site.ParseBytes(b.Size)
		case "completed":
			less = a.Completed < b.Completed
		default: // seeders
			less = a.Seeders < b.Seeders
		}
		if desc {
			return !less
		}
		return less
	})
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("已按 %s %s 排序 %d 条", field, orderWord(desc), len(ecx.Torrents)))
	return nil
}

// DedupeTorrentsNode 按归一化标题去重，每组保留做种最多/体积最大者（纯逻辑）。
type DedupeTorrentsNode struct{}

func NewDedupeTorrentsNode() *DedupeTorrentsNode { return &DedupeTorrentsNode{} }

func (n *DedupeTorrentsNode) Type() string     { return "dedupe_torrents" }
func (n *DedupeTorrentsNode) Name() string     { return "去重资源" }
func (n *DedupeTorrentsNode) Category() string { return "资源" }
func (n *DedupeTorrentsNode) Description() string {
	return "按归一化标题去重，每组保留最优者"
}
func (n *DedupeTorrentsNode) Icon() string { return "dedupe" }
func (n *DedupeTorrentsNode) DefaultParams() map[string]any {
	return map[string]any{"keep": "seeders"}
}
func (n *DedupeTorrentsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "keep", Label: "每组保留", Type: "select", Options: []workflow.ParamOption{
			{Label: "做种最多", Value: "seeders"}, {Label: "体积最大", Value: "size"},
		}},
	}
}

func (n *DedupeTorrentsNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	keep := workflow.ParamString(params, "keep")
	seen := make(map[string]int, len(ecx.Torrents)) // key → 保留者在结果中的下标
	out := ecx.Torrents[:0]
	for _, t := range ecx.Torrents {
		key := normalizeTitle(t.Title)
		idx, ok := seen[key]
		if !ok {
			seen[key] = len(out)
			out = append(out, t)
			continue
		}
		cur := out[idx]
		if keep == "size" {
			if site.ParseBytes(t.Size) > site.ParseBytes(cur.Size) {
				out[idx] = t
			}
		} else {
			if t.Seeders > cur.Seeders {
				out[idx] = t
			}
		}
	}
	ecx.Torrents = out
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("去重后剩余 %d 条", len(out)))
	return nil
}

// PickBestNode 按质量评分（免费>分辨率>做种数）择优，保留前 N 条（纯逻辑）。
type PickBestNode struct{}

func NewPickBestNode() *PickBestNode { return &PickBestNode{} }

func (n *PickBestNode) Type() string     { return "pick_best" }
func (n *PickBestNode) Name() string     { return "择优" }
func (n *PickBestNode) Category() string { return "资源" }
func (n *PickBestNode) Description() string {
	return "按 免费→分辨率→做种数 评分，取质量最高的 N 条"
}
func (n *PickBestNode) Icon() string { return "best" }
func (n *PickBestNode) DefaultParams() map[string]any {
	return map[string]any{"count": float64(1)}
}
func (n *PickBestNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "count", Label: "取前 N 条", Type: "number", Default: float64(1)},
	}
}

func (n *PickBestNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	count := workflow.ParamInt(params, "count")
	if count <= 0 {
		count = 1
	}
	sort.SliceStable(ecx.Torrents, func(i, j int) bool {
		return torrentScore(ecx.Torrents[i]) > torrentScore(ecx.Torrents[j])
	})
	if len(ecx.Torrents) > count {
		ecx.Torrents = ecx.Torrents[:count]
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("择优取 %d 条", len(ecx.Torrents)))
	return nil
}

// torrentScore 资源质量评分。优先级 必须 与做种数量级无关：免费 > 分辨率 > 做种数，
// 故用大跨度乘子（免费 1<<40、分辨率 1<<30），做种数（现实中 <1e6）落在最低档作 tiebreaker。
func torrentScore(t site.Torrent) int64 {
	var score int64
	if t.Free {
		score += int64(1) << 40
	}
	score += int64(resolutionRank(t.Title)) << 30
	score += int64(t.Seeders)
	return score
}

func resolutionRank(title string) int {
	t := strings.ToUpper(title)
	switch {
	case strings.Contains(t, "2160") || strings.Contains(t, "4K") || strings.Contains(t, "UHD"):
		return 4
	case strings.Contains(t, "1080"):
		return 3
	case strings.Contains(t, "720"):
		return 2
	}
	return 0
}

// normalizeTitle 标题归一化：小写、去括号内容与常见噪声，便于去重分组。
func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '[', ']', '(', ')', '【', '】':
			continue
		}
		b.WriteRune(r)
	}
	s = b.String()
	for _, sep := range []string{".", "_", "-"} {
		s = strings.ReplaceAll(s, sep, " ")
	}
	s = strings.TrimSpace(s)
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

func orderWord(desc bool) string {
	if desc {
		return "降序"
	}
	return "升序"
}
