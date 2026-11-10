<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { mediaConfigApi, type MediaSourceConfig } from '@/api/media'

const { t } = useI18n()
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
const omdbKey = ref('')

async function load() {
  loading.value = true
  try {
    config.value = await mediaConfigApi.get()
    tmdbDomain.value = config.value.tmdb_api_domain || ''
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.loadFailed'))
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
      omdb_api_key: omdbKey.value.trim(),
    })
    ElMessage.success(t('mediasettings.savedNow'))
    tmdbKey.value = ''
    doubanSecret.value = ''
    doubanAppKey.value = ''
    bangumiToken.value = ''
    omdbKey.value = ''
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

// 配置态文案：secret 已配置时显脱敏值，否则「未配置」。
function statusText(masked: string, configured: boolean) {
  return configured ? t('reseed.configuredAs', { v: masked }) : t('reseed.notConfigured')
}

onMounted(load)
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title">{{ t('mediasettings.title') }}</h2>
      <h3 v-else class="title embedded-title">{{ t('mediasettings.title') }}</h3>
      <el-button v-if="!props.embedded" size="small" @click="router.push('/search')">{{
        t('mediasettings.backToSearch')
      }}</el-button>
    </div>

    <div v-loading="loading" class="card glass">
      <div class="hint">
        {{ t('mediasettings.hint1') }}<code>api.themoviedb.org</code>{{ t('mediasettings.hint2')
        }}{{ t('mediasettings.hint3') }}
      </div>

      <!-- TMDB -->
      <div class="group">
        <div class="group-title">
          TMDB <span class="muted">{{ t('mediasettings.tmdbDesc') }}</span>
        </div>
        <div class="field">
          <label>API Key</label>
          <el-input
            v-model="tmdbKey"
            type="password"
            show-password
            clearable
            :placeholder="t('mediasettings.f.emptyKeep')"
          />
          <span class="pt-chip pt-chip--success" v-if="config?.tmdb_api_key_configured">{{
            statusText(config.tmdb_api_key, true)
          }}</span>
          <span class="pt-chip pt-chip--warn" v-else>{{ t('reseed.notConfigured') }}</span>
        </div>
        <div class="field">
          <label>{{ t('mediasettings.f.tmdbDomain') }}</label>
          <el-input v-model="tmdbDomain" clearable placeholder="api.themoviedb.org" />
          <span class="muted">{{ t('mediasettings.f.tmdbDomainHint') }}</span>
        </div>
      </div>

      <!-- 豆瓣 -->
      <div class="group">
        <div class="group-title">
          {{ t('mediasettings.douban') }} <span class="muted">{{ t('mediasettings.doubanDesc') }}</span>
        </div>
        <div class="field">
          <label>{{ t('mediasettings.f.doubanSecret') }}</label>
          <el-input
            v-model="doubanSecret"
            type="password"
            show-password
            clearable
            :placeholder="t('mediasettings.f.emptyDefault')"
          />
          <span class="pt-chip pt-chip--success" v-if="config?.douban_secret_configured">{{
            statusText(config.douban_secret, true)
          }}</span>
          <span class="pt-chip pt-chip--warn" v-else>{{ t('mediasettings.notConfiguredDefault') }}</span>
        </div>
        <div class="field">
          <label>{{ t('mediasettings.f.doubanAppKey') }}</label>
          <el-input
            v-model="doubanAppKey"
            type="password"
            show-password
            clearable
            :placeholder="t('mediasettings.f.emptyDefault')"
          />
          <span class="pt-chip pt-chip--success" v-if="config?.douban_app_key_configured">{{
            statusText(config.douban_app_key, true)
          }}</span>
          <span class="pt-chip pt-chip--warn" v-else>{{ t('mediasettings.notConfiguredDefault') }}</span>
        </div>
      </div>

      <!-- Bangumi -->
      <div class="group">
        <div class="group-title">
          Bangumi <span class="muted">{{ t('mediasettings.bangumiDesc') }}</span>
        </div>
        <div class="field">
          <label>{{ t('mediasettings.f.bangumiToken') }}</label>
          <el-input
            v-model="bangumiToken"
            type="password"
            show-password
            clearable
            :placeholder="t('mediasettings.f.emptyAnonymous')"
          />
          <span class="pt-chip pt-chip--success" v-if="config?.bangumi_token_configured">{{
            statusText(config.bangumi_token, true)
          }}</span>
          <span class="pt-chip pt-chip--warn" v-else>{{ t('mediasettings.notConfiguredAnonymous') }}</span>
        </div>
      </div>

      <!-- IMDb（OMDb）：评分聚合的第四个源，此前界面上没有任何入口 -->
      <div class="group">
        <div class="group-title">
          IMDb <span class="muted">{{ t('mediasettings.omdbDesc') }}</span>
        </div>
        <div class="field">
          <label>{{ t('mediasettings.f.omdbKey') }}</label>
          <el-input
            v-model="omdbKey"
            type="password"
            show-password
            clearable
            :placeholder="t('mediasettings.f.emptyKeep')"
          />
          <span class="pt-chip pt-chip--success" v-if="config?.omdb_api_key_configured">{{
            statusText(config.omdb_api_key, true)
          }}</span>
          <span class="pt-chip pt-chip--warn" v-else>{{ t('mediasettings.omdbSkipped') }}</span>
        </div>
      </div>

      <div class="actions">
        <el-button type="primary" size="large" :loading="saving" @click="save">{{
          t('mediasettings.saveNow')
        }}</el-button>
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
.page.embedded {
  padding: 0;
  max-width: none;
}
.page.embedded .embedded-title {
  font-size: var(--fs-lg);
  font-weight: 600;
  margin: 0;
}
.page.embedded .embedded-sub {
  margin-top: 0;
  margin-bottom: 10px;
  font-size: var(--fs-xs);
}
.page.embedded > .sub {
  display: none;
}
</style>
