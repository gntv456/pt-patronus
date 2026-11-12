// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// 资源过滤规则 API（对齐后端 internal/filter + api/v1/filter_rule.go）。
// 内置规则只读、自定义规则 CRUD、
// 规则组 CRUD（rule_string 表达式）、三个全局绑定（搜索/订阅/洗版）、规则试算。

// Rule —— 一条过滤规则（内置或自定义），对齐后端 filter.Rule。
// 内置规则的 include/exclude 是正则列表；自定义规则输入端为单条正则，入库后也是单元素列表。
export interface FilterRule {
  id: string
  name?: string
  include?: string[]
  exclude?: string[]
  match?: string[] // 属性重定向（如 ["labels"]）
  tmdb?: Record<string, string> // 元数据短路匹配
  size_range?: string // MB："1000-5000" | ">1000" | "<5000"
  seeders?: number // 最少做种人数
  downloadvolumefactor?: number // nil=不限；0=FREE
  publish_time?: string // 分钟："60" | "60-1440"
}

// CustomRuleInput —— 自定义规则 新建/编辑 请求体（对齐 filter.CustomRuleInput）。
// include/exclude 此处为单个正则字符串（用户在 UI 输入一段；多 alternation 写 a|b）。
export interface CustomRuleInput {
  id: string // 必填，仅字母数字
  name?: string
  include?: string
  exclude?: string
  size_range?: string
  seeders?: number
  publish_time?: string
}

// FilterRuleGroup —— 规则组定义，对齐后端 filter.RuleGroup。
export interface FilterRuleGroup {
  name: string
  rule_string: string // 表达式，如 "4K & !BLU > 1080P"
  media_type?: '' | '电影' | '电视剧'
  category?: string
}

// RuleGroupInput —— 规则组 新建/编辑 请求体（对齐 filter.RuleGroupInput）。
export interface RuleGroupInput {
  name: string // 必填
  rule_string: string // 必填
  media_type?: '' | '电影' | '电视剧'
  category?: string
}

// FilterBindings —— 三个全局绑定的组名列表（搜索/订阅/洗版）。
export interface FilterBindings {
  search: string[]
  subscribe: string[]
  best_version: string[]
}

// FilterTorrent —— 规则试算输入的种子（对齐后端 site.Torrent 子集）。
export interface FilterTorrent {
  title: string
  subtitle?: string
  size?: string
  seeders?: number
  free?: boolean
  tags?: string[]
}

// FilterMatched —— 试算命中结果：种子 + 命中的优先级（越大越优）。
export interface FilterMatched {
  torrent: FilterTorrent
  pri_order: number
}

export interface FilterTestReq {
  rule_string: string
  torrents: FilterTorrent[]
}

export interface FilterTestResult {
  matched: FilterMatched[]
  dropped: FilterTorrent[]
  levels: string[]
}

// 后端 filter.MatchedTorrent 结构体未加 json tag，默认序列化为 {Torrent, PriOrder}
// （首字母大写）。这里在 API 层归一化成小写键，组件侧统一用 m.torrent / m.pri_order，
// 既贴合任务契约 {torrent, pri_order}，又向后兼容若后端补 tag 后的小写形式。
interface RawMatched {
  Torrent?: FilterTorrent
  torrent?: FilterTorrent
  PriOrder?: number
  pri_order?: number
}

function normalizeMatched(m: RawMatched): FilterMatched {
  return {
    torrent: m.Torrent ?? m.torrent ?? { title: '' },
    pri_order: m.PriOrder ?? m.pri_order ?? 0,
  }
}

export const filterApi = {
  // —— 内置规则（只读）——
  builtins: () => http.get<{ rules: FilterRule[] }>('/filter/rules/builtin').then((r) => r.data.rules ?? []),

  // —— 自定义规则 ——
  custom: () => http.get<{ rules: FilterRule[] }>('/filter/rules/custom').then((r) => r.data.rules ?? []),
  createCustom: (body: CustomRuleInput) =>
    http.post<{ rule: FilterRule }>('/filter/rules/custom', body).then((r) => r.data.rule),
  updateCustom: (id: string, body: CustomRuleInput) =>
    http.put<{ rule: FilterRule }>(`/filter/rules/custom/${id}`, body).then((r) => r.data.rule),
  deleteCustom: (id: string) => http.delete(`/filter/rules/custom/${id}`).then((r) => r.data),

  // —— 规则组 ——
  groups: () => http.get<{ groups: FilterRuleGroup[] }>('/filter/groups').then((r) => r.data.groups ?? []),
  createGroup: (body: RuleGroupInput) =>
    http.post<{ group: FilterRuleGroup }>('/filter/groups', body).then((r) => r.data.group),
  updateGroup: (name: string, body: RuleGroupInput) =>
    http.put<{ group: FilterRuleGroup }>(`/filter/groups/${name}`, body).then((r) => r.data.group),
  deleteGroup: (name: string) => http.delete(`/filter/groups/${name}`).then((r) => r.data),

  // —— 全局绑定 ——
  bindings: () =>
    http.get<FilterBindings>('/filter/bindings').then((r) => ({
      search: r.data.search ?? [],
      subscribe: r.data.subscribe ?? [],
      best_version: r.data.best_version ?? [],
    })),
  setBindings: (b: FilterBindings) => http.put('/filter/bindings', b).then((r) => r.data),

  // —— 规则试算 ——
  test: (req: FilterTestReq) =>
    http.post<{ matched: RawMatched[]; dropped: FilterTorrent[]; levels: string[] }>('/filter/test', req).then((r) => ({
      matched: (r.data.matched ?? []).map(normalizeMatched),
      dropped: r.data.dropped ?? [],
      levels: r.data.levels ?? [],
    })),
}
