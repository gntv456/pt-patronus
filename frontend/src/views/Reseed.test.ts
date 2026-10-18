// @vitest-environment jsdom
// 渲染层回归：辅种候选列表后端单次最多返回 500 条。表头原先只写 tasks.length，
// 1300 条候选会被渲染成「候选任务（500）」——部分被当成全部。
// 后端现在同时给出 total/truncated，这里验前端确实把真实条数说出来。
import { describe, expect, it, vi } from 'vitest'
import { createApp } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  tasks: { tasks: [] as Record<string, unknown>[], total: null as number | null, truncated: false },
}))

vi.mock('@/api/reseed', () => ({
  reseedApi: {
    getConfig: vi.fn(async () => ({
      masked: '',
      configured: false,
      torznab_url: '',
      torznab_key_masked: '',
      torznab_configured: false,
    })),
    getSetting: vi.fn(async () => ({ downloader_id: 0, save_path: '', auto_reseed: false, auto_add: false, pause_after_add: false })),
    setSetting: vi.fn(async () => ({})),
    saveToken: vi.fn(async () => ({})),
    saveTorznab: vi.fn(async () => ({})),
    reseed: vi.fn(async () => ({ tasks: [], count: 0 })),
    tasks: vi.fn(async () => state.tasks),
    resolve: vi.fn(async () => ({})),
    findMissing: vi.fn(async () => ({ total: 0, checked: 0, targets: 0, missing: [] })),
  },
}))
vi.mock('@/api/downloader', () => ({ downloaderApi: { list: vi.fn(async () => []) } }))
vi.mock('@/api/site', () => ({ siteApi: { list: vi.fn(async () => []) } }))
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ push: vi.fn() }),
}))

import Reseed from './Reseed.vue'

function rows(n: number) {
  return Array.from({ length: n }, (_unused, i) => ({
    id: i + 1,
    downloader_id: 1,
    site_id: 0,
    source_name: `seed-${i}`,
    candidate_site: 'site-x',
    status: 'pending',
    created_at: new Date().toISOString(),
  }))
}

async function mountWith(payload: { tasks: Record<string, unknown>[]; total: number | null; truncated: boolean }): Promise<HTMLElement> {
  state.tasks = payload
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(Reseed)
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await vi.waitFor(() => {
    if (!el.textContent?.includes('候选任务')) throw new Error('候选表头未渲染')
  })
  return el
}

function headText(el: HTMLElement): string {
  const head = [...el.querySelectorAll('.card-head')].find((n) => n.textContent?.includes('候选任务'))
  return head?.textContent?.replace(/\s+/g, ' ') ?? ''
}

describe('辅种候选表头：被截断时要说清真实条数', () => {
  it('列表被截断时显示「最近 500 条 · 共 1303 条」', async () => {
    const el = await mountWith({ tasks: rows(500), total: 1303, truncated: true })
    const txt = headText(el)
    expect(txt).toContain('1303')
    expect(txt).toContain('500')
  })

  // 正控：没截断时不许凭空冒出「共 N 条」的口径（否则用户以为永远少了一批）。
  it('未截断时只显示条数本身', async () => {
    const el = await mountWith({ tasks: rows(12), total: 12, truncated: false })
    const txt = headText(el)
    expect(txt).toContain('（12）')
    expect(txt).not.toContain('共')
  })

  // 后端计数失败时 total 缺席：仍要承认「还有更多未列出」，不能装作这是全部。
  it('total 未知但已截断时说明还有更多', async () => {
    const el = await mountWith({ tasks: rows(500), total: null, truncated: true })
    expect(headText(el)).toContain('还有更多未列出')
  })
})
