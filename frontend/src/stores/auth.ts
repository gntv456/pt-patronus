import { defineStore } from 'pinia'
import { authApi, type AuthStatus, type HxptUser, type HxptLoginParams } from '@/api/auth'

interface AuthState {
  token: string
  user: HxptUser | null
}

// user 随 token 一起持久化：刷新后先从本地恢复（避免顶栏闪 fallback），再由 router guard
// 调 restoreUser 用后端最新 status 覆盖。损坏的 JSON 回退 null。
function loadPersistedUser(): HxptUser | null {
  try {
    const raw = localStorage.getItem('user')
    return raw ? (JSON.parse(raw) as HxptUser) : null
  } catch {
    return null
  }
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    token: localStorage.getItem('token') || '',
    user: loadPersistedUser(),
  }),
  getters: {
    authenticated: (s): boolean => !!s.token,
    username: (s): string => s.user?.hxpt_username || s.user?.username || '',
    isAdmin: (s): boolean => !!s.user?.is_admin,
    vipVerified: (s): boolean => !!s.user?.vip_level,
  },
  actions: {
    // 好学账号登录（POST takelogin + VIP 校验）
    async loginByHxpt(p: HxptLoginParams) {
      const { token, user } = await authApi.hxptLogin(p)
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
    },
    // 好学 Cookie 登录（最稳路径）
    async loginByCookie(cookie: string) {
      const { token, user } = await authApi.hxptCookieLogin(cookie)
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
    },
    async loginDemo() {
      const { token, user } = await authApi.demoLogin()
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
    },
    // 登出：先清本地（UI 立即响应），再通知后端 bump TokenVersion
    async logout() {
      this.token = ''
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      try {
        await authApi.logout()
      } catch {
        /* token 已失效也无妨 */
      }
    },
    async status(): Promise<AuthStatus> {
      return authApi.status()
    },
    // 刷新后 user 随内存丢失（只 token 持久化）：token 仍有效时拉 status 恢复 user，
    // 顶栏用户名 / VIP 状态才不致回退。由 router guard 在 authenticated && !user 时调用。
    async restoreUser() {
      try {
        const s = await authApi.status()
        if (s.user) {
          this.user = s.user
          localStorage.setItem('user', JSON.stringify(s.user))
        }
      } catch {
        /* token 失效由 http 拦截清 token 跳登录 */
      }
    },
  },
})
