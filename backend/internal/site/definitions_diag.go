// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"sync"
)

// 站点定义诊断账面。
//
// 背景：同一个「配置被静默忽略」族已有四层护栏（#326 正则编不出来给空值、#327 启动期正则体检、
// #329 未知顶层键、#330 同目录重复 id / 子目录不递归），但四条**都只到日志层**——
// 用户在界面上看到的仍然只是「这个站抓不到东西」，看不出「有 N 条规则根本没生效」。
// 日志继续出声，这里同时记账，由 GET /api/v1/sites/definitions 的 diagnostics 字段交给界面读。

// 诊断类别。新增一类时同时补前端映射（frontend 的 kind 文案表），否则界面显示裸英文。
const (
	DiagSkippedSubdir = "skipped_subdir" // 子目录里的 *.yaml 一条都不读（加载器不递归）
	DiagUnknownKey    = "unknown_key"    // 顶层/嵌套键名结构体不认识 ⇒ 那块配置整个不生效
	DiagDuplicateID   = "duplicate_id"   // 同目录两个文件同 id ⇒ 前者被后者静默顶掉
	DiagBadRegexp     = "bad_regexp"     // regexp 族 pattern 编不出来 ⇒ 那条规则永远提不出值
	// load_failed：这一份定义读不出/解析失败 ⇒ 整个不生效（其余文件继续加载）。
	DiagLoadFailed = "load_failed"
	// upgrade_required：定义（远端源新站）要求的程序版本高于当前——跳过加载并
	// 提示升级，而不是放它进表后抓取报错。
	DiagUpgradeRequired = "upgrade_required"
)

// DefinitionDiagnostic 是一条可展示的配置失效记录。Detail 用人话，Site/File 用于定位。
type DefinitionDiagnostic struct {
	Kind   string `json:"kind"`
	Site   string `json:"site,omitempty"`
	File   string `json:"file,omitempty"`
	Detail string `json:"detail"`
}

// diagMu 只护 diags：定义在启动期写入、运行期只读，但端点随时可能被并发读。
type definitionDiagnostics struct {
	mu    sync.RWMutex
	items []DefinitionDiagnostic
}

func (r *Registry) recordDiag(d DefinitionDiagnostic) {
	r.diag.mu.Lock()
	defer r.diag.mu.Unlock()
	r.diag.items = append(r.diag.items, d)
}

// Diagnostics 返回累计的定义诊断（启动期加载 + 启动体检产生，运行期只读）。
func (r *Registry) Diagnostics() []DefinitionDiagnostic {
	r.diag.mu.RLock()
	defer r.diag.mu.RUnlock()
	out := make([]DefinitionDiagnostic, len(r.diag.items))
	copy(out, r.diag.items)
	return out
}

// ResetDiagnostics 清空累计诊断（仅测试与重载定义用：避免同一进程里多次加载把读数叠起来）。
func (r *Registry) ResetDiagnostics() {
	r.diag.mu.Lock()
	defer r.diag.mu.Unlock()
	r.diag.items = nil
}
