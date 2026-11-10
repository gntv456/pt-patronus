// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// #382 筛选 composable 收按值数组 ⇒ computed 建不起依赖 ⇒ 数据到了也不重算。
//
// 复现自 Playwright core-flow E2E 的真实失败（core-flow.spec.ts:46）：
// 抽屉打开后表格恒显「0 / 1 个任务」+「无匹配任务」，而
//   - 统计条算出「任务总数 1」（同一个 list，只取 length，是活的）
//   - REST /torrents 反复 200，响应体里那颗种子字段完全正常
//   - 搜索框空、状态筛选「全部状态」、分类筛选未渲染、无「清除筛选」按钮
// 三个筛选条件都是默认值，按值传进来的数组又有 1 条 ⇒ filtered 本该是 1。
//
// 机制：useTorrentTableFilters(torrents: readonly Torrent[]) 收的是**快照**，
// filtered = computed(() => torrents.filter(...)) 只在**首次求值**时访问数组元素。
// 抽屉挂载那一刻 REST/WS 都还没回来 ⇒ 数组是空的 ⇒ computed 没收集到任何依赖
// ⇒ 之后 props.torrents 被替换/填充，filtered 永不重算，停在初始的 []。
//
// 典型 Vue 陷阱：把响应式数组按值传给 composable，而不是传 ref 或 getter。
// 单跑下载器用例不复现（REST 先于组件初始化返回），跑过搜索用例后打开抽屉必现——纯时序。
import { describe, expect, it } from 'vitest'
import { nextTick, ref, type Ref } from 'vue'
import type { Torrent } from '@/api/downloader'
import { useTorrentTableFilters } from './torrentTableFilters'

// seed 造最小可用的 Torrent（字段按 api/downloader.ts#Torrent 的必填项补齐）。
// 第一版只给了 4 个字段，vitest 不做类型检查所以照样绿，但 `vue-tsc` 判它不兼容 ——
// 判据文件自己过不了类型闸门是本仓吃过亏的（#361 假绿那一族）。
function seed(n: number): Torrent[] {
  return Array.from({ length: n }, (_, i) => ({
    hash: `h${i}`,
    name: `种子 ${i}`,
    size: 1024,
    progress: 0,
    ratio: 0,
    state: 'downloading',
    dl_rate: 0,
    up_rate: 0,
    save_path: '/data',
    category: '',
    tags: [] as readonly string[],
    num_seeds: 1,
    num_leechs: 0,
    num_complete: 1,
    num_incomplete: 0,
    availability: 0,
    ratio_limit: 0,
    seeding_time_limit: 0,
    force_start: false,
    auto_tmm: false,
    sequential: false,
    first_last_piece: false,
    priority: 0,
    added_at: '2026-01-01T00:00:00Z',
    completed_at: '0001-01-01T00:00:00Z',
  }))
}

describe('#382 筛选必须跟随数据源变化重算', () => {
  it('数据源从空变成 1 条时，filtered 必须跟着变（不许停在初始空数组）', () => {
    // 复刻 TorrentTable.vue 的用法：把响应式数组按 getter 传进去。
    const torrents = ref<Torrent[]>([])
    const f = useTorrentTableFilters(() => torrents.value)

    // 首次求值发生在「数据还没到」的时刻——正是抽屉刚挂载的状态。
    expect(f.filtered.value, '初始就该是空（数据确实没到）').toHaveLength(0)

    // 数据到了。组件重渲染、DOM 会重画，但 filtered 是 computed，必须自己重算。
    torrents.value = seed(1)
    expect(
      f.filtered.value,
      '数据源已有 1 条，filtered 却停在初始空数组：computed 没建立依赖（按值传数组的经典陷阱）',
    ).toHaveLength(1)
    expect(f.paged.value).toHaveLength(1)
  })

  it('反向对照：数据源缩回空时也要跟着变', async () => {
    const torrents = ref<Torrent[]>(seed(3))
    const f = useTorrentTableFilters(() => torrents.value)
    expect(f.filtered.value).toHaveLength(3)
    torrents.value = []
    await nextTick()
    expect(f.filtered.value, '数据源清空后 filtered 也要清空').toHaveLength(0)
  })

  it('筛选条件本身仍然生效（不许为了「跟着变」而放弃筛选）', () => {
    const torrents = ref<Torrent[]>(seed(3))
    const f = useTorrentTableFilters(() => torrents.value)
    f.search.value = '种子 1'
    expect(f.filtered.value.map((t) => t.name)).toEqual(['种子 1'])
    f.search.value = ''
    f.stateFilter.value = 'seeding'
    expect(f.filtered.value, 'downloading 的种子不该混进 seeding 档').toHaveLength(0)
  })
})
