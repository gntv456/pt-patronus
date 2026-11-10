// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 各下载器类型的能力位 + 文件优先级取值，集中管理。
// 新增下载器类型时在此加一条；UI 按 caps(type).xxx 门控，避免散落的 type === 'qb' 判断。
export type PrioOption = { label: string; value: number }

export type DlCaps = {
  category: boolean // 单种子 SetCategory
  categoryCRUD: boolean // 分类管理弹窗（Create/Edit/Delete）
  forceStart: boolean // 强制开始
  queueFull: boolean // 队列 top/up/down/bottom 全支持
  sequential: boolean // 顺序下载
  firstLast: boolean // 首尾优先
  swarm: boolean // 详情显示 swarm 总数 / 队列位置 / 可用性
  tags: boolean // 多标签
  location: boolean // 移动保存路径
  trackers: boolean // Tracker 增删查
  files: boolean // 文件列表与优先级
  peers: boolean // Peer 列表
  filePrio: PrioOption[] // 该类型可用文件优先级
}

export const DL_CAPS: Record<string, DlCaps> = {
  qb: {
    category: true,
    categoryCRUD: true,
    forceStart: true,
    queueFull: true,
    sequential: true,
    firstLast: true,
    swarm: true,
    tags: true,
    location: true,
    trackers: true,
    files: true,
    peers: true,
    filePrio: [
      { label: '不下载', value: 0 },
      { label: '正常', value: 2 },
      { label: '高', value: 1 },
      { label: '最高', value: 7 },
    ],
  },
  tr: {
    category: false,
    categoryCRUD: false,
    forceStart: false,
    queueFull: false,
    sequential: false,
    firstLast: false,
    swarm: false,
    tags: true,
    location: true,
    trackers: true,
    files: true,
    peers: true,
    filePrio: [
      { label: '低', value: -1 },
      { label: '正常', value: 0 },
      { label: '高', value: 1 },
    ],
  },
  ut: {
    category: true,
    categoryCRUD: false,
    forceStart: true,
    queueFull: true,
    sequential: false,
    firstLast: false,
    swarm: true,
    tags: false,
    location: false,
    trackers: false,
    files: true,
    peers: true,
    filePrio: [
      { label: '跳过', value: 0 },
      { label: '低', value: 1 },
      { label: '正常', value: 2 },
      { label: '高', value: 3 },
    ],
  },
  dl: {
    category: true,
    categoryCRUD: false,
    forceStart: false,
    queueFull: true,
    sequential: false,
    firstLast: false,
    swarm: true,
    tags: false,
    location: true,
    trackers: true,
    files: true,
    peers: true,
    filePrio: [
      { label: '跳过', value: 0 },
      { label: '低', value: 1 },
      { label: '正常', value: 2 },
      { label: '高', value: 3 },
    ],
  },
  ar: {
    category: false,
    categoryCRUD: false,
    forceStart: false,
    queueFull: true,
    sequential: false,
    firstLast: false,
    swarm: false,
    tags: false,
    location: false,
    trackers: false,
    files: true,
    peers: true,
    filePrio: [
      { label: '跳过', value: 0 },
      { label: '下载', value: 1 },
    ],
  },
  rt: {
    category: true,
    categoryCRUD: false,
    forceStart: false,
    queueFull: false,
    sequential: false,
    firstLast: false,
    swarm: false,
    tags: false,
    location: false,
    trackers: false,
    files: true,
    peers: true,
    filePrio: [
      { label: '不下载', value: 0 },
      { label: '正常', value: 1 },
      { label: '高', value: 2 },
    ],
  },
  fl: {
    category: true,
    categoryCRUD: false,
    forceStart: false,
    queueFull: false,
    sequential: true,
    firstLast: false,
    swarm: true,
    tags: true,
    location: true,
    trackers: true,
    files: true,
    peers: true,
    filePrio: [
      { label: '跳过', value: 0 },
      { label: '正常', value: 1 },
      { label: '高', value: 2 },
    ],
  },
}

// MINIMAL_CAPS 是未登记下载器类型的兜底（最保守：仅文件列表），替代旧 ?? DL_CAPS.qb
// ——避免新客户端未登记时 UI 显示一堆后端会 ErrNotImplemented 的按钮（与后端 caps.go 同源）。
const MINIMAL_CAPS: DlCaps = {
  category: false,
  categoryCRUD: false,
  forceStart: false,
  queueFull: false,
  sequential: false,
  firstLast: false,
  swarm: false,
  tags: false,
  location: false,
  trackers: false,
  files: true,
  peers: false,
  filePrio: [{ label: '正常', value: 1 }],
}

export const dlCaps = (type: string): DlCaps => DL_CAPS[type] ?? MINIMAL_CAPS
