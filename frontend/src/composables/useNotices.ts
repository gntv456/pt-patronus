import { ref, computed, readonly } from 'vue'
import { useWs } from '@/composables/useWs'
import { noticeApi, type Notice, type NoticeSource } from '@/api/notice'

// 通知中心共享状态：模块级单例，避免 NotificationCenter / Notices / AppSidebar 各自重复请求。
// 维护完整通知列表 + 未读数，WS 实时推送自动合并。
const notices = ref<Notice[]>([])
const loaded = ref(false)

const urgentSources = new Set<NoticeSource>(['assess_fail', 'host_offline', 'nas_offline', 'site_offline'])

export function useNotices() {
  const { notices: wsNotices } = useWs()

  const unread = computed(() => notices.value.filter((n) => !n.read).length)
  const urgentUnread = computed(() =>
    notices.value.filter((n) => !n.read && urgentSources.has(n.source)).length,
  )

  async function load() {
    try {
      notices.value = await noticeApi.list()
      loaded.value = true
    } catch {
      /* 静默 */
    }
  }

  // WS 实时推送：新通知 prepend 到本地列表（去重 id）
  function syncFromWs() {
    const n = wsNotices.value[0]
    if (n && !notices.value.some((x) => x.id === n.id)) {
      notices.value.unshift(n)
    }
  }

  async function readOne(id: number) {
    try {
      await noticeApi.read(id)
      const t = notices.value.find((n) => n.id === id)
      if (t) t.read = true
    } catch {
      /* 静默 */
    }
  }
  async function readAll() {
    try {
      await noticeApi.readAll()
      notices.value.forEach((n) => (n.read = true))
    } catch {
      /* 静默 */
    }
  }
  async function removeOne(id: number) {
    try {
      await noticeApi.delete(id)
      notices.value = notices.value.filter((n) => n.id !== id)
    } catch {
      /* 静默 */
    }
  }
  async function clearAll() {
    try {
      await noticeApi.clear()
      notices.value = []
    } catch {
      /* 静默 */
    }
  }

  return {
    notices,
    loaded: readonly(loaded),
    unread,
    urgentUnread,
    load,
    syncFromWs,
    readOne,
    readAll,
    removeOne,
    clearAll,
  }
}