// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { beforeEach, describe, expect, it, vi } from 'vitest'
import { streamAgentChat, type AgentEvent } from './agent'

function streamResponse(chunks: string[], status = 200) {
  const encoder = new TextEncoder()
  return new Response(
    new ReadableStream({
      start(controller) {
        for (const chunk of chunks) controller.enqueue(encoder.encode(chunk))
        controller.close()
      },
    }),
    { status },
  )
}

beforeEach(() => {
  localStorage.clear()
  vi.restoreAllMocks()
})

describe('streamAgentChat', () => {
  it('parses fragmented LF and CRLF SSE frames', async () => {
    localStorage.setItem('token', 'jwt')
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        streamResponse([
          'event: conversation\r\ndata: {"id":7}\r\n\r',
          '\nevent: token\ndata: {"content":"he',
          'llo"}\n\nevent: done\r\ndata: {}\r\n\r\n',
        ]),
      )
    vi.stubGlobal('fetch', fetchMock)
    const events: AgentEvent[] = []

    const conversationId = await streamAgentChat('hi', null, (event) => events.push(event))

    expect(conversationId).toBe(7)
    expect(events.map((event) => event.event)).toEqual(['conversation', 'token', 'done'])
    expect(events[1]).toEqual({ event: 'token', data: { content: 'hello' } })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/agent/chat',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({ Authorization: 'Bearer jwt' }),
      }),
    )
  })

  it('rejects non-success HTTP responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('denied', { status: 403 })))
    await expect(streamAgentChat('hi', null, () => undefined)).rejects.toThrow('HTTP 403')
  })
})
