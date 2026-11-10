// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import { generateAllOutcome, subSyncOutcome } from './useCloudStrmRules'

// 「全部生成」的结果提示：后端回跨规则汇总后，提示要能区分
// 成功 / 没有规则 / 部分或全部规则失败，而不是照旧一句「已触发」。
describe('generateAllOutcome', () => {
  it('全部规则失败时报错并带首条原因', () => {
    const r = generateAllOutcome({
      rules: 2,
      generated: 0,
      skipped: 0,
      file_errors: 0,
      failed: ['rule-movie: 网盘不存在'],
    })
    expect(r.kind).toBe('error')
    expect(r.text).toContain('2 条规则生成失败')
    expect(r.text).toContain('网盘不存在')
  })

  it('成功时报出条数与跳过数', () => {
    const r = generateAllOutcome({ rules: 3, generated: 12, skipped: 40, file_errors: 0 })
    expect(r.kind).toBe('success')
    expect(r.text).toContain('12')
    expect(r.text).toContain('40')
    expect(r.text).not.toContain('失败')
  })

  it('有单文件失败时成功提示里也要留痕', () => {
    const r = generateAllOutcome({ rules: 1, generated: 5, skipped: 0, file_errors: 2 })
    expect(r.kind).toBe('success')
    expect(r.text).toContain('单文件失败 2')
  })

  it('没有启用规则说「无事可做」而不是失败', () => {
    const r = generateAllOutcome({ rules: 0, generated: 0, skipped: 0, file_errors: 0 })
    expect(r.kind).toBe('info')
    expect(r.text).toContain('没有启用中的 STRM 规则')
  })

  // 后端不可达/旧版返回 undefined 时不得抛异常，也不得谎报成功。
  it('汇总缺失时降级为 info', () => {
    const r = generateAllOutcome(undefined)
    expect(r.kind).toBe('info')
    expect(r.text).not.toContain('完成')
  })
})

// 「订阅追更」的结果提示：后端已改回跨轮汇总（修复前恒 {ok:true}），
// 提示必须能区分「读失败/无订阅/无记录/零命中/真链接了」，否则又回到一句「匹配完成」。
describe('subSyncOutcome', () => {
  it('整轮没跑成时报错并带原因', () => {
    const r = subSyncOutcome({ subs: 0, records: 0, linked: 0, failed: ['读取订阅列表失败: database is locked'] })
    expect(r.kind).toBe('error')
    expect(r.text).toContain('订阅追更未能完成')
    expect(r.text).toContain('database is locked')
  })

  it('没有生效中的订阅说「无需追更」而不是成功', () => {
    const r = subSyncOutcome({ subs: 0, records: 0, linked: 0 })
    expect(r.kind).toBe('info')
    expect(r.text).toContain('没有生效中的订阅')
  })

  it('无待匹配记录时指出去哪一步补', () => {
    const r = subSyncOutcome({ subs: 3, records: 0, linked: 0 })
    expect(r.kind).toBe('info')
    expect(r.text).toContain('STRM 记录')
  })

  it('零命中也要如实说没有新命中', () => {
    const r = subSyncOutcome({ subs: 2, records: 17, linked: 0 })
    expect(r.kind).toBe('success')
    expect(r.text).toContain('没有新命中')
    expect(r.text).toContain('17')
  })

  it('链接成功时报出三个数', () => {
    const r = subSyncOutcome({ subs: 2, records: 17, linked: 4 })
    expect(r.kind).toBe('success')
    expect(r.text).toContain('2 条订阅')
    expect(r.text).toContain('17 条待匹配记录')
    expect(r.text).toContain('新链接 4')
  })

  it('后端不可达/旧版返回 undefined 时不得谎报成功', () => {
    const r = subSyncOutcome(undefined)
    expect(r.kind).toBe('error')
  })
})
