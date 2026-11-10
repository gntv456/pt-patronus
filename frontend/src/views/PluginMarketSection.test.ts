// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 聚合市场「少一个源」必须在界面上看得见：后端只在部分源失败时返回残缺清单（全失败才 502），
// 从前 sources_failed 没人读，用户看到的与「仓库里就这些插件」完全同形。
import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import PluginMarketSection from './PluginMarketSection.vue'
import { marketSourceIssues } from './pluginsHelpers'
import type { PluginMarketIndex } from '@/api/plugin'

const index = (over: Partial<PluginMarketIndex> = {}): PluginMarketIndex => ({
  version: 1,
  plugins: [],
  sources_ok: 2,
  ...over,
})

describe('市场源账面', () => {
  it('整理失败源：缺字段退回占位符，不抛错', () => {
    expect(marketSourceIssues(undefined)).toEqual([])
    expect(marketSourceIssues(index())).toEqual([])
    expect(
      marketSourceIssues(
        index({
          sources_failed: [{ source: 'https://gh.example/repo', error: '上游 502' }, {} as never],
        }),
      ),
    ).toEqual([
      { source: 'https://gh.example/repo', reason: '上游 502' },
      { source: '-', reason: '-' },
    ])
  })

  it('部分源没取到时渲染告警；没有失败时不常亮', async () => {
    const mount = async (idx: PluginMarketIndex) => {
      const el = document.createElement('div')
      document.body.appendChild(el)
      const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
      const app = createApp(PluginMarketSection, {
        market: idx,
        loading: false,
        installing: '',
        marketSources: ['https://gh.example/repo', 'https://other.example/repo'],
        marketSourceVisible: false,
        marketSourceDraft: [],
        marketSourceOfficial: '',
        marketCategory: '',
        marketCategories: [],
        filteredMarketPlugins: idx.plugins,
        avatarColor: () => '',
        pluginInitial: (n: string) => n.slice(0, 1),
        marketView: 'all' as const,
        marketCharts: { disciples: [], newcomers: [] },
        favoritePlugins: [],
        instancesVisible: false,
        instancePluginId: '',
        instances: [],
        singleInstance: false,
      })
      app.use(i18n)
      app.use(ElementPlus)
      app.mount(el)
      await nextTick()
      await Promise.resolve()
      await nextTick()
      return el
    }

    const warn = await mount(
      index({
        sources_failed: [
          { source: 'https://gh.example/repo', error: 'parse market index: unexpected end of JSON input' },
        ],
      }),
    )
    const box = warn.querySelector<HTMLElement>('.market-src-warn')
    expect(box).not.toBeNull()
    const text = (box?.textContent || '').replace(/\s+/g, ' ').trim()
    expect(text).toContain('1 个市场源没取到')
    expect(text).toContain('https://gh.example/repo')
    expect(text).toContain('parse market index')

    const clean = await mount(index())
    expect(clean.querySelector('.market-src-warn')).toBeNull()

    while (document.body.firstChild) document.body.removeChild(document.body.firstChild)
  })
})

describe('榜单与收藏：读不出来 vs 真的没有（渲染层）', () => {
  const base = {
    market: { version: 1, plugins: [], sources_ok: 2 } as never,
    loading: false,
    installing: '',
    marketSources: ['https://gh.example/repo'],
    marketSourceVisible: false,
    marketSourceDraft: [],
    marketSourceOfficial: '',
    marketCategory: '',
    marketCategories: [],
    filteredMarketPlugins: [],
    avatarColor: () => '',
    pluginInitial: (n: string) => n.slice(0, 1),
    marketView: 'charts' as const,
    marketCharts: { disciples: [], newcomers: [] } as { disciples: never[]; newcomers: never[] } | null,
    favoritePlugins: [] as never[],
    instancesVisible: false,
    instancePluginId: '',
    instances: [],
    singleInstance: false,
  }

  const mountSection = async (over: Record<string, unknown> = {}) => {
    const el = document.createElement('div')
    document.body.appendChild(el)
    const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
    const app = createApp(PluginMarketSection, { ...base, ...over } as never)
    app.use(i18n)
    app.use(ElementPlus)
    app.mount(el)
    await nextTick()
    await Promise.resolve()
    await nextTick()
    return { text: el.textContent ?? '', app }
  }

  it('榜单没读出来（null）⇒ 说「读不出来」，不许说「暂无数据」', async () => {
    const { text, app } = await mountSection({ marketCharts: null })
    expect(text).toContain('读不出来')
    expect(text).not.toContain('暂无')
    app.unmount()
  })

  it('正控：榜单读到了但真的为空 ⇒ 仍说「暂无」，不亮读失败', async () => {
    const { text, app } = await mountSection({
      marketCharts: { disciples: [], newcomers: [] },
    })
    expect(text).toContain('暂无')
    expect(text).not.toContain('读不出来')
    app.unmount()
  })

  it('收藏没读出来 ⇒ 说「不是您没收藏过」；读到空列表才说空空如也', async () => {
    const bad = await mountSection({ marketView: 'favorites', favoritePlugins: null })
    expect(bad.text).toContain('不是您没收藏过')
    expect(bad.text).not.toContain('空空如也')
    bad.app.unmount()

    const empty = await mountSection({ marketView: 'favorites', favoritePlugins: [] })
    expect(empty.text).toContain('空空如也')
    expect(empty.text).not.toContain('不是您没收藏过')
    empty.app.unmount()
  })
})
