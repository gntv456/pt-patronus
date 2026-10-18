// @vitest-environment jsdom
// 审计面板的窗口说明必须真的画在面板里：composable 导出了状态不代表视图读了它。
import { describe, expect, it } from 'vitest'
import { createApp, h, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import PluginAuditDialog from './PluginAuditDialog.vue'

async function mountWith(props: Record<string, unknown>): Promise<string> {
  // el-dialog 会把内容 teleport 到 body：不清掉上一例的残留，第二条断言会读到上一次的文案。
  document.body.innerHTML = ''
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp({ render: () => h(PluginAuditDialog as never, props as never) })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  await new Promise((r) => setTimeout(r, 0))
  return document.body.textContent ?? ''
}

const base = {
  visible: true,
  // 面板主体挂在 v-if="target" 上：target 为空时整个 logs 区不渲染，说明也就无从出现。
  target: { id: 'p', name: 'P' },
  logs: [],
  releases: [],
  loading: false,
  rollbacking: null,
}

describe('PluginAuditDialog 常驻窗口说明', () => {
  it('notice 非空：说明画在面板里', async () => {
    const text = await mountWith({ ...base, notice: '插件日志共 640 条，当前仅显示最近 80 条' })
    expect(text).toContain('共 640 条')
    expect(text).toContain('最近 80 条')
  })

  it('正控：notice 为空时不多出一句谎', async () => {
    const text = await mountWith({ ...base, notice: '' })
    expect(text).not.toContain('共 640 条')
    expect(text).not.toContain('仅显示最近')
  })
})
