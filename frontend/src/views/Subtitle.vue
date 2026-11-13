<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  subtitleApi,
  SOURCE_LABEL,
  LANG_LABEL,
  type SubtitleResult,
  type SubtitleConfig,
  type LangCode,
} from '@/api/subtitle'
import SptIcon from '@/components/SptIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const { t } = useI18n()
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const q = ref('')
const language = ref('')
const results = ref<SubtitleResult[]>([])
const loading = ref(false)
const downloading = ref<string | null>(null)
const searched = ref(false)
const error = ref('')

async function search() {
  if (!q.value.trim()) {
    ElMessage.warning(t('subtitle.keywordRequired'))
    return
  }
  loading.value = true
  error.value = ''
  searched.value = true
  try {
    results.value = await subtitleApi.search(q.value.trim(), language.value)
    if (!results.value.length) ElMessage.info(t('subtitle.notFound'))
  } catch (e: any) {
    ElMessage.error(e?.message || t('subtitle.searchFailed'))
    error.value = e?.message || t('subtitle.searchFailed')
    results.value = []
    searched.value = false // 出错时不显示「未找到」，由 error 态承接，避免误导
  } finally {
    loading.value = false
  }
}

async function download(sub: SubtitleResult) {
  const key = `${sub.source}:${sub.source_id}`
  downloading.value = key
  try {
    const { blob, filename } = await subtitleApi.download(sub)
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    ElMessage.success(t('subtitle.downloadedAs', { name: filename }))
  } catch (e: any) {
    // blob 错误体需手动解析（axios 拦截器对 blob 不解析 JSON）
    ElMessage.error(e?.message || t('subtitle.downloadFailed'))
  } finally {
    downloading.value = null
  }
}

// ---- 源配置 ----
const showConfig = ref(false)
const cfg = ref<SubtitleConfig | null>(null)
const cfgForm = ref({
  assrt_token: '',
  opensubtitles_key: '',
  subhd_cookie: '',
  zimuku_cookie: '',
  target_lang: '',
})
const cfgSaving = ref(false)

async function openConfig() {
  try {
    cfg.value = await subtitleApi.getConfig()
    cfgForm.value = {
      assrt_token: '',
      opensubtitles_key: '',
      subhd_cookie: '',
      zimuku_cookie: '',
      target_lang: cfg.value.target_lang || '',
    }
    showConfig.value = true
  } catch (e: any) {
    ElMessage.error(e?.message || t('subtitle.loadCfgFailed'))
  }
}

async function saveConfig() {
  cfgSaving.value = true
  try {
    await subtitleApi.setConfig({
      assrt_token: cfgForm.value.assrt_token,
      opensubtitles_key: cfgForm.value.opensubtitles_key,
      subhd_cookie: cfgForm.value.subhd_cookie,
      zimuku_cookie: cfgForm.value.zimuku_cookie,
      target_lang: cfgForm.value.target_lang,
    })
    ElMessage.success(t('common.saved'))
    showConfig.value = false
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.saveFailed'))
  } finally {
    cfgSaving.value = false
  }
}

// 语义色 + soft 底，避免 hex 与 +'22' 拼接（拼接对 CSS 变量无效）。
const sourceColor: Record<string, string> = {
  assrt: 'var(--color-primary)',
  opensubtitles: 'var(--pt-warn-fg)',
  subhd: 'var(--pt-success-fg)',
  zimuku: 'var(--color-primary-hover)',
}
const sourceSoft: Record<string, string> = {
  assrt: 'var(--color-primary-soft)',
  opensubtitles: 'var(--color-warning-soft)',
  subhd: 'var(--color-success-soft)',
  zimuku: 'var(--color-primary-soft)',
}

