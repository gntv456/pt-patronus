<script setup lang="ts">
// 仪表盘导出图片配置弹窗：主题 / 背景图(压缩后存 localStorage) / 总览字段开关 / 冠军7类选择
// / 每站明细开关+站点白名单+数量 / 脱敏。配置持久化，点「导出图片」按当前配置绘制下载。
import { ref, watch, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { exportDashboardImage, type DashboardShareData } from '@/utils/dashboardImage'
import {
  loadConfig,
  saveConfig,
  defaultConfig,
  ALL_RANK_METRICS,
  type DashboardExportConfig,
  type RankMetric,
} from '@/utils/dashboardExportConfig'

const props = defineProps<{ modelValue: boolean; data: DashboardShareData | null }>()
const emit = defineEmits<{ 'update:modelValue': [boolean] }>()

const config = ref<DashboardExportConfig>(loadConfig())
const exporting = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const RANK_LABELS: Record<RankMetric, string> = {
  uploaded: '上传',
  downloaded: '下载',
  uploads: '发布',
  seeding: '做种',
  ratio: '分享率',
  bonus: '魔力',
  bonus_per_hour: '时魔/h',
}

const FIELD_DEFS: { key: keyof DashboardExportConfig['fields']; label: string }[] = [
  { key: 'uploaded', label: '总上传' },
  { key: 'downloaded', label: '总下载' },
  { key: 'seeding', label: '总做种' },
  { key: 'seedingSize', label: '做种体积' },
  { key: 'bonus', label: '总魔力' },
  { key: 'ratio', label: '总分享率' },
  { key: 'uploadCount', label: '总发布数' },
  { key: 'bonusPerHour', label: '总时魔/h' },
  { key: 'earliestJoin', label: '最早入站' },
  { key: 'ptAge', label: 'PT 龄' },
]

const siteOptions = computed(() =>
  (props.data?.sites ?? []).map((s) => ({ value: s.site_id, label: s.site_name })),
)

function persist(): void {
  if (!saveConfig(config.value)) ElMessage.warning('配置保存失败：背景图可能过大，本次配置仅临时生效')
}

watch(config, () => persist(), { deep: true })

function resetConfig(): void {
  config.value = {
    ...defaultConfig,
    fields: { ...defaultConfig.fields },
    rankMetrics: [...defaultConfig.rankMetrics],
    selectedSiteIds: [],
  }
  ElMessage.success('已恢复默认配置')
}

function loadImage(dataUrl: string): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.onload = () => resolve(img)
    img.onerror = reject
    img.src = dataUrl
  })
}

// 压缩背景图（max 宽 1600 / jpeg 0.85），控制 localStorage 体积（~5MB 限额）。
async function compressImage(file: File): Promise<string> {
  const dataUrl = await new Promise<string>((res, rej) => {
    const r = new FileReader()
    r.onload = () => res(r.result as string)
    r.onerror = rej
    r.readAsDataURL(file)
  })
  const img = await loadImage(dataUrl)
  const maxW = 1600
  const sc = img.width > maxW ? maxW / img.width : 1
  const w = Math.round(img.width * sc)
  const h = Math.round(img.height * sc)
  const cv = document.createElement('canvas')
  cv.width = w
  cv.height = h
  const cx = cv.getContext('2d')
  if (!cx) return dataUrl
  cx.drawImage(img, 0, 0, w, h)
  return cv.toDataURL('image/jpeg', 0.85)
}

async function onPickBg(e: Event): Promise<void> {
  const f = (e.target as HTMLInputElement).files?.[0]
  if (!f) return
  try {
    config.value.backgroundImage = await compressImage(f)
  } catch {
    ElMessage.warning('背景图读取失败')
  }
  ;(e.target as HTMLInputElement).value = ''
}

function clearBg(): void {
  config.value.backgroundImage = ''
}

