<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted, watch, nextTick } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authApi, type AuthStatus } from '@/api/auth'
import { APP_VERSION, systemApi, type SystemInfo } from '@/api/system'
import { useLoginAuth } from '@/composables/useLoginAuth'
import { oidcFailText } from '@/utils/oidcFail'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()

const bootLoading = ref(false)
const systemInfo = ref<SystemInfo | null>(null)
const authStatus = ref<AuthStatus | null>(null)
const serviceErr = ref('')

// 登录表单与各登录方式抽出为 composable（useLoginAuth）。
// 旧好学账号/Cookie 登录 Tab 已随站点改版（FluxTorrent）下线删除。
const { auth, loading, err, acc, localLogin, demoLogin, oidcLogin } = useLoginAuth()

// 登录失败后把焦点移到可聚焦的错误摘要，便于键盘 / 读屏用户感知。
const errRef = ref<HTMLElement | null>(null)
watch(err, (v) => {
  if (v) void nextTick(() => errRef.value?.focus())
})

function goDashboard() {
  router.push({ name: 'dashboard' })
}

onMounted(async () => {
  if (auth.authenticated) {
    router.push({ name: 'dashboard' })
    return
  }
  // OIDC/SSO 回调：从 URL fragment(#oidc_token=) 取后端签发的 JWT（302 跳转回来）。存 token 后进首页。
  // fragment 不进服务器日志/Referrer/浏览器历史，比 query 安全（仍建议生产走 HTTPS）。
  // 兜底读 query，兼容旧后端可能仍签发的 query 跳转。
  const oidcHash = new URLSearchParams((route.hash || '').replace(/^#/, ''))
  const oidcToken =
    oidcHash.get('oidc_token') ?? (typeof route.query.oidc_token === 'string' ? route.query.oidc_token : '')
  if (oidcToken) {
    await auth.loginByToken(oidcToken)
    router.replace({ name: 'dashboard' })
    return
  }
  // SSO 失败：后端把错误码放进 fragment 回跳（整页导航下 JSON 信封会被浏览器渲染成一屏 JSON）。
  const oidcError = oidcHash.get('oidc_error')
  if (oidcError) {
    err.value = oidcFailText(oidcError)
    router.replace({ path: route.path, query: route.query }) // 抹掉 fragment，刷新不重播旧错误
  }
  await loadBootStatus()
})

async function loadBootStatus() {
  bootLoading.value = true
  serviceErr.value = ''
  try {
    const [info, status] = await Promise.all([systemApi.info(), authApi.status()])
    systemInfo.value = info
    authStatus.value = status
  } catch {
    serviceErr.value = t('login.boot.serviceErr')
  } finally {
    bootLoading.value = false
  }
}

// 包装登录成功回调：跳转首页。
function doLocalLogin() {
  localLogin(goDashboard)
}
function doDemoLogin() {
  demoLogin(goDashboard)
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card solid-card">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <h1>PTPatronus</h1>
      </div>
      <p class="subtitle">{{ t('login.subtitle') }}</p>

      <div class="boot-panel" :class="{ bad: serviceErr }">
        <div class="boot-row">
          <span class="dot" :class="{ ok: !serviceErr && !bootLoading, warn: bootLoading, bad: serviceErr }"></span>
          <span>{{
            bootLoading
              ? t('login.boot.checking')
              : serviceErr
                ? t('login.boot.unreachable')
                : t('login.boot.connected')
          }}</span>
          <span class="boot-version">{{ systemInfo?.version || APP_VERSION }}</span>
        </div>
        <p v-if="serviceErr" class="boot-desc">{{ serviceErr }}</p>
        <p v-else class="boot-desc">{{ t('login.boot.localOn') }}</p>
      </div>

      <!-- 本地密码登录（好学站点关闭后的主路径） -->
      <form class="form" novalidate @submit.prevent="doLocalLogin">
        <div class="field">
          <label class="field-label" for="login-username">{{ t('login.localForm.username') }}</label>
          <el-input
            id="login-username"
            v-model="acc.username"
            :placeholder="t('login.localForm.username')"
            size="large"
            autocomplete="username"
            aria-describedby="login-hint"
            :aria-invalid="!!err"
          />
        </div>
        <div class="field">
          <label class="field-label" for="login-password">{{ t('login.localForm.password') }}</label>
          <el-input
            id="login-password"
            v-model="acc.password"
            type="password"
            :placeholder="t('login.localForm.password')"
            size="large"
            show-password
            autocomplete="current-password"
            :aria-invalid="!!err"
          />
        </div>

        <p v-if="err" ref="errRef" class="err" role="alert" tabindex="-1">{{ err }}</p>

        <el-button
          type="primary"
          size="large"
          native-type="submit"
          class="submit"
          :loading="loading"
        >
          {{ t('login.submit') }}
        </el-button>
        <p id="login-hint" class="hint">{{ t('login.localForm.firstLoginHint') }}</p>
      </form>

      <div
        v-if="authStatus?.demo_mode || authStatus?.oidc_enabled"
        class="alt-divider"
        role="separator"
      ></div>
      <div class="alt-actions">
        <button v-if="authStatus?.demo_mode" class="link-btn" type="button" :disabled="loading" @click="doDemoLogin">
          {{ t('login.demo') }}
        </button>
        <button v-if="authStatus?.oidc_enabled" class="link-btn" type="button" :disabled="loading" @click="oidcLogin">
          {{ authStatus?.oidc_display_name || 'OIDC / SSO' }} {{ t('login.oidcSuffix') }}
        </button>
      </div>
      <div class="deploy-hint">
        {{ t('login.deployHint') }}
        <code>http://your-server:8088</code>。
      </div>
    </div>
  </div>
</template>

<style scoped src="./Login.css"></style>
