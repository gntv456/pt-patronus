// @vitest-environment jsdom
// 运维命令批次台账的窗口说明必须真的画在面板里：#181 只做到了 props 与类型，
// 这里补渲染层——截断时说清「共多少次、只列了多少次」，总数读不出换成另一句。
import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import NasExecPanel from '@/components/nas/NasExecPanel.vue'

const rows = (n: number) =>
  Array.from({ length: n }, (_, i) => ({
    id: i + 1,
    target_id: 1,
    command: 'uptime',
    status: 'ok',
    created_at: '2026-10-13T00:00:00Z',
  }))

async function mountWith(extra: Record<string, unknown>): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({
    render: () =>
      h(NasExecPanel as never, {
        execTargetIds: [1],
        sshTargets: [],
        selectedExecTargets: [],
        selectedCommandRun: null,
        execCommand: '',
        execTimeout: 30,
        execRunning: false,
        commandHistory: [],
        commandRuns: rows(20),
        execLogs: [],
        isAdmin: true,
        ...extra,
      } as never),
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  return el
}

function note(el: HTMLElement): string {
  return (el.querySelector('.runs-note')?.textContent ?? '').replace(/\s+/g, ' ').trim()
}

describe('NasExecPanel 批次台账窗口说明（渲染层）', () => {
  it('被单页上限截断：说出「共 137 次，只列最近 20 次」', async () => {
    const el = await mountWith({ runsTruncated: true, runsTotal: 137 })
    expect(note(el)).toContain('仅列最近 20 次批次')
    expect(note(el)).toContain('共 137 次')
  })

  it('总数读不出：换成「总数读取失败——可能还有更多」，不写共 0 次', async () => {
    const el = await mountWith({ runsTruncated: true, runsTotal: undefined })
    const t = note(el)
    expect(t).toContain('总数读取失败')
    expect(t).not.toContain('共 0 次')
  })

  it('正控：一次拿全时不画这条说明', async () => {
    const el = await mountWith({ runsTruncated: false, runsTotal: 20 })
    expect(el.querySelector('.runs-note')).toBeNull()
  })
})
