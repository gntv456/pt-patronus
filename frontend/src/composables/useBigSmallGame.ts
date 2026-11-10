// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, type Ref } from 'vue'
import {
  entertainmentApi,
  type BigSmallState,
  type BigSmallChat,
  type BigSmallTask,
  type BigSmallProp,
  type BigSmallRedpacket,
} from '@/api/entertainment'

export function useBigSmallChat(
  siteId: Ref<number>,
  game: Ref<string>,
  load: (silent?: boolean) => Promise<void>,
  state: Ref<BigSmallState | null>,
) {
  const chat = ref<BigSmallChat[]>([])
  const chatInput = ref('')
  const lastChatId = ref(0)
  const sending = ref(false)

  function mergeChat(inc: BigSmallChat[]) {
    const map = new Map(chat.value.map((m) => [m.id, m]))
    for (const m of inc) map.set(m.id, m)
    chat.value = [...map.values()].sort((a, b) => a.id - b.id).slice(-60)
    if (inc.length) lastChatId.value = chat.value.reduce((m, c) => Math.max(m, c.id), lastChatId.value)
  }

  async function refreshChat(silent = true) {
    try {
      const list = await entertainmentApi.bigsmallChat(siteId.value, game.value, lastChatId.value)
      if (list.length) mergeChat(list)
    } catch {
      /* 静默 */
    }
  }

  async function sendChat() {
    const msg = chatInput.value.trim()
    if (!msg || sending.value) return
    sending.value = true
    try {
      const res = await entertainmentApi.bigsmallSendChat(siteId.value, game.value, msg)
      if (res.success) {
        chatInput.value = ''
        await refreshChat()
      } else ElMessage.warning(res.message || '发送失败')
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '发送失败')
    } finally {
      sending.value = false
    }
  }

  // 由 load 调用：首屏填充聊天
  function initChatFromState(s: BigSmallState) {
    if (chat.value.length === 0 && s.chat?.length) {
      chat.value = s.chat
      lastChatId.value = s.chat.reduce((m, c) => Math.max(m, c.id), 0)
    }
  }

  return { chat, chatInput, sending, refreshChat, sendChat, initChatFromState }
}

export function useBigSmallItems(siteId: Ref<number>, game: Ref<string>, load: (silent?: boolean) => Promise<void>) {
  async function doTask(t: BigSmallTask, action: 'claim' | 'reward') {
    try {
      const res = await entertainmentApi.bigsmallTask(siteId.value, game.value, action, t.id)
      ElMessage[res.success ? 'success' : 'warning'](res.message || (action === 'reward' ? '已领取' : '已接取'))
      if (res.success) await load(true)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '操作失败')
    }
  }
  async function useRefund() {
    try {
      const res = await entertainmentApi.bigsmallTask(siteId.value, game.value, 'refund', 0)
      ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
      if (res.success) await load(true)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '操作失败')
    }
  }
  async function useProp(p: BigSmallProp) {
    try {
      const res = await entertainmentApi.bigsmallItem(siteId.value, game.value, p.tag)
      ElMessage[res.success ? 'success' : 'warning'](res.message || '已使用')
      if (res.success) await load(true)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '使用失败')
    }
  }
  async function claimRed(r: BigSmallRedpacket) {
    try {
      const res = await entertainmentApi.bigsmallRedpacket(siteId.value, game.value, 'claim', { id: r.id })
      ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
      if (res.success) await load(true)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '操作失败')
    }
  }

  // 红包发送表单
  const rpAmount = ref(1000)
  const rpCount = ref(5)
  const rpGreeting = ref('恭喜发财')
  const rpSending = ref(false)
  async function sendRed() {
    if (rpSending.value) return
    rpSending.value = true
    try {
      const res = await entertainmentApi.bigsmallRedpacket(siteId.value, game.value, 'send', {
        amount: String(rpAmount.value),
        count: String(rpCount.value),
        greeting: rpGreeting.value,
      })
      ElMessage[res.success ? 'success' : 'warning'](res.message || '操作结果')
      if (res.success) await load(true)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '发送失败')
    } finally {
      rpSending.value = false
    }
  }

  return {
    doTask,
    useRefund,
    useProp,
    claimRed,
    rpAmount,
    rpCount,
    rpGreeting,
    rpSending,
    sendRed,
  }
}

export function fmt(n: number | undefined) {
  return n ? n.toLocaleString() : '0'
}
