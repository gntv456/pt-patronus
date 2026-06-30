<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { authApi, type CaptchaChallenge } from '@/api/auth'

const router = useRouter()
const auth = useAuthStore()

type Tab = 'account' | 'cookie'
const tab = ref<Tab>('account')
const loading = ref(false)
const err = ref('')

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
  await fetchCaptcha()
})

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
  if (!cookieStr.value.trim()) {
    err.value = '请粘贴好学 cookie'
    return
  }
  loading.value = true
  err.value = ''
  try {
    await auth.loginByCookie(cookieStr.value.trim())
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

      <!-- 好学账号登录 -->
      <div v-if="tab === 'account'" class="form">
        <el-input v-model="acc.username" placeholder="好学用户名" size="large" class="field" />
        <el-input
          v-model="acc.password"
          type="password"
          placeholder="好学密码"
          size="large"
          show-password
          class="field"
        />
        <el-input
          v-model="acc.twoStep"
          placeholder="二级验证码（开了 2FA 才填，6 位）"
          size="large"
          class="field"
        />
        <div v-if="captcha?.need_captcha" class="captcha-row field">
          <img v-if="captcha?.image" :src="captcha.image" class="captcha-img" alt="验证码" @click="fetchCaptcha" title="点击刷新" />
          <el-input v-model="acc.imagestring" placeholder="验证码" size="large" class="captcha-input" />
        </div>
        <el-button type="primary" size="large" class="submit" :loading="loading" @click="accountLogin">
          登录
        </el-button>
        <p v-if="captcha?.challenge_mode" class="hint">好学已开启挑战响应认证，后端会自动处理，直接登录即可；如登录失败可改用 Cookie 登录</p>
      </div>

      <!-- Cookie 登录 -->
      <div v-else class="form">
        <el-input
          v-model="cookieStr"
          type="textarea"
          :rows="5"
          placeholder="粘贴好学 cookie（如 c_secure_uid=...; c_secure_pass=...），浏览器登录好学后 F12 复制"
          class="field"
        />
        <el-button type="primary" size="large" class="submit" :loading="loading" @click="cookieLogin">
          登录
        </el-button>
        <p class="hint">最稳路径，覆盖开了二级验证 / 挑战响应的好学账号</p>
      </div>

      <p v-if="err" class="err">{{ err }}</p>
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
.field {
  margin-bottom: 14px;
}
.submit {
  width: 100%;
  margin-top: 4px;
  background: linear-gradient(135deg, #6c5ce7, #4285f4);
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
</style>
