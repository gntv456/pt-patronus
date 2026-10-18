// @vitest-environment jsdom
// 渲染层回归：整理配置抽屉的下载器/云盘下拉数据源读失败时，
// 原先直接渲染空下拉，看起来像「没配置」。
// 现在 loadFailed 驱动抽屉顶部显示「加载失败：下载器、目标网盘」，这里挂真组件验证接线。
import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'

import TransferConfigDialog from './TransferConfigDialog.vue'
import type { TransferConfig } from '@/api/transfer'

const baseCfg = {
  path_mappings: [],
  failed_retry_minutes: 360,
  cloud_upload: { enabled: true, drive_id: 0, cloud_base: '', upload_videos: true, strm_output_dir: '', strm_template: '', delete_local: false },
} as unknown as TransferConfig

async function mountDialog(loadFailed: string[]): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({
    render: () =>
      h(TransferConfigDialog, {
        showConfig: true,
        'onUpdate:showConfig': () => {},
        cfg: baseCfg,
        catsStr: '',
        skipStr: '',
        cloudDrives: [],
        downloaders: [],
        loadFailed,
        savingCfg: false,
        renameVars: [],
        previewRename: (tpl: string) => tpl,
      }),
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await nextTick()
  return el
}

describe('TransferConfigDialog：下拉数据源读失败要说「加载失败」', () => {
  it('downloaders/cloud 失败时点名来源，而不是只给空下拉', async () => {
    const el = await mountDialog(['downloader', 'cloud'])
    const txt = (el.textContent ?? '').replace(/\s+/g, ' ')
    expect(txt).toContain('加载失败')
    expect(txt).toContain('下载器')
    expect(txt).toContain('目标网盘')
  })

  it('正控：无失败点名单时不显示加载失败', async () => {
    const el = await mountDialog([])
    expect(el.textContent ?? '').not.toContain('加载失败')
  })
})
