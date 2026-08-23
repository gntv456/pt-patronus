<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authApi, type AuthStatus } from '@/api/auth'
import { APP_VERSION, systemApi, type SystemInfo } from '@/api/system'
import { useLoginAuth } from '@/composables/useLoginAuth'

const router = useRouter()
const route = useRoute()

type Tab = 'account' | 'cookie'
const tab = ref<Tab>('account')
const bootLoading = ref(false)
const systemInfo = ref<SystemInfo | null>(null)
const authStatus = ref<AuthStatus | null>(null)
const serviceErr = ref('')

// 登录表单与各登录方式抽出为 composable（useLoginAuth）。
const { auth, loading, err, captcha, acc, cookieStr, fetchCaptcha, accountLogin, cookieLogin, demoLogin, oidcLogin } = useLoginAuth()

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
  const oidcToken = oidcHash.get('oidc_token') ?? (typeof route.query.oidc_token === 'string' ? route.query.oidc_token : '')
  if (oidcToken) {
    await auth.loginByToken(oidcToken)
    router.replace({ name: 'dashboard' })
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
  } catch {
    serviceErr.value = '无法连接后端服务，请确认 Docker/后端已启动'
  } finally {
    bootLoading.value = false
  }
}

// 包装登录成功回调：跳转首页。
function doAccountLogin() { accountLogin(goDashboard) }
function doCookieLogin() { cookieLogin(goDashboard) }
function doDemoLogin() { demoLogin(goDashboard) }
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
      <form v-if="tab === 'account'" class="form" @submit.prevent="doAccountLogin">
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
      <form v-else class="form" @submit.prevent="doCookieLogin">
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
      <button v-if="authStatus?.demo_mode" class="demo-btn" type="button" :disabled="loading" @click="doDemoLogin">
        浏览演示环境
      </button>
      <button v-if="authStatus?.oidc_enabled" class="oidc-btn" type="button" :disabled="loading" @click="oidcLogin">
        {{ authStatus?.oidc_display_name || 'OIDC / SSO' }} 登录
      </button>
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
  box-sizing: border-box;
}
.brand {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.logo {
  width: 40px;
  height: 40px;
}
.brand h1 {
  margin: 0;
  font-size: var(--fs-title);
  letter-spacing: -0.5px;
}
.subtitle {
  margin: 4px 0 22px;
  color: var(--text-muted);
  font-size: var(--fs-md);
}
.mode-tabs {
  display: flex;
  gap: var(--sp-2);
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
  font-size: var(--fs-md);
}
.mode-tabs button.active {
  background: linear-gradient(135deg, var(--pt-primary-bg), var(--pt-primary-2-bg));
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
  border-color: var(--pt-danger-bg);
}
.boot-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  font-size: var(--fs-body);
  color: var(--text-strong);
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--text-muted);
}
.dot.ok {
  background:var(--pt-success);
}
.dot.warn {
  background:var(--pt-warn);
}
.dot.bad {
  background: var(--pt-danger);
}
.boot-version {
  margin-left: auto;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
.boot-desc {
  margin: 6px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-xs);
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
  gap: var(--sp-2);
  align-items: center;
}
.captcha-img {
  height: 40px;
  border-radius: var(--radius-xs);
  cursor: pointer;
  border: 1px solid var(--glass-border);
}
.captcha-input {
  flex: 1;
}
.hint {
  margin-top: 10px;
  font-size: var(--fs-xs);
  color: var(--text-muted);
  line-height: 1.5;
}
.err {
  margin-top: 12px;
  font-size: var(--fs-body);
  color: var(--pt-danger);
}
.deploy-hint {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px solid var(--glass-border);
  color: var(--text-muted);
  font-size: var(--fs-xs);
  line-height: 1.55;
}
.deploy-hint code {
  color: var(--text-strong);
}
.demo-btn {
  width: 100%;
  margin-top: 12px;
  height: 38px;
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  background: rgba(255, 255, 255, 0.42);
  color: var(--pt-primary);
  font-weight: 600;
  cursor: pointer;
}
.demo-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.oidc-btn {
  width: 100%;
  margin-top: 8px;
  height: 38px;
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  background: var(--pt-primary-bg);
  color:var(--pt-primary);
  font-weight: 600;
  cursor: pointer;
}
.oidc-btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
@media (max-width: 520px) {
  .login-wrap {
    align-items: flex-start;
    padding: 12px;
  }
  .login-card {
    margin-top: 36px;
    padding: 28px 22px;
  }
  .brand h1 {
    font-size: var(--fs-title);
  }
}

/* 水润动效增强 */
.login-card { animation: pt-scale-in var(--dur-slow) var(--ease-out-spring) both; }
.mode-tabs button { transition: all var(--dur-base) var(--ease-out-smooth); }
.mode-tabs button:hover { transform: translateY(-1px); }
.submit { transition: transform var(--dur-fast) var(--ease-out-smooth), box-shadow var(--dur-base) var(--ease-out-smooth), filter var(--dur-fast) var(--ease-out-smooth); box-shadow: var(--shadow-primary); }
.submit:hover { transform: translateY(-1px); box-shadow: var(--shadow-primary-lg); filter: brightness(1.05); }
.submit:active { transform: translateY(0) scale(0.985); }
.demo-btn { transition: all var(--dur-base) var(--ease-out-smooth); }
.demo-btn:hover { transform: translateY(-1px); background: var(--pt-primary-bg); }
.captcha-img { transition: transform var(--dur-fast) var(--ease-out-spring), opacity var(--dur-fast) var(--ease-out-smooth); }
.captcha-img:hover { transform: scale(1.04); }
.dot { transition: background var(--dur-base) var(--ease-out-smooth); }
</style>
