// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：调度面板要交代「表达式按哪个钟解释」和「下一次什么时候跑」。
// 背景：robfig/cron 用 time.Local，容器不设 TZ 就是 UTC —— 用户填 0 8 * * * 以为早八点跑，
// 实际本地下午四点，而面板此前只回显表达式本身，没人能发现。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import SchedulePanel from './SchedulePanel.vue'
import { useSchedule } from '@/composables/useSchedule'

const cfg = vi.hoisted(() => ({ value: {} as Record<string, unknown> }))

vi.mock('@/api/schedule', () => ({
  scheduleApi: {
    getConfig: () => Promise.resolve(cfg.value),
    setConfig: () => Promise.resolve({}),
  },
}))

async function renderPanel(): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(SchedulePanel)
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  app.use(ElementPlus)
  app.mount(el)
  await useSchedule().loadSchedule()
  await nextTick()
  await nextTick()
  return el
}

describe('调度面板：说清时区与下次执行', () => {
  beforeEach(() => {
    cfg.value = {}
  })

  it('后端给 UTC 时，面板显示时区、下次执行，并给出可操作的 TZ 提示', async () => {
    cfg.value = {
      snapshot_cron: '0 8 * * *',
      snapshot_default: '7 */4 * * *',
      keepalive_cron: '13 * * * *',
      keepalive_default: '13 * * * *',
      cron_timezone: 'UTC+00:00',
      tz_is_utc: true,
      server_time: '2026-10-12 01:00:00',
      snapshot_next: '2026-10-12T08:00:00Z',
      hint: '聚合快照…',
    }
    const text = (await renderPanel()).textContent?.replace(/\s+/g, ' ') ?? ''
    expect(text).toContain('UTC+00:00')
    expect(text).toContain('下次执行：2026-10-12T08:00:00Z')
    expect(text).toContain('TZ=Asia/Shanghai')
  })

  it('已是东八区时不该再弹 UTC 提醒', async () => {
    cfg.value = {
      snapshot_cron: '0 8 * * *',
      snapshot_default: '7 */4 * * *',
      keepalive_cron: '13 * * * *',
      keepalive_default: '13 * * * *',
      cron_timezone: 'UTC+08:00',
      tz_is_utc: false,
      server_time: '2026-10-12 09:00:00',
      snapshot_next: '2026-10-13T08:00:00+08:00',
      hint: '聚合快照…',
    }
    const text = (await renderPanel()).textContent?.replace(/\s+/g, ' ') ?? ''
    expect(text).toContain('UTC+08:00')
    expect(text).not.toContain('TZ=Asia/Shanghai')
  })
})
