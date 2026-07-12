<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  mediaQualityApi,
  sourceOptions,
  sourceLabel,
  type MediaQualityConfig,
  type RatingSummary,
} from '@/api/mediaQuality'
import GlassIcon from '@/components/GlassIcon.vue'
import { ElMessage } from 'element-plus'

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
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await mediaQualityApi.setConfig(cfg.value)
    ElMessage.success('已保存')
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
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
    ElMessage.warning('请输入标题')
    return
  }
  previewing.value = true
  summary.value = null
  try {
    summary.value = await mediaQualityApi.rating(qTitle.value.trim(), qYear.value ?? 0, qType.value)
  } catch (e: any) {
    ElMessage.error(e?.message || '查询失败')
  } finally {
    previewing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">媒体质量</h2>
      <el-button type="primary" :loading="saving" @click="save">保存配置</el-button>
    </div>
    <p class="sub">聚合豆瓣 / TMDB / IMDb / Bangumi 多源评分，对订阅做低分保护：评分、投票数、上映天数三道闸门任一不达标即跳过推送。评分查询失败 fail-open（不阻断下载）。</p>

    <!-- 配置 -->
    <div v-loading="loading" class="glass card">
      <div class="card-title">质量闸门</div>
      <el-form label-width="140px" label-position="right">
        <el-form-item label="启用订阅闸门">
          <el-switch v-model="cfg.enabled" />
          <span class="hint">开启后所有订阅命中前查评分；可在各订阅单独开关覆盖</span>
        </el-form-item>
        <el-form-item label="最低综合评分">
          <el-input-number v-model="cfg.min_score" :min="0" :max="10" :step="0.1" :precision="1" />
          <span class="hint">低于此分不下载（默认 6.0）</span>
        </el-form-item>
        <el-form-item label="最低投票数">
          <el-input-number v-model="cfg.min_votes" :min="0" :step="50" />
          <span class="hint">样本无统计意义则不放行（默认 100）</span>
        </el-form-item>
        <el-form-item label="上映满 N 天">
          <el-input-number v-model="cfg.min_release_days" :min="0" :step="1" />
          <span class="hint">评分稳定前的作品暂缓（默认 7；0=不校验）</span>
        </el-form-item>
        <el-form-item label="参与聚合源">
          <el-select v-model="cfg.sources" multiple placeholder="空=全部源" style="width: 100%">
            <el-option v-for="o in sourceOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
      </el-form>
    </div>

    <!-- 评分预览 -->
    <div class="glass card">
      <div class="card-title">评分预览</div>
      <div class="preview-row">
        <el-input v-model="qTitle" placeholder="影视标题" style="max-width: 240px" @keyup.enter="preview" />
        <el-input-number v-model="qYear" :min="1900" :max="2099" placeholder="年份" controls-position="right" style="max-width: 130px" />
        <el-select v-model="qType" placeholder="类型" style="width: 110px">
          <el-option label="全部" value="" />
          <el-option label="电影" value="movie" />
          <el-option label="剧集" value="tv" />
        </el-select>
        <el-button type="primary" :loading="previewing" @click="preview">查询</el-button>
      </div>

      <div v-if="summary" class="summary">
        <div class="summary-head">
          <GlassIcon name="mediaquality" :size="34" class="summary-icon" />
          <div>
            <div class="summary-title">{{ summary.title }}<span v-if="summary.year" class="muted"> ({{ summary.year }})</span></div>
            <div class="muted small">综合 {{ summary.aggregated.toFixed(1) }} · 共 {{ summary.votes }} 票 · 上映 {{ summary.release_days || '?' }} 天</div>
          </div>
          <el-tag :type="summary.passed ? 'success' : 'warning'" class="summary-tag">
            {{ summary.passed ? '通过' : '未通过' }}
          </el-tag>
        </div>
        <div v-if="summary.reason" class="reason">原因：{{ summary.reason }}</div>
        <div class="sources">
          <span v-for="s in summary.sources" :key="s.source" class="source-pill">
            <span class="src-name">{{ sourceLabel(s.source) }}</span>
            <span class="src-score">{{ s.score.toFixed(1) }}</span>
            <span class="src-votes">{{ s.votes }} 票</span>
          </span>
          <span v-if="!summary.sources.length" class="muted small">未找到任何评分数据</span>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; justify-content: space-between; align-items: center; }
.title { margin: 0; font-size: 22px; font-weight: 700; }
.sub { margin: -6px 0 0; color: var(--text-muted); font-size: 13px; }
.card { padding: 18px; display: flex; flex-direction: column; gap: 14px; border-radius: var(--radius-md); }
.card-title { font-size: 15px; font-weight: 700; }
.hint { margin-left: 10px; color: var(--text-muted); font-size: 12px; }
.preview-row { display: flex; gap: 10px; flex-wrap: wrap; }
.summary { border-top: 1px solid var(--border-soft, rgba(255,255,255,0.08)); padding-top: 14px; display: flex; flex-direction: column; gap: 10px; }
.summary-head { display: flex; align-items: center; gap: 12px; }
.summary-icon { flex: none; }
.summary-title { font-size: 16px; font-weight: 700; }
.summary-tag { margin-left: auto; }
.muted { color: var(--text-muted); }
.small { font-size: 12px; }
.reason { color: var(--el-color-warning); font-size: 13px; }
.sources { display: flex; flex-wrap: wrap; gap: 8px; }
.source-pill { display: inline-flex; align-items: baseline; gap: 6px; padding: 4px 10px; border-radius: 999px; background: var(--bg-soft, rgba(255,255,255,0.06)); font-size: 13px; }
.src-name { color: var(--text-muted); }
.src-score { font-weight: 700; }
.src-votes { color: var(--text-muted); font-size: 12px; }
</style>
