<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authApi, type AuthStatus } from '@/api/auth'
import { APP_VERSION, systemApi, type SystemInfo } from '@/api/system'
import { useLoginAuth } from '@/composables/useLoginAuth'
import { oidcFailText } from '@/utils/oidcFail'
import { useI18n } from 'vue-i18n'

const router = useRouter()
const route = useRoute()
const { t } = useI18n()

type Tab = 'account' | 'cookie'
const tab = ref<Tab>('account')
const bootLoading = ref(false)
const systemInfo = ref<SystemInfo | null>(null)
const authStatus = ref<AuthStatus | null>(null)
const serviceErr = ref('')

// 登录表单与各登录方式抽出为 composable（useLoginAuth）。
const { auth, loading, err, captcha, acc, cookieStr, fetchCaptcha, accountLogin, cookieLogin, demoLogin, oidcLogin } =
  useLoginAuth()

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
  await Promise.all([loadBootStatus(), fetchCaptcha()])
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
function doAccountLogin() {
  accountLogin(goDashboard)
}
function doCookieLogin() {
  cookieLogin(goDashboard)
}
function doDemoLogin() {
  demoLogin(goDashboard)
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card glass">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <h1>PTPatronus</h1>
      </div>
      <p class="subtitle">{{ t('login.subtitle') }}</p>

      <div class="mode-tabs">
        <button :class="{ active: tab === 'account' }" @click="tab = 'account'">{{ t('login.tabs.account') }}</button>
        <button :class="{ active: tab === 'cookie' }" @click="tab = 'cookie'">{{ t('login.tabs.cookie') }}</button>
      </div>

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
        <p v-else class="boot-desc">
          {{ authStatus?.auth_site_on ? t('login.boot.vipOn') : t('login.boot.vipOff') }}
        </p>
      </div>

      <!-- 好学账号登录 -->
      <form v-if="tab === 'account'" class="form" @submit.prevent="doAccountLogin">
        <el-input
          v-model="acc.username"
          :placeholder="t('login.form.username')"
          size="large"
          autocomplete="username"
          class="field"
        />
        <el-input
          v-model="acc.password"
          type="password"
          :placeholder="t('login.form.password')"
          size="large"
          show-password
          autocomplete="current-password"
          class="field"
        />
        <el-input
          v-model="acc.twoStep"
          :placeholder="t('login.form.twoStep')"
          size="large"
          autocomplete="one-time-code"
          class="field"
        />
        <div v-if="captcha?.need_captcha" class="captcha-row field">
          <img
            v-if="captcha?.image"
            :src="captcha.image"
            class="captcha-img"
            :alt="t('login.form.captcha')"
            @click="fetchCaptcha"
            :title="t('login.clickRefresh')"
          />
          <el-input
            v-model="acc.imagestring"
            :placeholder="t('login.form.captcha')"
            size="large"
            autocomplete="off"
            class="captcha-input"
          />
        </div>
        <el-button type="primary" size="large" native-type="submit" class="submit" :loading="loading">
          {{ t('login.submit') }}
        </el-button>
        <p v-if="captcha?.challenge_mode" class="hint">{{ t('login.form.challengeHint') }}</p>
      </form>

      <!-- Cookie 登录 -->
      <form v-else class="form" @submit.prevent="doCookieLogin">
        <el-input
          v-model="cookieStr"
          type="textarea"
          :rows="5"
          :placeholder="t('login.form.cookiePlaceholder')"
          autocomplete="off"
          class="field"
        />
        <el-button type="primary" size="large" native-type="submit" class="submit" :loading="loading">
          {{ t('login.submit') }}
        </el-button>
        <p class="hint">{{ t('login.form.cookieHint') }}</p>
      </form>

      <p v-if="err" class="err">{{ err }}</p>
      <button v-if="authStatus?.demo_mode" class="demo-btn" type="button" :disabled="loading" @click="doDemoLogin">
        {{ t('login.demo') }}
      </button>
      <button v-if="authStatus?.oidc_enabled" class="oidc-btn" type="button" :disabled="loading" @click="oidcLogin">
        {{ authStatus?.oidc_display_name || 'OIDC / SSO' }} {{ t('login.oidcSuffix') }}
      </button>
      <div class="deploy-hint">
        {{ t('login.deployHint') }}
        <code>http://192.168.1.10:8088</code>。
      </div>
    </div>
  </div>
</template>

<style scoped src="./Login.css"></style>
