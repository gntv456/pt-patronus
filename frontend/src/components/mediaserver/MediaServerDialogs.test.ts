// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：健康中心「巡检未完整覆盖」必须可见。
// 背景：后端把连不上的服务器/读不出的库静默跳过后返回空报告，前端照常渲染
// 「库很健康，未发现问题」+ 全 0 计数，用户据此不再复检（数据缺失被当成没有问题）。
// 用真实 Element Plus 挂载（el-dialog 走 teleport、el-table 空态由组件库渲染），
// 并配 errors 缺失的正控证明断言可判别。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

import MediaServerDialogs from './MediaServerDialogs.vue'

// mountAndSettle 挂载健康对话框并等真实组件渲染出表格空态
// （el-dialog 内容经 teleport 落到 body，断言统一在 body 子树里查）。
async function mountAndSettle(report: Record<string, unknown> | null): Promise<void> {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(MediaServerDialogs, {
    showHealth: true,
    healthReport: report,
    healthLoading: false,
    fixingSubtitles: false,
    fixSubtitles: () => {},
    editing: false,
    form: {},
    loading: false,
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(host)
  await vi.waitFor(() => {
    if (!document.body.querySelector('.el-table__empty-text')) {
      throw new Error('健康表格尚未渲染出空态')
    }
  })
}

function emptyText(): string {
  return document.body.querySelector('.el-table__empty-text')?.textContent?.trim() ?? ''
}

beforeEach(() => {
  document.body.replaceChildren()
})

describe('MediaServerDialogs · 健康中心巡检覆盖', () => {
  it('后端回传 errors 时给出警告横幅并逐条列出哪台/哪库没读到', async () => {
    await mountAndSettle({
      issues: [],
      unmatched: 0,
      missing_episodes: 0,
      missing_subtitle: 0,
      upgradeable: 0,
      checked_at: '2026-10-10T00:00:00Z',
      errors: [
        '飞牛影视：库列表读取失败（GET /Library/MediaFolders: HTTP 500）',
        'Emby：库「剧集」条目读取失败（超时）',
      ],
    })
    const alert = document.body.querySelector('.el-alert')
    expect(alert).not.toBeNull()
    expect(alert?.className).toContain('el-alert--warning')
    expect(alert?.textContent).toContain('巡检未完整覆盖')
    const lines = [...document.body.querySelectorAll('.scan-alert-line')].map((n) => n.textContent?.trim())
    expect(lines).toHaveLength(2)
    expect(lines[0]).toContain('飞牛影视')
    expect(lines[1]).toContain('剧集')
  })

  it('有 errors 时空态文案不再宣称库很健康', async () => {
    await mountAndSettle({
      issues: [],
      unmatched: 0,
      missing_episodes: 0,
      missing_subtitle: 0,
      errors: ['Emby：连接失败'],
    })
    expect(emptyText()).toContain('没读到任何库内容')
    expect(emptyText()).not.toContain('库很健康')
  })

  it('正控：errors 缺失（真的扫完了）时仍是「库很健康」且无警告横幅', async () => {
    await mountAndSettle({ issues: [], unmatched: 0, missing_episodes: 0, missing_subtitle: 0, checked_at: '' })
    expect(document.body.querySelector('.el-alert')).toBeNull()
    expect(emptyText()).toBe('库很健康，未发现问题')
  })
})
