<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { mediaConfigApi, type MediaSourceConfig } from '@/api/media'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

// 媒体源设置：TMDB/豆瓣/Bangumi 凭据。secret 回显脱敏；提交时空字段=不修改（可只改部分）。
// 保存即时 rebuild 源，无需重启。参考 Reseed.vue 的 token 配置范式。
const router = useRouter()
const config = ref<MediaSourceConfig | null>(null)
const loading = ref(false)
const saving = ref(false)

// 编辑态：secret 留空=不修改；域名预填当前值便于原地编辑。
const tmdbKey = ref('')
const tmdbDomain = ref('')
const doubanSecret = ref('')
const doubanAppKey = ref('')
const bangumiToken = ref('')

async function load() {
  loading.value = true
  try {
    config.value = await mediaConfigApi.get()
    tmdbDomain.value = config.value.tmdb_api_domain || ''
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await mediaConfigApi.set({
      tmdb_api_key: tmdbKey.value.trim(),
      tmdb_api_domain: tmdbDomain.value.trim(),
      douban_secret: doubanSecret.value.trim(),
      douban_app_key: doubanAppKey.value.trim(),
      bangumi_token: bangumiToken.value.trim(),
    })
    ElMessage.success('已保存，即时生效')
    tmdbKey.value = ''
    doubanSecret.value = ''
    doubanAppKey.value = ''
    bangumiToken.value = ''
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

// 配置态文案：secret 已配置时显脱敏值，否则「未配置」。
function statusText(masked: string, configured: boolean) {
  return configured ? `已配置 ${masked}` : '未配置'
}

onMounted(load)
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title">媒体源设置</h2>
      <h3 v-else class="title embedded-title">媒体源设置</h3>
      <el-button v-if="!props.embedded" size="small" @click="router.push('/search')">返回搜索</el-button>
    </div>

    <div v-loading="loading" class="card glass">
      <div class="hint">
        影视搜索 / 推荐从这里取 TMDB、豆瓣、Bangumi 的凭据。留空 = 沿用默认（不修改）；
        国内访问 <code>api.themoviedb.org</code> 超时时，可在「TMDB 域名 / 镜像」填一个可达镜像。
        保存后即时生效，无需重启。
      </div>

      <!-- TMDB -->
      <div class="group">
        <div class="group-title">TMDB <span class="muted">影视主源 · 国际片元数据</span></div>
        <div class="field">
          <label>API Key</label>
          <el-input v-model="tmdbKey" type="password" show-password clearable placeholder="留空不修改" />
          <span class="chip ok" v-if="config?.tmdb_api_key_configured">{{ statusText(config.tmdb_api_key, true) }}</span>
          <span class="chip warn" v-else>未配置</span>
        </div>
        <div class="field">
          <label>域名 / 镜像</label>
          <el-input v-model="tmdbDomain" clearable placeholder="api.themoviedb.org" />
          <span class="muted">官方 api.themoviedb.org 国内常被墙；可换镜像</span>
        </div>
      </div>

      <!-- 豆瓣 -->
      <div class="group">
        <div class="group-title">豆瓣 <span class="muted">国产影视 · 零配置可用（公开默认凭据）</span></div>
        <div class="field">
          <label>Secret（签名密钥）</label>
          <el-input v-model="doubanSecret" type="password" show-password clearable placeholder="留空用公开默认" />
          <span class="chip ok" v-if="config?.douban_secret_configured">{{ statusText(config.douban_secret, true) }}</span>
          <span class="chip warn" v-else>未配置（用公开默认）</span>
        </div>
        <div class="field">
          <label>AppKey</label>
          <el-input v-model="doubanAppKey" type="password" show-password clearable placeholder="留空用公开默认" />
          <span class="chip ok" v-if="config?.douban_app_key_configured">{{ statusText(config.douban_app_key, true) }}</span>
          <span class="chip warn" v-else>未配置（用公开默认）</span>
        </div>
      </div>

      <!-- Bangumi -->
      <div class="group">
        <div class="group-title">Bangumi <span class="muted">动漫 · 匿名可用（可选 token 提速）</span></div>
        <div class="field">
          <label>Token（可选）</label>
          <el-input v-model="bangumiToken" type="password" show-password clearable placeholder="留空走匿名配额" />
          <span class="chip ok" v-if="config?.bangumi_token_configured">{{ statusText(config.bangumi_token, true) }}</span>
          <span class="chip warn" v-else>未配置（匿名）</span>
        </div>
      </div>

      <div class="actions">
        <el-button type="primary" size="large" :loading="saving" @click="save">保存（即时生效）</el-button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.card {
  padding: 20px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.hint {
  font-size: var(--fs-body);
  line-height: 1.7;
  color: var(--text-muted);
  background: var(--pt-primary-bg);
  padding: 12px 14px;
  border-radius: var(--radius-sm);
}
.hint code {
  background: var(--surface-muted);
  padding: 1px 5px;
  border-radius: var(--radius-2xs);
  font-size: var(--fs-xs);
}
.group {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.group-title {
  font-weight: 700;
  font-size: var(--fs-lg);
  padding-bottom: 6px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
}
.muted {
  color: var(--text-muted);
  font-weight: 400;
  font-size: var(--fs-xs);
}
.field {
  display: grid;
  grid-template-columns: 140px 1fr auto;
  align-items: center;
  gap: var(--sp-3);
}
.field label {
  font-size: var(--fs-body);
  color: var(--text-strong);
}
.chip {
  font-size: var(--fs-xs);
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  white-space: nowrap;
}
.chip.ok {
  background: var(--pt-success-bg);
  color: var(--pt-success, #2ecc71);
}
.chip.warn {
  background: var(--pt-warn-bg);
  color: var(--pt-warn, #f39c12);
}
.actions {
  display: flex;
  justify-content: flex-end;
}
@media (max-width: 640px) {
  .field {
    grid-template-columns: 1fr;
    align-items: start;
  }
}

/* embedded：嵌进设置页时去掉外页边距，标题交给分区 Tab */
.page.embedded { padding: 0; max-width: none; }
.page.embedded .embedded-title { font-size: var(--fs-lg); font-weight: 600; margin: 0; }
.page.embedded .embedded-sub { margin-top: 0; margin-bottom: 10px; font-size: var(--fs-xs); }
.page.embedded > .sub { display: none; }

</style>
