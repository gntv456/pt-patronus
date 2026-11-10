// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// @vitest-environment jsdom
// 渲染层回归：文件管理器侧栏的云盘列表接口挂掉时，
// 原先静默 catch 后只剩本地存储，看起来像「没配网盘」。
// 现在 drivesFailed 会驱动侧栏显示「加载失败」，这里挂真组件验证接线。
import { describe, expect, it } from 'vitest'
import { createApp, h } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

import FileManagerSidebar from './FileManagerSidebar.vue'
import type { StorageItem } from '@/composables/useFileManager'

const fsItem: StorageItem = {
  kind: 'fs',
  fs: { id: 1, name: 'local', type: 'local', url: '', username: '', base_path: '/', agent_id: 0 },
}

function mountSidebar(
  list: StorageItem[],
  drivesFailed: boolean,
  fs: { failed?: boolean; forbidden?: boolean } = {},
): HTMLElement {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({
    render: () =>
      h(FileManagerSidebar, {
        list,
        loading: false,
        drivesFailed,
        fsFailed: fs.failed ?? false,
        fsForbidden: fs.forbidden ?? false,
        current: null,
        itemKey: (it: StorageItem) => it.kind + ':1',
        itemIcon: () => '💾',
        itemLabel: () => 'fs',
        statusOf: () => 'ok',
      }),
  })
  app.use(i18n)
  app.use(ElementPlus)
  // vitest 只挂了 AutoImport：手工注册指令，否则 resolve warn 刷屏会埋掉真告警。
  app.directive('a11y-button', {})
  app.mount(el)
  return el
}

describe('FileManagerSidebar：云盘列表读失败要说「加载失败」', () => {
  it('drivesFailed 时显示加载失败，而不是「还没有存储」', () => {
    const el = mountSidebar([], true)
    const txt = el.textContent ?? ''
    expect(txt).toContain('加载失败')
    expect(txt).not.toContain('还没有存储')
  })

  it('正控：没有失败位且列表为空时仍显示「还没有存储」', () => {
    const el = mountSidebar([], false)
    const txt = el.textContent ?? ''
    expect(txt).toContain('还没有存储')
    expect(txt).not.toContain('加载失败')
  })

  it('有本地存储且 drivesFailed 时提示加载失败但不挡列表', () => {
    const el = mountSidebar([fsItem], true)
    const txt = el.textContent ?? ''
    expect(txt).toContain('加载失败')
    expect(txt).toContain('local')
  })

  // /storages 整组是 admin-only：非管理员的「读不到」要说成「仅管理员可见」，
  // 既不能画成「还没有存储」（诱导他去添加），也不能只说「加载失败」。
  it('fsForbidden 时说「仅管理员可见」，不是「还没有存储」', () => {
    const el = mountSidebar([], false, { forbidden: true })
    const txt = el.textContent ?? ''
    expect(txt).toContain('存储后端仅管理员可见')
    expect(txt).not.toContain('还没有存储')
  })

  it('fsFailed（非 403）时说「加载失败」，也不是空态', () => {
    const el = mountSidebar([], false, { failed: true })
    const txt = el.textContent ?? ''
    expect(txt).toContain('加载失败')
    expect(txt).not.toContain('还没有存储')
  })
})
