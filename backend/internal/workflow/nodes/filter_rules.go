package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// FilterRulesNode 用规则引擎（对标 MP-2 filter module）按 rule_string 优先级层过滤并排序 ecx.Torrents。
// 与 FilterTorrentsNode（简单 include/exclude）并列：本节点支持级联优先级 + 22 内置规则 + 自定义规则组。
type FilterRulesNode struct {
	eng *filter.Engine
}

// NewFilterRulesNode 构造。eng 为规则引擎（filter.NewEngine）。
func NewFilterRulesNode(eng *filter.Engine) *FilterRulesNode {
	return &FilterRulesNode{eng: eng}
}

func (n *FilterRulesNode) Type() string     { return "filter_rules" }
func (n *FilterRulesNode) Name() string     { return "优先级规则过滤" }
func (n *FilterRulesNode) Category() string { return "过滤" }
func (n *FilterRulesNode) Description() string {
	return "按 MP-2 规则组（rule_string 优先级层 + 22 内置规则）过滤并排序资源"
}
func (n *FilterRulesNode) Icon() string { return "filter_rules" }
func (n *FilterRulesNode) DefaultParams() map[string]any {
	return map[string]any{"rule_groups": "", "media_type": ""}
}
func (n *FilterRulesNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "rule_groups", Label: "规则组", Type: "text",
			Help: "规则组名称，| 分隔多个；空=用全局订阅绑定（SubscribeFilterRuleGroups）"},
		{Name: "media_type", Label: "媒体类型", Type: "select", Options: []workflow.ParamOption{
			{Label: "自动", Value: ""}, {Label: "电影", Value: "movie"}, {Label: "电视剧", Value: "tv"},
		}},
	}
}

func (n *FilterRulesNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	if n.eng == nil {
		return fmt.Errorf("规则引擎未装配")
	}
	groups := splitPipeList(workflow.ParamString(params, "rule_groups"))
	if len(groups) == 0 {
		// 兜底：未指定则用全局订阅绑定。
		if gnames, _ := n.eng.Store().GetBindings(filter.KeySubscribeFilterRuleGroups); len(gnames) > 0 {
			groups = gnames
		}
	}
	if len(groups) == 0 || len(ecx.Torrents) == 0 {
		return nil // 无规则或无资源=不过滤
	}
	media := &filter.MediaInfo{Type: workflow.ParamString(params, "media_type")}
	ranked, err := n.eng.Filter(ctx, groups, ecx.Torrents, media)
	if err != nil {
		return fmt.Errorf("规则过滤失败: %w", err)
	}
	out := make([]site.Torrent, 0, len(ranked))
	for _, mt := range ranked {
		out = append(out, mt.Torrent)
	}
	ecx.Torrents = out
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("规则过滤后剩余 %d 条（按优先级排序）", len(out)))
	return nil
}

// splitPipeList 按管道符拆分（trim + 去空）。
func splitPipeList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "|") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
