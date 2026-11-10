// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'
import { subtitleFixOutcome } from './subtitleFixFeedback'

describe('字幕补齐受理回执的口径', () => {
  it('排进队列 >0：报数量并指向通知', () => {
    expect(subtitleFixOutcome(7, 9)).toEqual({
      kind: 'success',
      key: 'mediaserver.subtitleFixAccepted',
      params: { n: 7 },
    })
  })

  it('一部都没排进队列：警告，且说清是「没有可用路径」而不是「已受理」', () => {
    const o = subtitleFixOutcome(0, 9)
    expect(o.kind).toBe('warning')
    expect(o.key).toBe('mediaserver.subtitleFixNone')
    expect(o.params).toEqual({ n: 9 })
    expect(o.key).not.toBe('mediaserver.subtitleFixAccepted')
  })
})
