<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
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
const savingGroup = ref<string | null>(null)
const domainError = ref('')

const dirtyTmdb = computed(() => !!tmdbKey.value.trim() || tmdbDomain.value.trim() !== (config.value?.tmdb_api_domain || ''))
const dirtyDouban = computed(() => !!doubanSecret.value.trim() || !!doubanAppKey.value.trim())
const dirtyBangumi = computed(() => !!bangumiToken.value.trim())
const dirtyOmdb = computed(() => !!omdbKey.value.trim())

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

async function save(group?: 'tmdb' | 'douban' | 'bangumi' | 'omdb') {
  domainError.value = ''
  if ((!group || group === 'tmdb') && tmdbDomain.value.trim() && !/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(tmdbDomain.value.trim())) {
    domainError.value = t('mediasettings.domainInvalid')
    return
  }
  const payload: Record<string, string> = {}
  if (!group || group === 'tmdb') {
    payload.tmdb_api_key = tmdbKey.value.trim()
    payload.tmdb_api_domain = tmdbDomain.value.trim()
  }
  if (!group || group === 'douban') {
    payload.douban_secret = doubanSecret.value.trim()
    payload.douban_app_key = doubanAppKey.value.trim()
  }
  if (!group || group === 'bangumi') payload.bangumi_token = bangumiToken.value.trim()
  if (!group || group === 'omdb') payload.omdb_api_key = omdbKey.value.trim()
  saving.value = true
  savingGroup.value = group ?? 'all'
  try {
    await mediaConfigApi.set(payload)
    ElMessage.success(t('mediasettings.savedNow'))
    if (!group || group === 'tmdb') tmdbKey.value = ''
    if (!group || group === 'douban') { doubanSecret.value = ''; doubanAppKey.value = '' }
    if (!group || group === 'bangumi') bangumiToken.value = ''
    if (!group || group === 'omdb') omdbKey.value = ''
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    saving.value = false
    savingGroup.value = null
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

    <el-skeleton v-if="loading" :rows="6" animated />
    <div v-else class="card solid-card">
      <div class="hint">
        {{ t('mediasettings.hint1') }}<code>api.themoviedb.org</code>{{ t('mediasettings.hint2')
        }}{{ t('mediasettings.hint3') }}
      </div>

      <!-- TMDB -->
      <div class="group">
        <div class="group-title">
          <span>TMDB <span class="muted">{{ t('mediasettings.tmdbDesc') }}</span></span>
          <span class="group-actions">
            <span v-if="dirtyTmdb" class="pt-chip pt-chip--warn">{{ t('mediasettings.unsaved') }}</span>
            <el-button size="small" type="primary" :loading="savingGroup === 'tmdb'" :disabled="!dirtyTmdb" @click="save('tmdb')">{{ t('common.save') }}</el-button>
          </span>
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
          <el-input v-model="tmdbDomain" clearable placeholder="api.themoviedb.org" :aria-invalid="!!domainError" aria-describedby="tmdb-domain-error" @input="domainError = ''" />
          <span class="muted">{{ t('mediasettings.f.tmdbDomainHint') }}</span>
          <div v-if="domainError" id="tmdb-domain-error" role="alert" class="field-error">{{ domainError }}</div>
        </div>
      </div>

      <!-- 豆瓣 -->
      <div class="group">
        <div class="group-title">
          <span>{{ t('mediasettings.douban') }} <span class="muted">{{ t('mediasettings.doubanDesc') }}</span></span>
          <span class="group-actions">
            <span v-if="dirtyDouban" class="pt-chip pt-chip--warn">{{ t('mediasettings.unsaved') }}</span>
            <el-button size="small" type="primary" :loading="savingGroup === 'douban'" :disabled="!dirtyDouban" @click="save('douban')">{{ t('common.save') }}</el-button>
          </span>
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
          <span>Bangumi <span class="muted">{{ t('mediasettings.bangumiDesc') }}</span></span>
          <span class="group-actions">
            <span v-if="dirtyBangumi" class="pt-chip pt-chip--warn">{{ t('mediasettings.unsaved') }}</span>
            <el-button size="small" type="primary" :loading="savingGroup === 'bangumi'" :disabled="!dirtyBangumi" @click="save('bangumi')">{{ t('common.save') }}</el-button>
          </span>
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
          <span>IMDb <span class="muted">{{ t('mediasettings.omdbDesc') }}</span></span>
          <span class="group-actions">
            <span v-if="dirtyOmdb" class="pt-chip pt-chip--warn">{{ t('mediasettings.unsaved') }}</span>
            <el-button size="small" type="primary" :loading="savingGroup === 'omdb'" :disabled="!dirtyOmdb" @click="save('omdb')">{{ t('common.save') }}</el-button>
          </span>
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
  padding: var(--card-padding);
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
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  flex-wrap: wrap;
  font-weight: 700;
  font-size: var(--fs-lg);
  padding-bottom: var(--space-2);
  border-bottom: 1px solid var(--color-border);
}
.group-actions { display: inline-flex; align-items: center; gap: var(--space-2); }
.field-error { grid-column: 2 / -1; color: var(--pt-danger-fg); font-size: var(--fs-xs); }
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
