// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, it, expect } from 'vitest'
import { buildDiagnosticsLines } from './useGeneralSettings'

// ADR-003：匿名遥测默认拒绝，改为「复制脱敏诊断信息」。
// 这里钉死脱敏契约：输出只含结构性计数与版本/平台，绝不出现 Cookie / URL / 用户名。
describe('buildDiagnosticsLines (redacted)', () => {
  it('只包含约定的脱敏字段', () => {
    const lines = buildDiagnosticsLines('1.03.31', { version: '1.03.31' }, { site_count: 5, downloader_count: 2 })
    const text = lines.join('\n')
    expect(text).toContain('version: 1.03.31')
    expect(text).toContain('platform: web')
    expect(text).toContain('site_count: 5')
    expect(text).toContain('downloader_count: 2')
    expect(lines.length).toBe(6)
  })

  it('不泄漏 URL / 用户名 / Cookie / 站点名', () => {
    const lines = buildDiagnosticsLines(
      '',
      { version: '1.03.31' },
      {
        site_count: 1,
        downloader_count: 1,
        // 即便上游对象混入敏感字段，也不得进入输出
        ...({ base_url: 'https://secret.example', username: 'alice', cookie: 'sid=abc123', site_name: '私密站' } as Record<string, unknown>),
      },
    )
    const text = lines.join('\n').toLowerCase()
    for (const leak of ['secret.example', 'alice', 'sid=abc123', '私密站', 'https://', 'cookie:']) {
      expect(text, '不得包含: ' + leak).not.toContain(leak)
    }
  })

  it('上游返回 null 时用 n/a 兜底，不抛错', () => {
    const text = buildDiagnosticsLines('', null, null).join('\n')
    expect(text).toContain('site_count: n/a')
    expect(text).toContain('downloader_count: n/a')
  })
})
