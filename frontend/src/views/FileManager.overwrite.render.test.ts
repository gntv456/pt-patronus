// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// #244 的最后一米：判据与接线用例都绿了，但「用户真的看得见这个确认框」没证过。
// 这里挂真 FileManager.vue 走真交互链：行内「移动」→ 对话框「移动到此处」→ doMoveCopy 的同名预检
// ⇒ 断言 document.body 上真的渲染出那句话（ElMessageBox 是 append-to-body 的遮罩，
// 读组件根拿不到），并盯住「取消就不许动、同意才继续」。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import { createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'
import zhCN from '@/i18n/locales/zh-CN'
import { a11yButton } from '@/directives/a11yButton'

const state = vi.hoisted(() => ({
  // 预检阶段的可控开关：列目录整批失败 / 目标里点名有这个名字。
  failFiles: false,
  // 非空 ⇒ 列目录返回「里面有这一个名字」，用来分别造「无同名」与「真同名」两种目标。
  overrideName: '' as string,
  moveCalls: [] as string[],
  copyCalls: [] as string[],
}))

vi.mock('@/api/filemanager', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/api/filemanager')>()
  const ent = (name: string, isDir: boolean) => ({
    name,
    path: (isDir ? '' : '/media') + '/' + name,
    is_dir: isDir,
    size: 3,
    mtime: 0,
  })
  return {
    ...actual,
    fileApi: {
      ...actual.fileApi,
      list: async () => [{ id: 1, name: '本地盘', type: 'local', root_path: '/' }],
      test: async () => ({ ok: true }),
      files: async (_id: number, path: string) => {
        // 后端 filemanager.go:105 读失败时回 200 + {entries: [], error:'…'}，
        // api 层（#244）把它抛成异常——这里就造异常，模拟「读不到」。
        if (state.failFiles) throw new Error('WebDAV 目录列举失败：403 Forbidden')
        if (state.overrideName) return [ent(state.overrideName, false)]
        return path === '/' ? [ent('media', true), ent('data', true)] : [ent('a.mkv', false)]
      },
      downloadUrl: () => '',
      move: async (_id: number, src: string) => {
        state.moveCalls.push(src)
        return { ok: true }
      },
      copy: async (_id: number, src: string) => {
        state.copyCalls.push(src)
        return { ok: true }
      },
    },
  }
})
vi.mock('@/api/cloud', () => ({
  cloudApi: { listDrives: async () => [], test: async () => ({ ok: true }), browse: async () => [] },
}))

const FileManager = (await import('@/views/FileManager.vue')).default

let app: ReturnType<typeof createApp> | undefined

async function settle(times = 8): Promise<void> {
  for (let i = 0; i < times; i += 1) {
    await nextTick()
    await new Promise((r) => setTimeout(r, 0))
  }
}

async function mountView(): Promise<HTMLElement> {
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  app = createApp({ render: () => h(FileManager as never, { embedded: true }) })
  const pinia = createPinia()
  app.use(pinia)
  app.use(i18n)
  app.use(ElementPlus)
  // 存储列表被 auth.isAdmin 钳制（useFileManager.ts:124）：不种管理员就一行文件都画不出来，
  // 那样用例只会红在「找不到按钮」，红得没道理。
  useAuthStore().user = { id: 1, username: 'admin', is_admin: true } as never
  app.directive('a11y-button', a11yButton)
  app.mount(el)
  await settle()
  return el
}

function buttons(root: ParentNode, label: string): HTMLButtonElement[] {
  return [...root.querySelectorAll('button')].filter(
    (b) => (b.textContent ?? '').trim() === label,
  ) as HTMLButtonElement[]
}

/** 屏幕全文：遮罩挂在 document.body 上，只读组件根会漏掉确认框。 */
function screenText(): string {
  return (document.body.textContent ?? '').replace(/\s+/g, ' ')
}

