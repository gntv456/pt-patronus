// @vitest-environment jsdom
// 云端任务队列的账面渲染（#170 界面那一半）：接口把 total/truncated 交回来了，
// 这里挂 CloudRecordsTab 真组件，确认「最近 50 条 · 共 60 条」真的画在队列 tab 上，
// 而不是只活在 api/cloud.ts 的类型里。el-tab-pane 单独挂时拿不到 el-tabs 上下文就不渲染
// 内容，所以用透传 slot 的桩替掉外壳与表格——被测的仍是真模板里的 v-if 与文案。
import { describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import { createI18n } from 'vue-i18n'
import zhCN from '@/i18n/locales/zh-CN'

import CloudRecordsTab from './CloudRecordsTab.vue'
import type { CloudTask } from '@/api/cloud'

const passthrough = {
  name: 'passthrough',
  setup(_props: unknown, ctx: { slots: Record<string, ((...args: any[]) => any) | undefined> }) {
    return () => h('div', null, ctx.slots.default ? ctx.slots.default() : null)
  },
}
// 表格/下拉的具名插槽要 scoped 参数（row），桩直接调用会炸；它们不是被测对象，渲染空即可。
const nothing = { name: 'nothing', setup: () => () => h('div') }

type Meta = { total?: number; limit: number; truncated: boolean }

function tasks(n: number): CloudTask[] {
  return Array.from({ length: n }, (_, i) => ({
    id: i + 1,
    drive_id: 1,
    kind: 'upload',
    status: 'pending',
    cloud_path: `/t${i}`,
    local_path: '',
    error: '',
    size: 0,
    progress: 0,
    attempts: 0,
    created_at: '2026-10-12T00:00:00Z',
    updated_at: '2026-10-12T00:00:00Z',
  }))
}

function mountTab(meta: Meta, list: CloudTask[]): string {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const app = createApp(
    h(CloudRecordsTab, {
      records: [],
      tasks: list,
      tasksMeta: meta,
      loading: false,
      driveName: () => 'drive',
      fmtSize: () => '0 B',
      taskStatusFilter: '',
    }),
  )
  app.use(createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } }))
  for (const name of [
    'el-tabs',
    'el-tab-pane',
    'el-table',
    'el-table-column',
    'el-select',
    'el-option',
    'el-button',
    'el-tag',
    'el-icon',
    'el-empty',
    'el-progress',
  ]) {
    app.component(name, ['el-tabs', 'el-tab-pane'].includes(name) ? passthrough : nothing)
  }
  app.mount(el)
  const note = el.querySelector('.tasks-note')
  app.unmount()
  el.remove()
  return note ? (note.textContent ?? '').trim() : ''
}

describe('Cloud 任务队列：被上限截断要说出来', () => {
  it('truncated + total 可读：写清已显示几条、共几条', () => {
    const txt = mountTab({ total: 60, limit: 50, truncated: true }, tasks(50))
    expect(txt).toContain('最近 50 条')
    expect(txt).toContain('共 60 条')
  })

  it('truncated 而 total 读不出：说「还有更多未列出」，不编总数', () => {
    const txt = mountTab({ limit: 50, truncated: true }, tasks(50))
    expect(txt).toContain('最近 50 条')
    expect(txt).toContain('还有更多未列出')
    expect(txt).not.toContain('共 0 条')
  })

  it('正控：一次拿全时不渲染提示', () => {
    expect(mountTab({ total: 3, limit: 50, truncated: false }, tasks(3))).toBe('')
  })
})
