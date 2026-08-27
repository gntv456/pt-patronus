import { ref, watch, type Ref } from 'vue'
import { authApi } from '@/api/auth'
import { useAuthStore } from '@/stores/auth'

// 用户偏好跨浏览器同步（对标 MSaber「偏好随账号同步」）：
// localStorage 即时生效（离线可用），登录后与服务端 /auth/preferences/{key} 双向同步——
// 本地新于服务端则推，服务端新于本地则拉（简单策略：登录时拉一次覆盖本地缺失域，
// 之后每次变更 debounce 推送；单账号多浏览器以后写为准）。

interface SyncState { syncedAt: number }

const prefCache: Record<string, Ref<Record<string, any> | null>> = {}
const syncState: Record<string, SyncState> = {}
let pushTimers: Record<string, ReturnType<typeof setTimeout> | undefined> = {}

function readLocal(key: string): Record<string, any> | null {
  try {
    const raw = localStorage.getItem(`pref.${key}`)
    return raw ? JSON.parse(raw) : null
  } catch {
    return null
  }
}

function writeLocal(key: string, value: Record<string, any>) {
  try {
    localStorage.setItem(`pref.${key}`, JSON.stringify(value))
  } catch { /* 配额满静默 */ }
}

export function useUserPrefs(key: string) {
  if (!prefCache[key]) {
    prefCache[key] = ref(readLocal(key))
    syncState[key] = { syncedAt: 0 }
  }
  const prefs = prefCache[key]

  async function loadFromServer() {
    const auth = useAuthStore()
    if (!auth.token) return
    try {
      const res = await authApi.getPreferences(key)
      const remote = JSON.parse(res.value || '{}')
      // 合并策略：远端字段优先（服务端为真源），本地新增字段保留
      const merged = { ...(prefs.value ?? {}), ...remote }
      prefs.value = merged
      writeLocal(key, merged)
      syncState[key].syncedAt = Date.now()
    } catch { /* 未登录/网络失败：沿用本地 */ }
  }

  function pushToServer() {
    const auth = useAuthStore()
    if (!auth.token || !prefs.value) return
    authApi.setPreferences(key, prefs.value).catch(() => { /* 静默：下次变更重推 */ })
    syncState[key].syncedAt = Date.now()
  }

  function update(patch: Record<string, any>) {
    prefs.value = { ...(prefs.value ?? {}), ...patch }
    writeLocal(key, prefs.value)
    if (pushTimers[key]) clearTimeout(pushTimers[key])
    pushTimers[key] = setTimeout(pushToServer, 1500) // debounce：连点主题色不逐次打库
  }

  // 首次使用该域时拉取服务端（登录态下）
  if (syncState[key].syncedAt === 0) {
    loadFromServer()
  }

  // 偏好整体引用（null=尚未初始化；调用方应传默认值兜底）
  const safePrefs: Ref<Record<string, any>> = prefs as Ref<Record<string, any>>

  return { prefs: safePrefs, update, loadFromServer, pushToServer }
}
