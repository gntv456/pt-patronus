<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { traktApi, type TraktConfig } from '@/api/trakt'
import GlassIcon from '@/components/GlassIcon.vue'
import { ElMessage } from 'element-plus'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const cfg = ref<TraktConfig | null>(null)
const loading = ref(false)

// 配置表单
const showConfig = ref(false)
const form = ref({
  enabled: false,
  client_id: '',
  client_secret: '',
  redirect_uri: 'urn:ietf:wg:oauth:2.0:oob',
  sync_movies: true,
  sync_shows: true,
  resolution: '',
  downloader_id: 0,
})
const saving = ref(false)

// 设备授权
const device = ref<{ user_code: string; verification_url: string } | null>(null)
const authBusy = ref(false)
const syncing = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

async function load() {
  loading.value = true
  try {
    cfg.value = await traktApi.getConfig()
    if (cfg.value.pending_user_code) {
      device.value = {
        user_code: cfg.value.pending_user_code,
        verification_url: cfg.value.pending_verification_url,
      }
      startPolling()
    }
  } catch (e: unknown) {
    ElMessage.error('加载配置失败')
  } finally {
    loading.value = false
  }
}

async function openConfig() {
  if (!cfg.value) await load()
  if (!cfg.value) return
  form.value = {
    enabled: cfg.value.enabled,
    client_id: cfg.value.client_id,
    client_secret: '',
    redirect_uri: cfg.value.redirect_uri || 'urn:ietf:wg:oauth:2.0:oob',
    sync_movies: cfg.value.sync_movies,
    sync_shows: cfg.value.sync_shows,
    resolution: cfg.value.resolution,
    downloader_id: cfg.value.downloader_id,
  }
  showConfig.value = true
}

async function saveConfig() {
  saving.value = true
  try {
    await traktApi.setConfig(form.value)
    ElMessage.success('已保存')
    showConfig.value = false
    await load()
  } catch (e: unknown) {
    ElMessage.error('保存失败')
  } finally {
    saving.value = false
  }
}

async function startDeviceAuth() {
  authBusy.value = true
  try {
    const dc = await traktApi.deviceCode()
    device.value = { user_code: dc.user_code, verification_url: dc.verification_url }
    ElMessage.success('请在打开的页面输入授权码')
    window.open(dc.verification_url, '_blank')
    startPolling()
  } catch (e: unknown) {
    ElMessage.error('发起授权失败，请先在配置填写 client_id/client_secret')
  } finally {
    authBusy.value = false
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const st = await traktApi.deviceStatus()
      if (st.authorized) {
        stopPolling()
        device.value = null
        ElMessage.success('Trakt 授权成功')
        await load()
      }
    } catch {
      /* ignore */
    }
  }, 3000)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function syncNow() {
  syncing.value = true
  try {
    const r = await traktApi.sync()
    ElMessage.success(`同步完成，新增 ${r.created} 条订阅`)
    await load()
  } catch (e: unknown) {
    ElMessage.error('同步失败，请先完成授权')
  } finally {
    syncing.value = false
  }
}

onMounted(load)
onUnmounted(stopPolling)
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title"><GlassIcon name="trakt" :size="26" /> Trakt 同步</h2>
      <h3 v-else class="title embedded-title">Trakt 同步</h3>
      <div class="head-actions">
        <el-button @click="openConfig">配置</el-button>
        <el-button v-if="cfg?.authorized" type="primary" :loading="syncing" @click="syncNow">立即同步</el-button>
        <el-button v-else type="primary" :loading="authBusy" @click="startDeviceAuth">授权 Trakt</el-button>
      </div>
    </div>
    <p class="sub">把 Trakt.tv 的 WatchList（电影/剧集）自动同步为本地订阅，跨站搜种下载。</p>

    <div v-loading="loading" class="glass status-card">
      <div class="status-row">
        <span class="status-dot" :class="cfg?.authorized ? 'on' : 'off'"></span>
        <span>{{ cfg?.authorized ? '已授权' : '未授权' }}</span>
        <span v-if="cfg?.enabled" class="badge">同步已开启</span>
      </div>
      <div v-if="cfg?.last_sync_at" class="meta">
        最近同步：{{ new Date(cfg.last_sync_at * 1000).toLocaleString() }} · 新增 {{ cfg.last_sync_count }} 条
      </div>
      <div v-if="cfg?.last_error" class="err">{{ cfg.last_error }}</div>

      <!-- 设备授权引导 -->
      <div v-if="device" class="device">
        <p>1. 打开：<a :href="device.verification_url" target="_blank">{{ device.verification_url }}</a></p>
        <p>2. 输入授权码：</p>
        <code class="uc">{{ device.user_code }}</code>
        <p class="muted">授权完成后本页自动检测，无需刷新。</p>
      </div>
    </div>

    <el-dialog class="pt-dialog" append-to-body align-center v-model="showConfig" title="Trakt 配置" width="520px">
      <el-form :model="form" label-width="150px" label-position="right">
        <el-form-item label="启用定时同步">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="Client ID">
          <el-input v-model="form.client_id" placeholder="trakt.tv 创建应用所得" />
        </el-form-item>
        <el-form-item label="Client Secret">
          <el-input
            v-model="form.client_secret"
            type="password"
            show-password
            :placeholder="cfg?.client_secret_configured ? '已配置（留空=不修改）' : 'trakt.tv 创建应用所得'"
          />
        </el-form-item>
        <el-form-item label="Redirect URI">
          <el-input v-model="form.redirect_uri" placeholder="device 流用 urn:ietf:wg:oauth:2.0:oob" />
        </el-form-item>
        <el-form-item label="同步电影">
          <el-switch v-model="form.sync_movies" />
        </el-form-item>
        <el-form-item label="同步剧集">
          <el-switch v-model="form.sync_shows" />
        </el-form-item>
        <el-form-item label="默认分辨率">
          <el-input v-model="form.resolution" placeholder="如 1080p（留空不过滤）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-3); }
.head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
.head .title { display: flex; align-items: center; gap: var(--sp-2); margin: 0; font-size: var(--fs-title); font-weight: 700; }
.head-actions { display: flex; gap: var(--sp-2); }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.status-card { padding: 16px; display: flex; flex-direction: column; gap: var(--sp-2); }
.status-row { display: flex; align-items: center; gap: var(--sp-2); font-weight: 600; }
.status-dot { width: 10px; height: 10px; border-radius: 50%; }
.status-dot.on { background:var(--pt-success); }
.status-dot.off { background:var(--pt-danger); }
.badge { padding: 2px 8px; border-radius: var(--radius-sm); background: var(--pt-primary-bg); font-size: var(--fs-xs); color:var(--pt-primary); }
.meta { font-size: var(--fs-body); color: var(--text-muted); }
.err { color:var(--pt-danger); font-size: var(--fs-body); }
.device { padding-top: 8px; border-top: 1px solid var(--border); }
.uc { font-size: var(--fs-hero); font-weight: 700; letter-spacing: 3px; background: var(--card); padding: 6px 14px; border-radius: var(--radius-xs); }
.muted { font-size: var(--fs-xs); color: var(--text-muted); }

/* embedded：嵌进设置页时去掉外页边距，标题交给分区 Tab */
.page.embedded { padding: 0; max-width: none; }
.page.embedded .embedded-title { font-size: var(--fs-lg); font-weight: 600; margin: 0; }
.page.embedded .embedded-sub { margin-top: 0; margin-bottom: 10px; font-size: var(--fs-xs); }
.page.embedded > .sub { display: none; }

</style>