onMounted(() => {})
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title">{{ t('subtitle.title') }}</h2>
      <h3 v-else class="title embedded-title">{{ t('subtitle.sourcesTitle') }}</h3>
      <el-button @click="openConfig">{{ t('subtitle.sourcesCfg') }}</el-button>
    </div>
    <p class="sub">
      {{ t('subtitle.subtitle') }}
    </p>

    <div class="search-bar solid-card">
      <SptIcon name="message" :size="28" aria-hidden="true" />
      <el-input v-model="q" :placeholder="t('subtitle.searchPlaceholder')" class="search-input" @keyup.enter="search" />
      <el-select v-model="language" :placeholder="t('subtitle.langPlaceholder')" class="lang-select" clearable>
        <el-option :label="t('subtitle.lang.chs')" value="chs" />
        <el-option :label="t('subtitle.lang.cht')" value="cht" />
        <el-option :label="t('subtitle.lang.eng')" value="eng" />
      </el-select>
      <el-button type="primary" :loading="loading" @click="search">{{ t('common.search') }}</el-button>
    </div>

    <el-skeleton v-if="loading" :rows="5" animated />
    <div v-else class="results">
      <div v-for="(s, i) in results" :key="`${s.source}:${s.source_id}:${i}`" class="solid-card result">
        <div class="result-main">
          <el-tag
            size="small"
            :style="{ backgroundColor: sourceSoft[s.source], color: sourceColor[s.source], border: 'none' }"
          >
            {{ SOURCE_LABEL[s.source] }}
          </el-tag>
          <el-tag size="small" effect="plain">{{
            LANG_LABEL[s.lang_code] || s.language || t('medalwall.status.unknown')
          }}</el-tag>
          <span class="result-title" :title="s.title">{{ s.title }}</span>
        </div>
        <el-button
          size="small"
          type="primary"
          :loading="downloading === `${s.source}:${s.source_id}`"
          @click="download(s)"
          >{{ t('harem.down') }}</el-button
        >
      </div>
      <EmptyState v-if="error && !loading" :title="t('subtitle.searchFailed')" :description="error">
        <el-button @click="search">{{ t('common.retry') }}</el-button>
      </EmptyState>
      <EmptyState
        v-if="searched && !results.length && !loading"
        :title="t('subtitle.notFound')"
        :description="t('subtitle.notFoundDesc')"
      />
      <EmptyState
        v-if="!searched && !loading"
        :title="t('subtitle.searchTitle')"
        :description="t('subtitle.searchDesc')"
      >
        <el-button @click="openConfig">{{ t('subtitle.sourcesCfg') }}</el-button>
      </EmptyState>
    </div>

    <!-- 字幕源配置 -->
    <el-dialog
      class="pt-dialog"
      append-to-body
      align-center
      v-model="showConfig"
      :title="t('subtitle.sourcesCfg')"
      width="560px"
    >
      <el-form :model="cfgForm" label-width="150px" label-position="right">
        <el-form-item label="ASSRT Token">
          <el-input
            v-model="cfgForm.assrt_token"
            type="password"
            show-password
            :placeholder="cfg?.assrt_token_configured ? t('subtitle.f.configuredKeep') : t('subtitle.f.assrtToken')"
          />
        </el-form-item>
        <el-form-item label="OpenSubtitles Key">
          <el-input
            v-model="cfgForm.opensubtitles_key"
            type="password"
            show-password
            :placeholder="cfg?.opensubtitles_key_configured ? t('subtitle.f.configuredKeep') : t('subtitle.f.osKey')"
          />
        </el-form-item>
        <el-form-item label="SubHD Cookie">
          <el-input
            v-model="cfgForm.subhd_cookie"
            type="textarea"
            :rows="2"
            :placeholder="
              cfg?.subhd_cookie_configured ? t('subtitle.f.configuredKeep') : t('subtitle.f.optionalAntiCrawl')
            "
          />
        </el-form-item>
        <el-form-item :label="t('subtitle.f.zimukuCookie')">
          <el-input
            v-model="cfgForm.zimuku_cookie"
            type="textarea"
            :rows="2"
            :placeholder="
              cfg?.zimuku_cookie_configured ? t('subtitle.f.configuredKeep') : t('subtitle.f.optionalAntiCrawl')
            "
          />
        </el-form-item>
        <el-form-item :label="t('subtitle.f.langConvert')">
          <el-select v-model="cfgForm.target_lang" :placeholder="t('subtitle.f.noConvert')" clearable>
            <el-option :label="t('subtitle.f.noConvert')" value="" />
            <el-option :label="t('subtitle.f.toChs')" value="chs" />
            <el-option :label="t('subtitle.f.toCht')" value="cht" />
          </el-select>
          <span class="hint">{{ t('subtitle.f.langConvertHint') }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showConfig = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="cfgSaving" @click="saveConfig">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.title {
  margin: 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.sub {
  margin: -8px 0 0;
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.search-bar {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 12px 14px;
}
.search-input {
  flex: 1;
}
.lang-select {
  width: 110px;
  flex-shrink: 0;
}
.results {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  min-height: 200px;
}
.result {
  padding: 12px 14px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.result-main {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  min-width: 0;
  flex: 1;
}
.result-title {
  font-size: var(--fs-body);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hint {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  display: block;
  margin-top: 4px;
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
