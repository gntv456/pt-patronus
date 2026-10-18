// @vitest-environment jsdom
// #331 渲染层：站点定义诊断条必须真的把「N 条配置没生效」画出来，
// 且「读失败」与「真的没有诊断」不能共用一行文案（本仓最常见的一族）。
import { describe, expect, it } from 'vitest'
import { createApp, nextTick } from 'vue'
import { createI18n } from 'vue-i18n'
import ElementPlus from 'element-plus'
import zhCN from '@/i18n/locales/zh-CN'
import SiteDefinitionDiagnostics from '@/components/sites/SiteDefinitionDiagnostics.vue'
import type { DefinitionDiagnostic } from '@/api/site'

async function mount(props: { diags?: DefinitionDiagnostic[]; failed?: boolean; errText?: string }): Promise<HTMLElement> {
  const el = document.createElement('div')
  document.body.appendChild(el)
  const i18n = createI18n({ legacy: false, locale: 'zh-CN', messages: { 'zh-CN': zhCN } })
  const app = createApp(SiteDefinitionDiagnostics, {
    diags: props.diags ?? [],
    failed: props.failed ?? false,
    errText: props.errText ?? '',
  })
  app.use(i18n)
  app.use(ElementPlus)
  app.mount(el)
  await nextTick()
  return el
}

function visible(el: HTMLElement): string {
  return el.textContent?.replace(/\s+/g, ' ').trim() ?? ''
}

describe('站点定义诊断条', () => {
  it('没有诊断也没读失败 ⇒ 整条不出现（不许画成「0 条问题」那种假安心）', async () => {
    const el = await mount({})
    expect(el.querySelector('.el-alert')).toBeNull()
    expect(visible(el)).toBe('')
  })

  it('有诊断 ⇒ 标题带条数，逐条带类别中文 + 定位 + 原因', async () => {
    const el = await mount({
      diags: [
        { kind: 'unknown_key', site: '', file: 'myself.yaml', detail: '有不认识的键' },
        { kind: 'duplicate_id', site: 'sameweb', file: 'b.yaml', detail: '与 a.yaml 撞车' },
      ],
    })
    const text = visible(el)
    expect(text).toContain('站点定义里有 2 条配置没有生效')
    expect(text).toContain('有不认识的配置键')
    expect(text).toContain('myself.yaml')
    expect(text).toContain('站点 id 撞车')
    expect(text).toContain('sameweb')
    // 不许把 i18n 键路径当文案显示
    expect(text).not.toContain('sites.diagKind.')
  })

  it('后端新增一类 kind ⇒ 落「其它配置问题」，不露裸键路径', async () => {
    const el = await mount({ diags: [{ kind: 'brand_new', site: 's9', detail: '新情况' }] })
    const text = visible(el)
    expect(text).toContain('其它配置问题')
    expect(text).not.toContain('sites.diagKind')
  })

  it('超过 6 条只列 6 条，并说清「另有 N 条」', async () => {
    const many: DefinitionDiagnostic[] = []
    for (let i = 0; i < 8; i++) many.push({ kind: 'bad_regexp', site: `s${i}`, detail: `第 ${i} 条` })
    const el = await mount({ diags: many })
    expect(el.querySelectorAll('.def-diag-list li')).toHaveLength(6)
    expect(visible(el)).toContain('另有 2 条未列出')
  })

  it('读失败 ⇒ 显示读取失败与原因，而不是「一切正常」', async () => {
    const el = await mount({ failed: true, errText: 'Network Error' })
    expect(el.querySelector('.el-alert')).not.toBeNull()
    const text = visible(el)
    expect(text).toContain('站点定义诊断读取失败')
    expect(text).toContain('Network Error')
    expect(text).not.toContain('没有生效')
  })
})
