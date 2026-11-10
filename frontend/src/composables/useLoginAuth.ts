// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'

// 登录页表单与各登录方式（本地密码/演示/OIDC）逻辑。
// 抽出为 composable 收敛 Login.vue 脚本体积；ElMessage 依赖 unplugin-auto-import（如有）。
// 旧好学站点账号/Cookie 登录已随站点改版（FluxTorrent，无验证码/verifyAccess 链路）下线删除。
export function useLoginAuth() {
  const auth = useAuthStore()
  const { t } = useI18n()

  const loading = ref(false)
  const err = ref('')
  // 本地密码登录
  const acc = ref({ username: '', password: '' })

  // 本地密码登录（好学站点关闭后的主路径，#<issue>）。
  async function localLogin(onSuccess: () => void) {
    if (!acc.value.username || !acc.value.password) {
      err.value = t('login.err.fillLocal')
      return
    }
    loading.value = true
    err.value = ''
    try {
      await auth.loginByLocal(acc.value.username.trim(), acc.value.password)
      onSuccess()
    } catch (e: unknown) {
      err.value = msg(e) || t('login.err.localFailed')
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
      err.value = msg(e) || t('login.err.demoDisabled')
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

  return {
    auth,
    loading,
    err,
    acc,
    localLogin,
    demoLogin,
    oidcLogin,
  }
}
