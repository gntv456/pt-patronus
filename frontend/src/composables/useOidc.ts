// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// OIDC/SSO 设置（管理员配置 Authelia/Keycloak/Casdoor 等）。
// 从 Settings.vue 抽出：管理 OIDC Provider 的回显、表单与保存。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { oidcApi, type OIDCConfig, type OIDCConfigInput } from '@/api/oidc'

// OIDC 回调地址（需在 Provider 端登记）：同源 /api/v1/auth/oidc/callback。
const callbackURL = `${window.location.origin}/api/v1/auth/oidc/callback`
const oidcCfg = ref<OIDCConfig | null>(null)
const oidcForm = ref<OIDCConfigInput>({
  enabled: false,
  issuer: '',
  client_id: '',
  client_secret: '',
  scopes: 'openid profile email',
  display_name: 'OIDC / SSO',
  frontend_redirect: '/login',
})
const oidcLoading = ref(false)
const oidcSaving = ref(false)

async function loadOIDC() {
  oidcLoading.value = true
  try {
    oidcCfg.value = await oidcApi.getConfig()
    oidcForm.value = {
      enabled: oidcCfg.value.enabled,
      issuer: oidcCfg.value.issuer,
      client_id: oidcCfg.value.client_id,
      client_secret: '',
      scopes: oidcCfg.value.scopes || 'openid profile email',
      display_name: oidcCfg.value.display_name || 'OIDC / SSO',
      frontend_redirect: oidcCfg.value.frontend_redirect || '/login',
    }
  } catch {
    ElMessage.error('加载 OIDC 配置失败')
  } finally {
    oidcLoading.value = false
  }
}

async function saveOIDC() {
  oidcSaving.value = true
  try {
    await oidcApi.setConfig(oidcForm.value)
    ElMessage.success('已保存')
    await loadOIDC()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    oidcSaving.value = false
  }
}

export function useOidc() {
  return { callbackURL, oidcCfg, oidcForm, oidcLoading, oidcSaving, loadOIDC, saveOIDC }
}
