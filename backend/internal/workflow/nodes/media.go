package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// 本文件新增「媒体」分类节点：媒体搜索、媒体详情、推荐信息流。
// 封装 MediaChain（TMDB/豆瓣/Bangumi 元数据）与 RecommendChain（热门/趋势信息流）。

// SearchMediaNode 搜索媒体元数据，标题列表写入文本载荷。
type SearchMediaNode struct {
	mediaCh *chain.MediaChain
}

func NewSearchMediaNode(mediaCh *chain.MediaChain) *SearchMediaNode {
	return &SearchMediaNode{mediaCh: mediaCh}
}

func (n *SearchMediaNode) Type() string        { return "search_media" }
func (n *SearchMediaNode) Name() string        { return "搜索媒体" }
func (n *SearchMediaNode) Category() string    { return "媒体" }
func (n *SearchMediaNode) Description() string { return "按关键词搜索影视/音乐元数据（写入文本载荷）" }
func (n *SearchMediaNode) Icon() string        { return "media" }
func (n *SearchMediaNode) DefaultParams() map[string]any {
	return map[string]any{"keyword": "", "year": float64(0), "filter": "video"}
}
func (n *SearchMediaNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "keyword", Label: "关键词", Type: "text", Required: true},
		{Name: "year", Label: "年份", Type: "number", Help: "0=不限"},
		{Name: "filter", Label: "类型", Type: "select", Options: []workflow.ParamOption{
			{Label: "影视", Value: "video"}, {Label: "电影", Value: "movie"}, {Label: "剧集", Value: "tv"}, {Label: "音乐", Value: "music"},
		}},
	}
}

func (n *SearchMediaNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	kw := strings.TrimSpace(workflow.ParamString(params, "keyword"))
	if kw == "" {
		return fmt.Errorf("关键词为空")
	}
	year := workflow.ParamInt(params, "year")
	filter := workflow.ParamString(params, "filter")
	list, err := n.mediaCh.Search(ctx, kw, year, filter)
	if err != nil {
		return fmt.Errorf("媒体搜索失败: %w", err)
	}
	writeJSON(ecx, list)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("媒体搜索「%s」：%d 条", kw, len(list)))
	return nil
}

// MediaDetailNode 取单条媒体详情，写入文本载荷。
type MediaDetailNode struct {
	mediaCh *chain.MediaChain
}

func NewMediaDetailNode(mediaCh *chain.MediaChain) *MediaDetailNode {
	return &MediaDetailNode{mediaCh: mediaCh}
}

func (n *MediaDetailNode) Type() string        { return "media_detail" }
func (n *MediaDetailNode) Name() string        { return "媒体详情" }
func (n *MediaDetailNode) Category() string    { return "媒体" }
func (n *MediaDetailNode) Description() string { return "取单条媒体详情（评分/演职/简介，写入文本载荷）" }
func (n *MediaDetailNode) Icon() string        { return "media" }
func (n *MediaDetailNode) DefaultParams() map[string]any {
	return map[string]any{"source": "tmdb", "source_id": ""}
}
func (n *MediaDetailNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "source", Label: "数据源", Type: "select", Options: []workflow.ParamOption{
			{Label: "TMDB", Value: "tmdb"}, {Label: "豆瓣", Value: "douban"}, {Label: "MusicBrainz", Value: "musicbrainz"},
		}},
		{Name: "source_id", Label: "源内 ID", Type: "text", Required: true, Help: "支持 {{var.x}}"},
	}
}

func (n *MediaDetailNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	source := workflow.ParamString(params, "source")
	if source == "" {
		source = "tmdb"
	}
	id := strings.TrimSpace(render(workflow.ParamString(params, "source_id"), ecx))
	if id == "" {
		return fmt.Errorf("源内 ID 为空")
	}
	mi, err := n.mediaCh.Detail(ctx, source, id)
	if err != nil {
		return fmt.Errorf("媒体详情失败: %w", err)
	}
	writeJSON(ecx, mi)
	title := ""
	if mi != nil {
		title = mi.Title
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("媒体详情：%s", title))
	return nil
}

// RecommendFeedNode 取推荐信息流，写入文本载荷。
type RecommendFeedNode struct {
	recommendCh *chain.RecommendChain
}

func NewRecommendFeedNode(recommendCh *chain.RecommendChain) *RecommendFeedNode {
	return &RecommendFeedNode{recommendCh: recommendCh}
}

func (n *RecommendFeedNode) Type() string        { return "recommend_feed" }
func (n *RecommendFeedNode) Name() string        { return "推荐信息流" }
func (n *RecommendFeedNode) Category() string    { return "媒体" }
func (n *RecommendFeedNode) Description() string { return "取热门/趋势推荐（豆瓣热映/TMDB 趋势等，写入文本载荷）" }
func (n *RecommendFeedNode) Icon() string        { return "recommend" }
func (n *RecommendFeedNode) DefaultParams() map[string]any {
	return map[string]any{"source": "", "category": "", "page": float64(1), "force": false}
}
func (n *RecommendFeedNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "source", Label: "数据源", Type: "text", Help: "留空=全部；如 douban/tmdb/bangumi"},
		{Name: "category", Label: "分类", Type: "text", Help: "如 留空/anime/duanju"},
		{Name: "page", Label: "页码", Type: "number", Default: float64(1)},
		{Name: "force", Label: "强制刷新", Type: "switch"},
	}
}

func (n *RecommendFeedNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	source := workflow.ParamString(params, "source")
	category := workflow.ParamString(params, "category")
	page := workflow.ParamInt(params, "page")
	force := workflow.ParamBool(params, "force")
	list := n.recommendCh.RecommendFeeds(ctx, source, category, page, force)
	writeJSON(ecx, list)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("推荐信息流：%d 条", len(list)))
	return nil
}
