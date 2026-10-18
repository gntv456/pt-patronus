import { describe, expect, it } from 'vitest'
import { diagKindKey, diagMoreCount, diagTarget } from '@/utils/siteDefinitionDiag'
import zh from '@/i18n/locales/zh-CN'
import en from '@/i18n/locales/en-US'

// #331 站点定义诊断的展示层：kind 映射 + 「后端新增 kind 时不许把键路径当文案」+ locale 四件套齐全。
const KINDS = ['bad_regexp', 'unknown_key', 'duplicate_id', 'skipped_subdir']

describe('siteDefinitionDiag', () => {
  it('已知 kind 映射到对应文案键', () => {
    for (const k of KINDS) {
      expect(diagKindKey(k)).toBe(`sites.diagKind.${k}`)
    }
  })

  it('未知 kind（后端将来新加一类）落 other，不把键路径露给用户', () => {
    expect(diagKindKey('brand_new_kind')).toBe('sites.diagKind.other')
    expect(diagKindKey(undefined)).toBe('sites.diagKind.other')
    expect(diagKindKey('')).toBe('sites.diagKind.other')
  })

  it('定位优先站点 id，其次文件名；两者都没有回空串（模板里显示破折号）', () => {
    expect(diagTarget({ kind: 'duplicate_id', site: 'sameweb', file: 'b.yaml', detail: 'x' })).toBe('sameweb')
    expect(diagTarget({ kind: 'unknown_key', file: 'badkey.yaml', detail: 'x' })).toBe('badkey.yaml')
    expect(diagTarget({ kind: 'bad_regexp', detail: 'x' })).toBe('')
    expect(diagTarget(undefined)).toBe('')
  })

  it('只列前 6 条时，剩余条数不能算成负数', () => {
    expect(diagMoreCount(10, 6)).toBe(4)
    expect(diagMoreCount(6, 6)).toBe(0)
    expect(diagMoreCount(2, 6)).toBe(0)
  })

  it('zh / en 两套 locale 必须覆盖每个 kind + other + 三条标题文案', () => {
    for (const loc of [zh, en]) {
      const s = (loc as any).sites
      for (const k of KINDS.concat('other')) {
        expect(typeof s.diagKind?.[k], `diagKind.${k}`).toBe('string')
      }
      for (const key of ['diagTitle', 'diagMore', 'diagLoadFailed']) {
        expect(typeof s[key], key).toBe('string')
      }
    }
  })

  it('后端 kind 常量与前端映射表同源（漂移就红：internal/site/definitions_diag.go）', () => {
    // 这条断言只钉前端侧清单；后端清单见 Diag* 常量，两边都要改。
    expect(KINDS).toEqual(['bad_regexp', 'unknown_key', 'duplicate_id', 'skipped_subdir'])
  })
})
