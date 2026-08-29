import { defineStore } from 'pinia'
import { authApi, type AuthStatus, type HxptUser, type HxptLoginParams } from '@/api/auth'

interface AuthState {
  token: string
  user: HxptUser | null
  // cookie 会话标记（收权加固）：后端把 JWT 种在 httpOnly cookie 里（JS 读不到），
  // 这里只存一个布尔标记（sessionStorage，标签页生命周期），供 authenticated 判定。
  // 标记的真实性由后端保证：cookie 失效时任意 API 401 → http 拦截器清标记跳登录。
  cookieSession: boolean
}

// user 随 token 一起持久化：刷新后先从本地恢复（避免顶栏闪 fallback），再由 router guard
// 调 restoreUser 用后端最新 status 覆盖。损坏的 JSON 回退 null。
function loadPersistedUser(): HxptUser | null {
  try {
    const raw = localStorage.getItem('user')
    return raw ? (JSON.parse(raw) as HxptUser) || null : null
  } catch {
    return null
  }
}

// cookie 会话标记：sessionStorage（每标签页独立，关标签即清）。
// 旧会话迁移：localStorage 有 Bearer 时先视为已认证，首个 API 调用会刷新 cookie。
function loadCookieSession(): boolean {
  return sessionStorage.getItem('ptp_cookie_session') === '1' || !!localStorage.getItem('token')
}

export const useAuthStore = defineStore('auth', {
  state: (): AuthState => ({
    token: localStorage.getItem('token') || '',
    user: loadPersistedUser(),
    cookieSession: loadCookieSession(),
  }),
  getters: {
    authenticated: (s): boolean => !!s.token || s.cookieSession,
    username: (s): string => s.user?.hxpt_username || s.user?.username || '',
    isAdmin: (s): boolean => !!s.user?.is_admin,
    vipVerified: (s): boolean => !!s.user?.vip_level,
  },
  actions: {
    // 登录成功统一落位（好学账号/Cookie/演示共用）：httpOnly cookie 由 Set-Cookie 头
    // 自动种下；Bearer 同时保留（扩展打开的 Web 页等非 cookie 场景兜底）。
    onLoginSuccess(token: string, user: HxptUser | null) {
      this.token = token
      this.user = user
      this.cookieSession = true
      sessionStorage.setItem('ptp_cookie_session', '1')
      localStorage.setItem('token', token)
      if (user) localStorage.setItem('user', JSON.stringify(user))
    },
    // 好学账号登录（POST takelogin + VIP 校验）
    async loginByHxpt(p: HxptLoginParams) {
      const { token, user } = await authApi.hxptLogin(p)
      this.onLoginSuccess(token, user)
    },
    // 好学 Cookie 登录（最稳路径）
    async loginByCookie(cookie: string) {
      const { token, user } = await authApi.hxptCookieLogin(cookie)
      this.onLoginSuccess(token, user)
    },
    async loginDemo() {
      const { token, user } = await authApi.demoLogin()
      this.onLoginSuccess(token, user)
    },
    // OIDC/SSO 回调：只拿 token（无 user），存 token 后由 router guard 的 restoreUser 拉回 user。
    async loginByToken(token: string) {
      this.onLoginSuccess(token, null)
      localStorage.removeItem('user')
    },
    // 会话失效（http 拦截器 401/VIP 失败调用）：清双通道。
    onSessionExpired() {
      this.token = ''
      this.user = null
      this.cookieSession = false
      sessionStorage.removeItem('ptp_cookie_session')
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },
    // 登出：先清本地（UI 立即响应），再通知后端 bump TokenVersion + 清 cookie
    async logout() {
      this.onSessionExpired()
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
    // 同时回填 cookie 会话标记（Bearer 过期但 cookie 仍有效的旧会话在此续上）。
    async restoreUser() {
      try {
        const s = await authApi.status()
        this.cookieSession = true
        sessionStorage.setItem('ptp_cookie_session', '1')
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
