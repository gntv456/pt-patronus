// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 站点定义诊断的展示映射（纯函数，便于单测；文案本体在 i18n 的 sites.diagKind.* / sites.diag*）。
import type { DefinitionDiagnostic } from '@/api/site'

// 后端 kind 的权威清单：internal/site/definitions_diag.go 的 Diag* 常量。
const KNOWN_KINDS = ['bad_regexp', 'unknown_key', 'duplicate_id', 'skipped_subdir', 'load_failed']

// diagKindKey 把后端 kind 映射到 i18n 键。未知 kind 一律落 `other`，
// 否则后端新增一类时界面会直接把键路径当文案显示出来。
export function diagKindKey(kind?: string): string {
  return KNOWN_KINDS.includes(kind || '') ? `sites.diagKind.${kind}` : 'sites.diagKind.other'
}

// diagTarget 给出定位对象（站点 id 优先，其次文件名）。
export function diagTarget(d?: DefinitionDiagnostic): string {
  if (!d) return ''
  return d.site || d.file || ''
}

// diagMoreCount 只列前 max 条时，剩下的条数（0 表示无需「还有 N 条」提示）。
export function diagMoreCount(total: number, max: number): number {
  return Math.max(0, total - max)
}
