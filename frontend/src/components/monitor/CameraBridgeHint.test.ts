// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import zhCN from '@/i18n/locales/zh-CN'
import enUS from '@/i18n/locales/en-US'
import cameraBridge from '@/i18n/locales/cameraBridge'

// 摄像头接入指引（cameraBridge 独立模块）的文案覆盖。
//
// 注：本仓 tsconfig 不暴露 node 类型（node:fs 一律 TS2307，vue-tsc 整棵树红），
// 故不读 SFC 源码做静态断言，改为校验「合并后真正上屏的两套文案」。
// 组件模板是否引用了这些 key，由 lint / 快照外的行为验证兜住。
type Node = string | { [k: string]: Node }

function flatten(obj: Node, prefix = ''): string[] {
  const out: string[] = []
  for (const [k, v] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.push(path)
    else out.push(...flatten(v, path))
  }
  return out
}

// 用 Record 中转：as Node 可能被 TS 判成 string（联合类型），拒绝展开。
const zh = { ...(zhCN as unknown as Record<string, Node>), ...(cameraBridge.zh as unknown as Record<string, Node>) }
const en = { ...(enUS as unknown as Record<string, Node>), ...(cameraBridge.en as unknown as Record<string, Node>) }
const zhKeys = new Set(flatten(zh))
const enKeys = new Set(flatten(en))

describe('CameraBridgeHint 摄像头接入指引', () => {
  it('指引段落已合入（否则组件里 t() 会渲染出 key 原文）', () => {
    expect([...zhKeys].filter((k) => k.startsWith('cambridge.')).length).toBeGreaterThanOrEqual(10)
  })

  it('中文把小米接入路径说清楚：Miloco + go2rtc + 端口 + H.265 + 替代方案', () => {
    const joined = [...zhKeys]
      .filter((k) => k.startsWith('cambridge.'))
      .map((k) => flatten(zh).find(() => false) ?? k)
      .join('\n')
    // 直接取文案而非 key
    const text = JSON.stringify(cameraBridge.zh)
    expect(text).toContain('Miloco')
    expect(text).toContain('go2rtc')
    expect(text).toContain('8554')
    expect(text).toContain('H.265')
    expect(text).toContain('yi-hack')
    expect(joined.length).toBeGreaterThan(0)
  })

  it('英文覆盖同一批主题，且未混入中文正文', () => {
    const text = JSON.stringify(cameraBridge.en)
    expect(text).toContain('Miloco')
    expect(text).toContain('go2rtc')
    expect(text).toContain('H.265')
    expect(text).toContain('yi-hack')
    expect(/[一-龥]/.test(text.replace(/RTSP 地址/g, ''))).toBe(false)
  })

  it('两套文案无 vue-i18n 未替换插值标记', () => {
    // 直接遍历叶子值，不 stringify：中文里的 」} 」等字符序列与 JSON 转义都可能
    // 产生误报。vue-i18n 真没接上时留下的是双花括号 {{key}}，单花括号 {n} 合法。
    const leaves = (o: Node): string[] => {
      const out: string[] = []
      for (const v of Object.values(o)) {
        if (typeof v === 'string') out.push(v)
        else out.push(...leaves(v))
      }
      return out
    }
    const all = [...leaves(cameraBridge.zh as unknown as Node), ...leaves(cameraBridge.en as unknown as Node)]
    expect(all.filter((s) => s.includes('{{') || s.includes('}}'))).toEqual([])
  })

  it('ONVIF 与 RTSP 两类指引各自完整（都说了系统会自动处理什么）', () => {
    expect(cameraBridge.zh.cambridge.onvifTitle).toBeTruthy()
    expect(cameraBridge.zh.cambridge.rtspTitle).toBeTruthy()
    expect(cameraBridge.zh.cambridge.onvifBody).toMatch(/自动|回填/)
  })
})
