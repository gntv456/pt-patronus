<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { authApi, type AuthStatus, type CaptchaChallenge } from '@/api/auth'
import { APP_VERSION, systemApi, type SystemInfo } from '@/api/system'

const router = useRouter()
const auth = useAuthStore()

type Tab = 'account' | 'cookie'
const tab = ref<Tab>('account')
const loading = ref(false)
const err = ref('')
const bootLoading = ref(false)
const systemInfo = ref<SystemInfo | null>(null)
const authStatus = ref<AuthStatus | null>(null)
const serviceErr = ref('')

// 账号登录
const acc = ref({ username: '', password: '', twoStep: '', imagestring: '' })
const captcha = ref<CaptchaChallenge | null>(null)
// Cookie 登录
const cookieStr = ref('')

onMounted(async () => {
  if (auth.authenticated) {
    router.push({ name: 'dashboard' })
    return
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
  } catch (e: unknown) {
    serviceErr.value = msg(e) || '无法连接后端服务，请确认 Docker/后端已启动'
  } finally {
    bootLoading.value = false
  }
}

async function fetchCaptcha() {
  try {
    captcha.value = await authApi.hxptCaptcha()
  } catch (e: unknown) {
    err.value = msg(e) || '取验证码失败（可改用 Cookie 登录）'
  }
}

async function accountLogin() {
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
    router.push({ name: 'dashboard' })
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

async function cookieLogin() {
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
    router.push({ name: 'dashboard' })
  } catch (e: unknown) {
    err.value = msg(e) || '登录失败'
  } finally {
    loading.value = false
  }
}

function msg(e: unknown): string {
  const r = (e as { response?: { data?: { message?: string; error?: string } } })?.response?.data
  return r?.message || r?.error || ''
}
</script>

<template>
  <div class="login-wrap">
    <div class="login-card glass">
      <div class="brand">
        <img src="/logo-mark.svg" alt="PTPatronus" class="logo" />
        <h1>PTPatronus</h1>
      </div>
      <p class="subtitle">PT 守护神 · 好学 APP 专属</p>

      <div class="mode-tabs">
        <button :class="{ active: tab === 'account' }" @click="tab = 'account'">好学账号</button>
        <button :class="{ active: tab === 'cookie' }" @click="tab = 'cookie'">Cookie 登录</button>
      </div>

      <div class="boot-panel" :class="{ bad: serviceErr }">
        <div class="boot-row">
          <span class="dot" :class="{ ok: !serviceErr && !bootLoading, warn: bootLoading, bad: serviceErr }"></span>
          <span>{{ bootLoading ? '正在检查后端服务' : serviceErr ? '后端不可用' : '后端已连接' }}</span>
          <span class="boot-version">{{ systemInfo?.version || APP_VERSION }}</span>
        </div>
        <p v-if="serviceErr" class="boot-desc">{{ serviceErr }}</p>
        <p v-else class="boot-desc">
          {{ authStatus?.auth_site_on ? '当前实例使用好学 VIP 准入，首个通过验证的用户会成为管理员。' : '当前实例未开启好学准入限制。' }}
        </p>
      </div>

      <!-- 好学账号登录 -->
      <form v-if="tab === 'account'" class="form" @submit.prevent="accountLogin">
        <el-input v-model="acc.username" placeholder="好学用户名" size="large" autocomplete="username" class="field" />
        <el-input
          v-model="acc.password"
          type="password"
          placeholder="好学密码"
          size="large"
          show-password
          autocomplete="current-password"
          class="field"
        />
        <el-input
          v-model="acc.twoStep"
          placeholder="二级验证码（开了 2FA 才填，6 位）"
          size="large"
          autocomplete="one-time-code"
          class="field"
        />
        <div v-if="captcha?.need_captcha" class="captcha-row field">
          <img v-if="captcha?.image" :src="captcha.image" class="captcha-img" alt="验证码" @click="fetchCaptcha" title="点击刷新" />
          <el-input v-model="acc.imagestring" placeholder="验证码" size="large" autocomplete="off" class="captcha-input" />
        </div>
        <el-button type="primary" size="large" native-type="submit" class="submit" :loading="loading">
          登录
        </el-button>
        <p v-if="captcha?.challenge_mode" class="hint">好学已开启挑战响应认证，后端会自动处理，直接登录即可；如登录失败可改用 Cookie 登录</p>
      </form>

      <!-- Cookie 登录 -->
      <form v-else class="form" @submit.prevent="cookieLogin">
        <el-input
          v-model="cookieStr"
          type="textarea"
          :rows="5"
          placeholder="粘贴好学 cookie（如 c_secure_uid=...; c_secure_pass=...），浏览器登录好学后 F12 复制"
          autocomplete="off"
          class="field"
        />
        <el-button type="primary" size="large" native-type="submit" class="submit" :loading="loading">
          登录
        </el-button>
        <p class="hint">最稳路径，覆盖开了二级验证 / 挑战响应的好学账号</p>
      </form>

      <p v-if="err" class="err">{{ err }}</p>
      <div class="deploy-hint">
        Web 端跟随后端同源访问 API。若移动端首次连接，请在客户端填写你的 Docker/后端地址，例如
        <code>http://192.168.1.10:8088</code>。
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  padding: 24px;
}
.login-card {
  width: 100%;
  max-width: 400px;
  padding: 36px 32px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
}
.logo {
  width: 40px;
  height: 40px;
}
.brand h1 {
  margin: 0;
  font-size: 24px;
  letter-spacing: -0.5px;
}
.subtitle {
  margin: 4px 0 22px;
  color: var(--text-muted);
  font-size: 14px;
}
.mode-tabs {
  display: flex;
  gap: 8px;
  margin-bottom: 18px;
}
.mode-tabs button {
  flex: 1;
  padding: 9px;
  border-radius: var(--radius-md);
  border: 1px solid var(--glass-border);
  background: transparent;
  cursor: pointer;
  color: var(--text-muted);
  font-size: 14px;
}
.mode-tabs button.active {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.25), rgba(66, 133, 244, 0.18));
  color: var(--text-strong);
  font-weight: 600;
}
.mode-tabs button {
  appearance: none;
}
.boot-panel {
  margin: -6px 0 16px;
  padding: 10px 12px;
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  background: rgba(255, 255, 255, 0.36);
}
.boot-panel.bad {
  border-color: rgba(231, 76, 60, 0.28);
}
.boot-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-strong);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}
.dot.ok {
  background: #27ae60;
}
.dot.warn {
  background: #f39c12;
}
.dot.bad {
  background: var(--pt-danger);
}
.boot-version {
  margin-left: auto;
  color: var(--text-muted);
  font-size: 12px;
}
.boot-desc {
  margin: 6px 0 0;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
}
.field {
  margin-bottom: 14px;
}
.submit {
  width: 100%;
  margin-top: 4px;
  background: var(--grad-primary);
  border: none;
  border-radius: var(--radius-md);
  height: 44px;
  font-weight: 600;
}
.captcha-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.captcha-img {
  height: 40px;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid var(--glass-border);
}
.captcha-input {
  flex: 1;
}
.hint {
  margin-top: 10px;
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.5;
}
.err {
  margin-top: 12px;
  font-size: 13px;
  color: var(--pt-danger);
}
.deploy-hint {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--glass-border);
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.55;
}
.deploy-hint code {
  color: var(--text-strong);
}
</style>