async function doExport(): Promise<void> {
  if (!props.data) return
  exporting.value = true
  try {
    await exportDashboardImage(props.data, config.value)
    emit('update:modelValue', false)
  } catch {
    ElMessage.warning('导出失败')
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    title="导出仪表盘图片"
    width="560px"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="export-cfg" v-loading="exporting">
      <!-- 主题 -->
      <div class="sec">
        <div class="sec-t">主题</div>
        <el-radio-group v-model="config.theme">
          <el-radio value="dark" label="dark">深色</el-radio>
          <el-radio value="light" label="light">浅色</el-radio>
          <el-radio value="glass" label="glass">玻璃</el-radio>
        </el-radio-group>
        <div class="hint text-muted">「玻璃」建议搭配背景图；有背景图时自动白字 + 暗遮罩。</div>
      </div>

      <!-- 背景图 -->
      <div class="sec">
        <div class="sec-t">背景图</div>
        <div class="bg-row">
          <input ref="fileInput" type="file" accept="image/*" hidden @change="onPickBg" />
          <el-button size="small" @click="fileInput?.click()">选择图片</el-button>
          <el-button v-if="config.backgroundImage" size="small" @click="clearBg">清除</el-button>
          <img v-if="config.backgroundImage" :src="config.backgroundImage" class="bg-preview" alt="背景预览" />
        </div>
        <div v-if="config.backgroundImage" class="sliders">
          <div class="slider-row">
            <span>模糊</span><b>{{ config.backgroundBlur }}px</b>
            <el-slider v-model="config.backgroundBlur" :min="0" :max="20" :step="1" />
          </div>
          <div class="slider-row">
            <span>遮罩</span><b>{{ Math.round(config.backgroundDim * 100) }}%</b>
            <el-slider v-model="config.backgroundDim" :min="0" :max="0.8" :step="0.05" />
          </div>
        </div>
      </div>

      <!-- 总览字段 -->
      <div class="sec">
        <div class="sec-t">总览字段</div>
        <div class="fields-grid">
          <el-checkbox v-for="f in FIELD_DEFS" :key="f.key" v-model="config.fields[f.key]">{{ f.label }}</el-checkbox>
        </div>
      </div>

      <!-- 冠军排行 -->
      <div class="sec">
        <div class="sec-t">
          <span>各项冠军</span>
          <el-switch v-model="config.showRank" size="small" />
        </div>
        <el-checkbox-group v-model="config.rankMetrics" :disabled="!config.showRank">
          <el-checkbox v-for="m in ALL_RANK_METRICS" :key="m" :value="m" :label="m">{{ RANK_LABELS[m] }}冠军</el-checkbox>
        </el-checkbox-group>
      </div>

      <!-- 每站明细 -->
      <div class="sec">
        <div class="sec-t">
          <span>每站明细</span>
          <el-switch v-model="config.showSites" size="small" />
        </div>
        <div class="site-row">
          <el-select
            v-model="config.selectedSiteIds"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="全部站点"
            :disabled="!config.showSites"
            style="flex: 1"
          >
            <el-option v-for="o in siteOptions" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
          <el-input-number
            v-model="config.siteLimit"
            :min="0"
            :disabled="!config.showSites"
            size="small"
            controls-position="right"
          />
          <span class="text-muted num-hint">数量(0=全部)</span>
        </div>
        <el-checkbox v-model="config.showIdentity" :disabled="!config.showSites">显示用户名 / ID / 邮箱</el-checkbox>
      </div>

      <!-- 脱敏 -->
      <div class="sec">
        <div class="sec-t">脱敏</div>
        <div class="switch-row">
          <el-checkbox v-model="config.blurSite">站点名脱敏</el-checkbox>
          <el-checkbox v-model="config.blurUser">用户名脱敏</el-checkbox>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="resetConfig">恢复默认</el-button>
      <el-button type="primary" :loading="exporting" :disabled="!data" @click="doExport">导出图片</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.export-cfg { display: flex; flex-direction: column; gap: 16px; }
.sec { display: flex; flex-direction: column; gap: 8px; }
.sec-t { display: flex; align-items: center; justify-content: space-between; font-weight: 600; font-size: 14px; }
.hint { font-size: 12px; }
.bg-row { display: flex; align-items: center; gap: 8px; }
.bg-preview { width: 56px; height: 36px; object-fit: cover; border-radius: 6px; border: 1px solid rgba(255, 255, 255, 0.12); }
.sliders { display: flex; flex-direction: column; gap: 4px; margin-top: 4px; }
.slider-row { display: grid; grid-template-columns: 40px 48px 1fr; align-items: center; gap: 8px; font-size: 12px; color: var(--text-muted); }
.slider-row b { color: #c7ccdb; font-weight: 600; }
.fields-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 4px 12px; }
.site-row { display: flex; align-items: center; gap: 8px; }
.num-hint { font-size: 12px; white-space: nowrap; }
.switch-row { display: flex; gap: 18px; }
</style>
