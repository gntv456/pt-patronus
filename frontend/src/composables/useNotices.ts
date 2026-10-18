import { ref, computed, readonly } from 'vue'
import { useWs } from '@/composables/useWs'
import { noticeApi, type Notice, type NoticeSource } from '@/api/notice'
// ElMessage 依赖 unplugin-auto-import（与 useAgentChat 等一致）。

// 通知中心共享状态：模块级单例，避免 NotificationCenter / Notices / AppSidebar 各自重复请求。
// 维护完整通知列表 + 未读数，WS 实时推送自动合并。
const notices = ref<Notice[]>([])
const loaded = ref(false)
// 后端一次只回最近 limit 条（默认 200、封顶 1000）。未读超过一页时，
// 只从这一页里数未读会让角标少报——用户以为「只剩这几条」，实际还有更多。
const serverUnread = ref<number | null>(null)
// 收件箱读取失败的真值：静默吞掉时列表为空，铃铛下拉就显示「暂无通知」——
// 用户以为没有消息，其实是没读到。空列表 + 这个标记才能把两种情况分开。
const loadFailed = ref(false)
const inboxWindow = ref<{ shown: number; total?: number; truncated: boolean }>({
  shown: 0,
  total: undefined,
  truncated: false,
})

const urgentSources = new Set<NoticeSource>(['assess_fail', 'host_offline', 'nas_offline', 'camera_offline', 'site_offline'])

// 后端信封里的中文原因优先（#204/#205 之后「标/删一条已经不存在的通知」是 404 + 一句
// 「通知不存在，可能已被删除，请刷新通知列表」），泛化文案只作兜底。
// 收件箱页与铃铛抽屉共用这一个判据，别在第二处重抄。
export function noticeFailReason(e: unknown, fallback: string): string {
  const why = (e as Error)?.message
  return why || fallback
}

function dropUnread(n?: Notice) {
  if (n && !n.read && serverUnread.value != null) serverUnread.value = Math.max(0, serverUnread.value - 1)
}

export function useNotices() {
  const { notices: wsNotices } = useWs()

  const unread = computed(() => {
    const local = notices.value.filter((n) => !n.read).length
    return serverUnread.value == null ? local : Math.max(local, serverUnread.value)
  })
  const urgentUnread = computed(() => notices.value.filter((n) => !n.read && urgentSources.has(n.source)).length)

  async function load() {
    try {
      const page = await noticeApi.listPage()
      notices.value = page.notices ?? []
      serverUnread.value = page.unread_total ?? null
      inboxWindow.value = {
        shown: notices.value.length,
        total: page.total,
        truncated: page.truncated,
      }
      loaded.value = true
      loadFailed.value = false
    } catch {
      loadFailed.value = true
    }
  }

  // WS 实时推送：新通知 prepend 到本地列表（去重 id）
  function syncFromWs() {
    const n = wsNotices.value[0]
    if (n && !notices.value.some((x) => x.id === n.id)) {
      notices.value.unshift(n)
      if (!n.read && serverUnread.value != null) serverUnread.value += 1
    }
  }

  // 返回「这条到底标成功没有」：从前调用方只看 Promise 会不会 reject，
  // 而这里把失败 catch 掉只弹提示 ⇒ 组级批量操作永远以为全部成功（#294）。
  // quiet=true 时不逐条弹（批量操作自己出一条聚合提示）。
  async function readOne(id: number, quiet = false): Promise<boolean> {
    try {
      await noticeApi.read(id)
      const t = notices.value.find((n) => n.id === id)
      dropUnread(t)
      if (t) t.read = true
      return true
    } catch (e) {
      if (!quiet) ElMessage.warning(noticeFailReason(e, '标记已读失败'))
      return false
    }
  }
  async function readAll() {
    try {
      await noticeApi.readAll()
      notices.value.forEach((n) => (n.read = true))
      serverUnread.value = 0
    } catch (e) {
      ElMessage.warning(noticeFailReason(e, '全部已读失败'))
    }
  }
  async function removeOne(id: number) {
    try {
      await noticeApi.delete(id)
      dropUnread(notices.value.find((n) => n.id === id))
      notices.value = notices.value.filter((n) => n.id !== id)
    } catch (e) {
      ElMessage.error(noticeFailReason(e, '删除通知失败'))
    }
  }
  async function clearAll() {
    try {
      await noticeApi.clear()
      notices.value = []
      serverUnread.value = 0
      inboxWindow.value = { shown: 0, total: 0, truncated: false }
    } catch (e) {
      ElMessage.error(noticeFailReason(e, '清空通知失败'))
    }
  }

  return {
    notices,
    loaded: readonly(loaded),
    unread,
    urgentUnread,
    inboxWindow: readonly(inboxWindow),
    loadFailed: readonly(loadFailed),
    load,
    syncFromWs,
    readOne,
    readAll,
    removeOne,
    clearAll,
  }
}
