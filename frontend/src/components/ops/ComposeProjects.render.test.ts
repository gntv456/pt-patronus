// @vitest-environment jsdom
// #218/#219 的渲染层收口：后端把「这次扫描不完整」放进 scan_error，
// 但字段进了 JSON 不等于上了屏幕。这里在真组件里证明两个出口都会把这句中文画出来
// （available=false 的空态、以及 available=true 但有条目被隐藏时的列表头）。
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

const state = vi.hoisted(() => ({
  list: { projects: [], scan_dir: '/srv/compose', available: false } as Record<string, unknown>,
}))

vi.mock('@/api/docker', () => ({
  dockerApi: {
    localProjects: vi.fn(async () => state.list),
    localProjectDetail: vi.fn(async () => ({ content: '', file: 'compose.yml', dir: '/srv/compose/x' })),
    localProjectAction: vi.fn(async () => ({ ok: true, output: '' })),
    localProjectSave: vi.fn(async () => ({ ok: true })),
  },
}))

const ComposeProjects = (await import('@/components/ops/ComposeProjects.vue')).default

async function mountPanel(): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({ render: () => h(ComposeProjects as never) })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await new Promise((r) => setTimeout(r, 0))
  await nextTick()
  return el
}

function textOf(el: HTMLElement): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

describe('ComposeProjects 把扫描失败的原因画上屏幕', () => {
  beforeEach(() => {
    state.list = { projects: [], scan_dir: '/srv/compose', available: false }
  })

  it('空态：读不出扫描根时，屏幕上要点名是哪个根、为什么', async () => {
    state.list = {
      projects: [],
      scan_dir: '/srv/compose',
      available: false,
      scan_error: 'compose 扫描根 /srv/compose 读不出来：permission denied（请确认容器启动时把宿主机目录挂到这里）',
    }
    const el = await mountPanel()
    const txt = textOf(el)
    expect(txt).toContain('读不出来')
    expect(txt).toContain('permission denied')
    // 不显示真实根路径时，下面的挂载提示会把用户引向程序根本没看过的目录（#219）。
    expect(el.querySelectorAll('.scan-error').length).toBe(1)
    expect(txt).toContain('/srv/compose')
  })

  it('列表态：有项目被隐藏时，列表头也要出声，且带上数量与项目名', async () => {
    state.list = {
      projects: [{ name: 'good', path: '/srv/compose/good/compose.yml', dir: '/srv/compose/good' }],
      scan_dir: '/srv/compose',
      available: true,
      scan_error: '1 个项目目录的 compose 文件读不出来，已从列表隐藏：broken-stack',
    }
    const el = await mountPanel()
    const txt = textOf(el)
    expect(txt).toContain('已从列表隐藏')
    expect(txt).toContain('broken-stack')
    expect(el.querySelectorAll('.scan-error').length).toBe(1)
    // 结构断言：项目行确实渲染了，不是靠空态分支蒙过去的
    expect(txt).toContain('good')
  })

  it('对照：干净的一次扫描不该出现任何告警节点', async () => {
    state.list = {
      projects: [{ name: 'good', path: '/srv/compose/good/compose.yml', dir: '/srv/compose/good' }],
      scan_dir: '/srv/compose',
      available: true,
    }
    const el = await mountPanel()
    expect(el.querySelectorAll('.scan-error').length).toBe(0)
    expect(textOf(el)).not.toContain('隐藏')
  })
})