/** 用户路径：根目录列表里有 media/ 与 data/ 两行，点第二行「data」的行内「移动」。 */
async function openMoveDialog(el: HTMLElement): Promise<void> {
  const rowBtns = buttons(el, '移动')
  expect(rowBtns.length, '至少要有一个「移动」入口').toBeGreaterThan(0)
  rowBtns[rowBtns.length - 1].click()
  await settle()
}

async function clickMoveHere(el: HTMLElement): Promise<void> {
  // el-dialog 是 append-to-body：对话框不在组件根里，必须整页查（只在 el 里查会永远找不到）。
  void el
  const okBtn = buttons(document, '移动到此处')
  expect(okBtn.length, '对话框 footer 要有「移动到此处」').toBeGreaterThan(0)
  okBtn[okBtn.length - 1].click()
  await settle()
}

async function clickMessageBox(which: 'cancel' | 'ok'): Promise<void> {
  const btns = [...document.querySelectorAll('.el-message-box__btns button')] as HTMLButtonElement[]
  expect(btns.length, '确认框要有取消/确定两个按钮').toBeGreaterThanOrEqual(2)
  // EP 的默认顺序是取消在前、确定在后。
  ;(which === 'cancel' ? btns[0] : btns[btns.length - 1]).click()
  await settle()
}

function confirmBoxCount(): number {
  return document.querySelectorAll('.el-message-box__btns').length
}

describe('文件管理器读不到目标目录时，屏幕上真弹覆盖确认', () => {
  beforeEach(() => {
    state.failFiles = false
    state.overrideName = ''
    state.moveCalls = []
    state.copyCalls = []
  })
  afterEach(() => {
    app?.unmount()
    app = undefined
    document.body.innerHTML = ''
  })

  it('先把列表画上（否则后面点的按钮根本不存在，红会落在别处）', async () => {
    await mountView()
    expect(screenText()).toContain('data')
    expect(screenText()).toContain('media')
  })

  it('预检读不到 ⇒ 屏幕出现「无法确认 / 不可恢复 / 共 1 项」，取消就一个字节都不动', async () => {
    const el = await mountView()
    state.failFiles = true
    await openMoveDialog(el)
    await clickMoveHere(el)

    // 先钉这一步的后果：读不到目标的那一刻，一个字节都不许已经写出去。
    // 旧行为（跳过检查交给后端）在这里红成 expected ['/media/data'] to deeply equal []——
    // 那就是「用户没被问一句就把同名文件盖掉了」的实物证据，比「没弹框」更直白。
    expect(state.moveCalls).toEqual([])

    const txt = screenText()
    expect(txt).toContain('无法确认目标是否有同名文件')
    expect(txt).toContain('不可恢复')
    expect(txt).toContain('共 1 项待处理')
    expect(state.moveCalls).toEqual([])

    await clickMessageBox('cancel')
    expect(state.moveCalls).toEqual([])
  })

  it('读不到目标但用户明确同意 ⇒ 才继续执行（那是闸门不是禁令）', async () => {
    const el = await mountView()
    state.failFiles = true
    await openMoveDialog(el)
    await clickMoveHere(el)
    await clickMessageBox('ok')
    expect(state.moveCalls.length).toBe(1)
    expect(state.moveCalls[0]).toContain('data')
  })

  it('正控：目标读得到且无同名 ⇒ 什么都不弹，直接执行', async () => {
    const el = await mountView()
    // 目标目录里只有一个别的名字：移动的是「data」，所以同名预检应当放行。
    state.overrideName = 'other.txt'
    await openMoveDialog(el)
    await clickMoveHere(el)
    expect(confirmBoxCount()).toBe(0)
    expect(state.moveCalls.length).toBe(1)
  })

  it('正控：目标读得到且真有同名 ⇒ 弹的是「目标存在同名 1 项」而不是「无法确认」', async () => {
    const el = await mountView()
    state.overrideName = 'data'
    await openMoveDialog(el)
    await clickMoveHere(el)
    const txt = screenText()
    expect(txt).toContain('目标存在同名 1 项')
    expect(txt).not.toContain('无法确认')
    await clickMessageBox('cancel')
    expect(state.moveCalls).toEqual([])
  })
})
