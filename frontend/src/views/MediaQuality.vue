<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  mediaQualityApi,
  sourceOptions,
  sourceLabel,
  type MediaQualityConfig,
  type RatingSummary,
} from '@/api/mediaQuality'
import GlassIcon from '@/components/GlassIcon.vue'
import { ElMessage } from 'element-plus'

const { t } = useI18n()
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const cfg = ref<MediaQualityConfig>({
  enabled: false,
  min_score: 6,
  min_votes: 100,
  min_release_days: 7,
  sources: ['tmdb', 'douban', 'imdb', 'bangumi'],
})
const loading = ref(false)
const saving = ref(false)

async function load() {
  loading.value = true
  try {
    const c = await mediaQualityApi.getConfig()
    cfg.value = { ...cfg.value, ...c }
    if (!c.sources || c.sources.length === 0) cfg.value.sources = ['tmdb', 'douban', 'imdb', 'bangumi']
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await mediaQualityApi.setConfig(cfg.value)
    ElMessage.success(t('common.saved'))
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

// —— 评分预览 ——
const qTitle = ref('')
const qYear = ref<number | undefined>(undefined)
const qType = ref('')
const summary = ref<RatingSummary | null>(null)
const previewing = ref(false)

async function preview() {
  if (!qTitle.value.trim()) {
    ElMessage.warning(t('mediaquality.titleRequired'))
    return
  }
  previewing.value = true
  summary.value = null
  try {
    summary.value = await mediaQualityApi.rating(qTitle.value.trim(), qYear.value ?? 0, qType.value)
  } catch (e: any) {
    ElMessage.error(e?.message || t('mediaquality.queryFailed'))
  } finally {
    previewing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title">{{ t('mediaquality.title') }}</h2>
      <h3 v-else class="title embedded-title">{{ t('mediaquality.title') }}</h3>
      <el-button type="primary" :loading="saving" @click="save">{{ t('mediaquality.saveCfg') }}</el-button>
    </div>
    <p class="sub">{{ t('mediaquality.subtitle') }}</p>

    <!-- 配置 -->
    <div v-loading="loading" class="glass card">
      <div class="card-title">{{ t('mediaquality.gateTitle') }}</div>
      <el-form label-width="140px" label-position="right">
        <el-form-item :label="t('mediaquality.f.enableGate')">
          <el-switch v-model="cfg.enabled" />
          <span class="hint">{{ t('mediaquality.f.enableGateHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaquality.f.minScore')">
          <el-input-number v-model="cfg.min_score" :min="0" :max="10" :step="0.1" :precision="1" />
          <span class="hint">{{ t('mediaquality.f.minScoreHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaquality.f.minVotes')">
          <el-input-number v-model="cfg.min_votes" :min="0" :step="50" />
          <span class="hint">{{ t('mediaquality.f.minVotesHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaquality.f.minDays')">
          <el-input-number v-model="cfg.min_release_days" :min="0" :step="1" />
          <span class="hint">{{ t('mediaquality.f.minDaysHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('mediaquality.f.sources')">
          <el-select v-model="cfg.sources" multiple :placeholder="t('mediaquality.f.sourcesHint')" style="width: 100%">
            <el-option v-for="o in sourceOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
      </el-form>
    </div>

    <!-- 评分预览 -->
    <div class="glass card">
      <div class="card-title">{{ t('mediaquality.previewTitle') }}</div>
      <div class="preview-row">
        <el-input
          v-model="qTitle"
          :placeholder="t('mediaquality.f.qTitle')"
          style="max-width: 240px"
          @keyup.enter="preview"
        />
        <el-input-number
          v-model="qYear"
          :min="1900"
          :max="2099"
          :placeholder="t('mediaquality.f.qYear')"
          controls-position="right"
          style="max-width: 130px"
        />
        <el-select v-model="qType" :placeholder="t('mediaquality.f.qType')" style="width: 110px">
          <el-option :label="t('assessment.filter.all')" value="" />
          <el-option :label="t('search.catMovie')" value="movie" />
          <el-option :label="t('search.catTv')" value="tv" />
        </el-select>
        <el-button type="primary" :loading="previewing" @click="preview">{{ t('mediaquality.query') }}</el-button>
      </div>

      <div v-if="summary" class="summary">
        <div class="summary-head">
          <GlassIcon name="mediaquality" :size="34" class="summary-icon" />
          <div>
            <div class="summary-title">
              {{ summary.title }}<span v-if="summary.year" class="muted"> ({{ summary.year }})</span>
            </div>
            <div class="muted small">
              {{ t('mediaquality.aggScore') }} {{ summary.aggregated.toFixed(1) }} ·
              {{ t('mediaquality.votesCount', { n: summary.votes }) }} ·
              {{ t('mediaquality.releasedDays', { n: summary.release_days || '?' }) }}
            </div>
          </div>
          <el-tag :type="summary.passed ? 'success' : 'warning'" class="summary-tag">
            {{ summary.passed ? t('mediaquality.passed') : t('mediaquality.notPassed') }}
          </el-tag>
        </div>
        <div v-if="summary.reason" class="reason">{{ t('mediaquality.reasonPrefix') }}{{ summary.reason }}</div>
        <div class="sources">
          <span v-for="s in summary.sources" :key="s.source" class="source-pill">
            <span class="src-name">{{ sourceLabel(s.source) }}</span>
            <span class="src-score">{{ s.score.toFixed(1) }}</span>
            <span class="src-votes">{{ t('mediaquality.votesUnit', { n: s.votes }) }}</span>
          </span>
          <span v-if="!summary.sources.length" class="muted small">{{ t('mediaquality.noRatingData') }}</span>
        </div>
      </div>
    </div>
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
  justify-content: space-between;
  align-items: center;
}
.title {
  margin: 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.sub {
  margin: -6px 0 0;
  color: var(--text-muted);
  font-size: var(--fs-body);
}
.card {
  padding: 18px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  border-radius: var(--radius-md);
}
.card-title {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.hint {
  margin-left: 10px;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
.preview-row {
  display: flex;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.summary {
  border-top: 1px solid var(--border-soft, rgba(255, 255, 255, 0.08));
  padding-top: 14px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.summary-head {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.summary-icon {
  flex: none;
}
.summary-title {
  font-size: var(--fs-lg);
  font-weight: 700;
}
.summary-tag {
  margin-left: auto;
}
.muted {
  color: var(--text-muted);
}
.small {
  font-size: var(--fs-xs);
}
.reason {
  color: var(--el-color-warning);
  font-size: var(--fs-body);
}
.sources {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
}
.source-pill {
  display: inline-flex;
  align-items: baseline;
  gap: var(--sp-1);
  padding: 4px 10px;
  border-radius: var(--radius-pill);
  background: var(--bg-soft, rgba(255, 255, 255, 0.06));
  font-size: var(--fs-body);
}
.src-name {
  color: var(--text-muted);
}
.src-score {
  font-weight: 700;
}
.src-votes {
  color: var(--text-muted);
  font-size: var(--fs-xs);
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
