// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { describe, expect, it } from 'vitest'

import { devicePollTerminal, traktFailText, traktPollGiveUpText, TRAKT_POLL_GIVEUP } from './traktHelpers'

// #280：后端把 Trakt 的失败分成四档各写一句人话，面板要把它原样说出来，
// 并且知道哪一档值得停下来不再轮询。

const expired = {
  response: {
    status: 409,
    data: {
      code: 'trakt_code_expired',
      message: '这张设备授权码已经过期：请重新点「发起授权」拿新码（继续等不会生效）',
      authorized: false,
      terminal: true,
    },
  },
}

describe('traktFailText', () => {
  it('后端写了原因就用后端那句（不是本地猜的兜底文案）', () => {
    expect(traktFailText(expired, '发起授权失败')).toContain('重新点「发起授权」')
  })

  it('只带 error 兼容字段的旧信封也认', () => {
    const e = { response: { data: { error: 'Trakt 还没走完设备授权' } } }
    expect(traktFailText(e, '兜底')).toBe('Trakt 还没走完设备授权')
  })

  it('正控：什么都读不到时退回本地文案，不把 undefined 推上人眼', () => {
    expect(traktFailText(new Error('boom'), '同步失败')).toBe('同步失败')
    expect(traktFailText({ response: { data: { message: '   ' } } }, '同步失败')).toBe('同步失败')
    expect(traktFailText(undefined, '同步失败')).toBe('同步失败')
  })
})

describe('devicePollTerminal', () => {
  it('授权码过期 / 凭据没填 / 未授权：停下来', () => {
    expect(devicePollTerminal(expired)).toBe(true)
    for (const code of ['trakt_not_configured', 'trakt_not_authorized']) {
      expect(devicePollTerminal({ response: { data: { code, authorized: false } } })).toBe(true)
    }
  })

  it('上游没响应不是终端态：网络抖动不能被判成「授权坏了」', () => {
    const e = {
      response: {
        data: { code: 'trakt_upstream_failed', message: 'Trakt 没有完成这次请求', terminal: false },
      },
    }
    expect(devicePollTerminal(e)).toBe(false)
  })

  it('正控：连响应体都没有（超时/断网）继续等', () => {
    expect(devicePollTerminal(new Error('timeout'))).toBe(false)
    expect(devicePollTerminal(undefined)).toBe(false)
  })
})

describe('traktPollGiveUpText', () => {
  it('放弃轮询只能说「读不到状态」，不许说成授权失败', () => {
    const text = traktPollGiveUpText(TRAKT_POLL_GIVEUP)
    expect(text).toContain('读不到')
    expect(text).toContain(String(TRAKT_POLL_GIVEUP))
    expect(text).not.toMatch(/授权失败/)
    expect(text).not.toMatch(/未能授权/)
  })
})
