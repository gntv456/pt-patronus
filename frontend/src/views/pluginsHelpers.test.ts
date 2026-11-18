// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect } from 'vitest'
import { fmtTime, scheduleNotConfigured } from './pluginsHelpers'

// pluginsHelpers.fmtTime 用于插件列表/审计/日志等多处时间展示。
// 修复点：与 utils/format.fmtTime 对齐 Go time.Time 零值（0001-01-01，DB 脏数据时出现）→ '-'，
// 避免渲染出 "1-1-1 08:05:43" 这类无意义串。
describe('pluginsHelpers.fmtTime', () => {
  it('空值/undefined → -', () => {
    expect(fmtTime(undefined)).toBe('-')
    expect(fmtTime('')).toBe('-')
  })

  it('合法 ISO → 本地可读串（含年份，无 / 分隔）', () => {
    const s = fmtTime('2026-08-03T10:00:00Z')
    expect(s).toContain('2026')
    expect(s).not.toMatch(/\//)
  })

  it('非法日期 → -', () => {
    expect(fmtTime('not-a-date')).toBe('-')
  })

  it('Go time.Time 零值（0001-01-01）→ -（不再显示 1-1-1）', () => {
    expect(fmtTime('0001-01-01T00:00:00Z')).toBe('-')
  })
})


describe('pluginsHelpers.scheduleNotConfigured', () => {
  const schedule = [{ id: 'daily', action: 'run', cron: '0 3 * * *', cron_config_key: 'cron' }]

  it('未启用插件不提示', () => {
    expect(scheduleNotConfigured({ enabled: false, schedule, config: { cron: '' } })).toBe(false)
  })

  it('没有调度声明不提示', () => {
    expect(scheduleNotConfigured({ enabled: true, config: {} })).toBe(false)
  })

  it('cron 为空则提示未配置', () => {
    expect(scheduleNotConfigured({ enabled: true, schedule, config: { cron: '' } })).toBe(true)
    expect(scheduleNotConfigured({ enabled: true, schedule, config: {} })).toBe(true)
  })

  it('cron 已填写则不提示', () => {
    expect(scheduleNotConfigured({ enabled: true, schedule, config: { cron: '0 6 * * *' } })).toBe(false)
  })

  it('无 cron_config_key 时按 manifest 的 cron 判断', () => {
    const inline = [{ id: 'x', action: 'run', cron: '' }]
    expect(scheduleNotConfigured({ enabled: true, schedule: inline, config: {} })).toBe(true)
  })
})
