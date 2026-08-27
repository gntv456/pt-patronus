import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { authApi, type AuthStatus, type CaptchaChallenge } from '@/api/auth'

// 登录页表单与各登录方式（账号/Cookie/演示/OIDC）逻辑。
// 抽出为 composable 收敛 Login.vue 脚本体积；ElMessage 依赖 unplugin-auto-import（如有）。
export function useLoginAuth() {
  const auth = useAuthStore()

  const loading = ref(false)
  const err = ref('')
  const captcha = ref<CaptchaChallenge | null>(null)
  // 账号登录
  const acc = ref({ username: '', password: '', twoStep: '', imagestring: '' })
  // Cookie 登录
  const cookieStr = ref('')

  async function fetchCaptcha() {
    try {
      captcha.value = await authApi.hxptCaptcha()
      // OCR 服务端自动识别成功 → 预填验证码（用户可改；识别失败保持空人工输入）。
      if (captcha.value?.need_captcha && captcha.value.ocr_text && !acc.value.imagestring) {
        acc.value.imagestring = captcha.value.ocr_text
      }
    } catch (e: unknown) {
      err.value = msg(e) || '取验证码失败（可改用 Cookie 登录）'
    }
  }

  async function accountLogin(onSuccess: () => void) {
    if (!acc.value.username || !acc.value.password) {
      err.value = '请填写好学账号和密码'
      return
    }
    if (acc.value.twoStep && !/^\d{6}$/.test(acc.value.twoStep.trim())) {
      err.value = '二级验证码应为 6 位数字'
      return
    }
    loading.value = true
    err.value = ''
    try {
      await auth.loginByHxpt({
        username: acc.value.username,
        password: acc.value.password,
        two_step_code: acc.value.twoStep || undefined,
        imagehash: captcha.value?.need_captcha ? captcha.value.imagehash : undefined,
        imagestring: captcha.value?.need_captcha ? acc.value.imagestring : undefined,
        extra_form: captcha.value?.extra_form,
        challenge_mode: captcha.value?.challenge_mode || false,
      })
      onSuccess()
    } catch (e: unknown) {
      err.value = msg(e) || '登录失败'
      // 凭据/验证码错（401）→ 刷新验证码
      if ((e as { response?: { status?: number } })?.response?.status === 401) {
        acc.value.imagestring = ''
        await fetchCaptcha()
      }
    } finally {
      loading.value = false
    }
  }

  async function cookieLogin(onSuccess: () => void) {
    const c = cookieStr.value.trim()
    if (!c) {
      err.value = '请粘贴好学 cookie'
      return
    }
    if (!c.includes('=')) {
      err.value = 'cookie 格式有误：应含「键=值」字段，请从浏览器完整复制整段 cookie'
      return
    }
    loading.value = true
    err.value = ''
    try {
      await auth.loginByCookie(c)
      onSuccess()
    } catch (e: unknown) {
      err.value = msg(e) || '登录失败'
    } finally {
      loading.value = false
    }
  }

  async function demoLogin(onSuccess: () => void) {
    loading.value = true
    err.value = ''
    try {
      await auth.loginDemo()
      onSuccess()
    } catch (e: unknown) {
      err.value = msg(e) || '演示登录未启用'
    } finally {
      loading.value = false
    }
  }

  // OIDC/SSO：整页跳转到后端 /auth/oidc/start，后端 302 到 Provider，完成后回带 oidc_token。
  function oidcLogin() {
    window.location.href = '/api/v1/auth/oidc/start'
  }

  function msg(e: unknown): string {
    const r = (e as { response?: { data?: { message?: string; error?: string } } })?.response?.data
    return r?.message || r?.error || ''
  }

  return { auth, loading, err, captcha, acc, cookieStr, fetchCaptcha, accountLogin, cookieLogin, demoLogin, oidcLogin }
}
