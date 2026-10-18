// @vitest-environment jsdom
// #250 的渲染层收口：本机 Docker 起起来之后，用户要有办法让界面知道。
// 「本机」按钮在 localAvailable=false 时是 disabled 的，探测又只在挂载发一次 ⇒ 按下 Docker 之后
// 按钮永远灰着，只能整页刷新。这里在真组件里跑一个前后 A/B：同一个布尔量（按钮 disabled）
// 从 true 变 false，且那句后端给的 DOCKER_HOST 出路先上屏、解锁后消失。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import { a11yButton } from '@/directives/a11yButton'

const state = vi.hoisted(() => ({
  // 每次探测按顺序取这里的一条；用尽后重复最后一条。
  probes: [] as Array<Record<string, unknown>>,
  probeCalls: 0,
  localListCalls: 0,
  // 非空即让 local/stats 以「后端给的中文原因」失败，用来验停更横幅上不上屏。
  statsFail: '' as string,
  localList: { containers: [], images: [], volumes: [], networks: [] } as Record<string, unknown>,
}))

vi.mock('@/api/docker', () => {
  const api = new Proxy({} as Record<string, unknown>, {
    get(_t, key: string) {
      if (key === 'localStatus') {
        return () => {
          const p = state.probes[Math.min(state.probeCalls, state.probes.length - 1)]
          state.probeCalls += 1
          return Promise.resolve(p)
        }
      }
      if (key === 'localList') {
        return () => {
          state.localListCalls += 1
          return Promise.resolve(state.localList)
        }
      }
      if (key === 'localStats') {
        return () =>
          state.statsFail ? Promise.reject({ response: { data: { error: state.statsFail } } }) : Promise.resolve({ stats: [] })
      }
      if (key === 'localProjects') return () => Promise.resolve({ projects: [] })
      return () => Promise.resolve({})
    },
  })
  return { dockerApi: api, dockerRemoteActions: api }
})

// 远程主机列表为空：这就是「只跑本机 Docker、没配 SSH」的常见形态（Windows/macOS 原生运行）。
vi.mock('@/api/monitor', () => ({ monitorApi: { list: () => Promise.resolve([]) } }))

const OpsDocker = (await import('@/components/ops/OpsDocker.vue')).default

const NOT_RUNNING = {
  available: false,
  endpoint: 'unix:///var/run/docker.sock',
  error:
    'socket 文件不存在：/var/run/docker.sock（Windows/macOS 原生运行本程序时该地址不存在，可设 DOCKER_HOST=tcp://127.0.0.1:2375 后重启）',
}
const RUNNING = { available: true, endpoint: 'unix:///var/run/docker.sock' }

let app: ReturnType<typeof createApp> | undefined

async function mountPanel(): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  app = createApp({ render: () => h(OpsDocker as never) })
  app.use(i18n)
  app.use(ElementPlus)
  app.directive('a11y-button', a11yButton)
  app.mount(el)
  await flush()
  return el
}

// 挂载后有一串 await（loadTargets → 探测 → 可选 refresh），逐轮 tick 让它们跑完。
async function flush(): Promise<void> {
  for (let i = 0; i < 6; i += 1) {
    await nextTick()
    await new Promise((r) => setTimeout(r, 0))
  }
}

function localBtn(el: HTMLElement): HTMLButtonElement {
  return el.querySelectorAll('.mode-btn')[0] as HTMLButtonElement
}
function refreshBtn(el: HTMLElement): HTMLButtonElement {
  const btn = [...el.querySelectorAll('.docker-toolbar button')].find((b) => (b.textContent ?? '').trim() === '刷新')
  if (!btn) throw new Error('工具栏里没有「刷新」按钮')
  return btn as HTMLButtonElement
}
function textOf(el: HTMLElement): string {
  return (el.textContent ?? '').replace(/\s+/g, ' ')
}

describe('本机 Docker 起来之后，界面要有重新探测的入口', () => {
  beforeEach(() => {
    state.probes = []
    state.probeCalls = 0
    state.localListCalls = 0
    state.statsFail = ''
    state.localList = { containers: [], images: [], volumes: [], networks: [] }
  })
  afterEach(() => {
    // 轮询定时器由组件自己回收；不卸载会让 5 秒轮询跨用例发请求。
    app?.unmount()
    app = undefined
  })

  it('挂载只发一次探测，屏幕上是具体的出路而不是泛化提示', async () => {
    state.probes = [NOT_RUNNING]
    const el = await mountPanel()
    expect(state.probeCalls).toBe(1)
    const txt = textOf(el)
    expect(txt).toContain('DOCKER_HOST')
    // localHint 有值时不该再退回那句「socket 未挂载」泛化文案。
    expect(txt).not.toContain('Docker socket 未挂载')
  })

  it('引擎不在时「本机」按钮是灰的', async () => {
    state.probes = [NOT_RUNNING]
    const el = await mountPanel()
    expect(localBtn(el).disabled).toBe(true)
  })

  it('按下 Docker 后点刷新 ⇒ 本机按钮解禁、那句停更提示消失', async () => {
    state.probes = [NOT_RUNNING, RUNNING]
    const el = await mountPanel()
    expect(localBtn(el).disabled).toBe(true)
    expect(textOf(el)).toContain('DOCKER_HOST')

    refreshBtn(el).click()
    await flush()

    expect(localBtn(el).disabled).toBe(false)
    expect(textOf(el)).not.toContain('DOCKER_HOST')
    // 解锁不只是按钮：本机列表确实被拉了一次。
    expect(state.localListCalls).toBeGreaterThanOrEqual(1)
  })

  it('点「本机」也会重新探测（切走再切回不靠旧判定）', async () => {
    state.probes = [NOT_RUNNING, RUNNING, RUNNING]
    const el = await mountPanel()
    expect(state.probeCalls).toBe(1)
    // 先解锁，才能点那个原本 disabled 的按钮——真实用户的顺序就是这样。
    refreshBtn(el).click()
    await flush()
    expect(localBtn(el).disabled).toBe(false)

    localBtn(el).click()
    await flush()
    expect(state.probeCalls).toBe(3)
  })

  // #245 的渲染层收口：横幅那句是 i18n 模板 + 后端原文拼出来的，字段进了 ref 不等于上了屏幕。
  it('统计读失败时屏幕上有「停更」横幅，原因用后端给的原文', async () => {
    state.probes = [RUNNING]
    state.statsFail = '连不上本机 Docker 引擎：默认走 unix:///var/run/docker.sock'
    const el = await mountPanel()
    const txt = textOf(el)
    expect(txt).toContain('资源读数已停更')
    expect(txt).toContain('连不上本机 Docker 引擎')
    // 从没成功过 ⇒ 不许编一个「上次成功时间」出来。
    expect(txt).toContain('--:--:--')
  })
})
