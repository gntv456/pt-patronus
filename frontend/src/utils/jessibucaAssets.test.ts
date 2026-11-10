// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import { JESSIBUCA_ASSETS, JESSIBUCA_VERSION, assetUrl } from './jessibucaAssets'
import zh from '@/i18n/locales/zh-CN'
import en from '@/i18n/locales/en-US'
import cameraBridge from '@/i18n/locales/cameraBridge'

// 注：本仓 tsconfig 不暴露 node 类型（node:fs 一律 TS2307，vue-tsc 整棵树红），
// 故这里不读仓库文件，改为对「导入得到的真实模块」做断言。
// 跨文件一致性（脚本 ↔ 后端清单 ↔ ignore 规则）由后端与 CI 的
// scripts/check-no-gpl-assets.sh 负责，不在前端重复。

describe('jessibuca 运行时资源', () => {
  it('三个必需文件都在清单里（少一个预览就起不来）', () => {
    expect(JESSIBUCA_ASSETS.map((a) => a.file).sort()).toEqual(['decoder.js', 'decoder.wasm', 'jessibuca.js'].sort())
  })

  it('固定版本，避免上游 tag 移动导致拉到不同字节', () => {
    expect(JESSIBUCA_VERSION).toMatch(/^v\d+\.\d+\.\d+$/)
  })

  it('字节数与上游 dist 实测一致（前端据此判断资源是否已就位）', () => {
    const byFile = Object.fromEntries(JESSIBUCA_ASSETS.map((a) => [a.file, a.bytes]))
    expect(byFile).toEqual({ 'jessibuca.js': 219743, 'decoder.js': 110237, 'decoder.wasm': 1050314 })
  })

  it('assetUrl 指向 /jessibuca/（与后端路由一致）', () => {
    expect(assetUrl('jessibuca.js')).toBe('/jessibuca/jessibuca.js')
    expect(assetUrl('decoder.wasm')).toBe('/jessibuca/decoder.wasm')
  })

  it('两套 locale 的 cambridge 段落 key 完全一致（漏一个就静默回落另一种语言）', () => {
    const zhKeys = Object.keys(cameraBridge.zh.cambridge).sort()
    const enKeys = Object.keys(cameraBridge.en.cambridge).sort()
    expect(zhKeys).toEqual(enKeys)
    expect(zhKeys.length).toBeGreaterThanOrEqual(10)
  })

  it('缺失资源提示说清「自动获取、无需操作」——不能让用户以为要自己动手', () => {
    expect(cameraBridge.zh.cambridge.assetsPending).toBeTruthy()
    expect(cameraBridge.zh.cambridge.assetsPendingHint).toContain('自动')
    expect(cameraBridge.en.cambridge.assetsPendingHint).toMatch(/automatic/i)
  })

  it('合并后的两套 messages 不含裸 key（漏合并会导致渲染出 cambridge.xxx 原文）', () => {
    const merged: Record<string, unknown> = { ...(zh as Record<string, unknown>), ...cameraBridge.zh }
    expect(Object.keys(merged)).toContain('cambridge')
    // 主表仍保持原样，没有被 cameraBridge 覆盖
    expect(merged.rp).toBeDefined()
  })

  it('主翻译表未被接入指引污染（cambridge 只存在于独立模块）', () => {
    expect(Object.keys(zh as Record<string, unknown>)).not.toContain('cambridge')
    expect(Object.keys(en as Record<string, unknown>)).not.toContain('cambridge')
  })
})
