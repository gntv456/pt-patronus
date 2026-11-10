// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package workflow

import "sort"

// Registry 是节点注册表（构造期在 app.go 注入依赖并 Register 全部节点）。
type Registry struct {
	nodes map[string]Node
}

// NewRegistry 构造。
func NewRegistry() *Registry { return &Registry{nodes: make(map[string]Node)} }

// Register 注册一个节点（按 Type 键；重复注册覆盖）。
func (r *Registry) Register(n Node) {
	if n == nil {
		return
	}
	r.nodes[n.Type()] = n
}

// Get 按类型键取节点。
func (r *Registry) Get(t string) (Node, bool) {
	n, ok := r.nodes[t]
	return n, ok
}

// List 返回节点目录（按 Category→Name 排序），供 /workflows/actions 展示与前端表单渲染。
func (r *Registry) List() []NodeInfo {
	out := make([]NodeInfo, 0, len(r.nodes))
	for _, n := range r.nodes {
		out = append(out, NodeInfo{
			Type:          n.Type(),
			Name:          n.Name(),
			Category:      n.Category(),
			Description:   n.Description(),
			Icon:          n.Icon(),
			DefaultParams: n.DefaultParams(),
			ParamSchema:   n.ParamSchema(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}
