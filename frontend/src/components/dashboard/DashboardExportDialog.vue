<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 仪表盘导出图片配置弹窗：主题 / 背景图(压缩后存 localStorage) / 总览字段开关 / 冠军7类选择
// / 每站明细开关+站点白名单+数量 / 脱敏。配置持久化，点「导出图片」按当前配置绘制下载。
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
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

const RANK_LABELS = computed<Record<RankMetric, string>>(() => ({
  uploaded: t('sitedetail.uploaded'),
  downloaded: t('sitedetail.downloaded'),
  uploads: t('smt.uploads'),
  seeding: t('sitedetail.seeding'),
  ratio: t('sitedetail.ratio'),
  bonus: t('sitedetail.bonus'),
  bonus_per_hour: t('smt.bonusH'),
}))

const FIELD_DEFS = computed<{ key: keyof DashboardExportConfig['fields']; label: string }[]>(() => [
  { key: 'uploaded', label: t('ded.f.totalUploaded') },
  { key: 'downloaded', label: t('ded.f.totalDownloaded') },
  { key: 'seeding', label: t('ded.f.totalSeeding') },
  { key: 'seedingSize', label: t('ded.f.seedingSize') },
  { key: 'bonus', label: t('ded.f.totalBonus') },
  { key: 'ratio', label: t('ded.f.totalRatio') },
  { key: 'uploadCount', label: t('ded.f.totalUploads') },
  { key: 'bonusPerHour', label: t('ded.f.totalBonusH') },
  { key: 'earliestJoin', label: t('ded.f.earliestJoin') },
  { key: 'ptAge', label: t('ded.f.ptAge') },
])

const siteOptions = computed(() => (props.data?.sites ?? []).map((s) => ({ value: s.site_id, label: s.site_name })))

function persist(): void {
  if (!saveConfig(config.value)) ElMessage.warning(t('ded.saveCfgFailed'))
}

watch(config, () => persist(), { deep: true })

function resetConfig(): void {
  config.value = {
    ...defaultConfig,
    fields: { ...defaultConfig.fields },
    rankMetrics: [...defaultConfig.rankMetrics],
    selectedSiteIds: [],
  }
  ElMessage.success(t('ded.resetOk'))
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
    ElMessage.warning(t('ded.bgReadFailed'))
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
    ElMessage.warning(t('ded.exportFailed'))
  } finally {
    exporting.value = false
  }
}
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="t('ded.title')"
    width="560px"
    append-to-body
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="export-cfg" v-loading="exporting">
      <!-- 主题 -->
      <div class="sec">
        <div class="sec-t">{{ t('ded.theme') }}</div>
        <el-radio-group v-model="config.theme">
          <el-radio value="dark" label="dark">{{ t('ded.themeDark') }}</el-radio>
          <el-radio value="light" label="light">{{ t('ded.themeLight') }}</el-radio>
          <el-radio value="glass" label="glass">{{ t('ded.themeGlass') }}</el-radio>
        </el-radio-group>
        <div class="hint text-muted">{{ t('ded.glassHint') }}</div>
      </div>

      <!-- 背景图 -->
      <div class="sec">
        <div class="sec-t">{{ t('ded.bg') }}</div>
        <div class="bg-row">
          <input ref="fileInput" type="file" accept="image/*" hidden @change="onPickBg" />
          <el-button size="small" @click="fileInput?.click()">{{ t('ded.pickImage') }}</el-button>
          <el-button v-if="config.backgroundImage" size="small" @click="clearBg">{{ t('ded.clear') }}</el-button>
          <img v-if="config.backgroundImage" :src="config.backgroundImage" class="bg-preview" :alt="t('ded.bgAlt')" />
        </div>
        <div v-if="config.backgroundImage" class="sliders">
          <div class="slider-row">
            <span>{{ t('ded.blur') }}</span
            ><b>{{ config.backgroundBlur }}px</b>
            <el-slider v-model="config.backgroundBlur" :min="0" :max="20" :step="1" />
          </div>
          <div class="slider-row">
            <span>{{ t('gp.mask') }}</span
            ><b>{{ Math.round(config.backgroundDim * 100) }}%</b>
            <el-slider v-model="config.backgroundDim" :min="0" :max="0.8" :step="0.05" />
          </div>
        </div>
      </div>

      <!-- 总览字段 -->
      <div class="sec">
        <div class="sec-t">{{ t('ded.fields') }}</div>
        <div class="fields-grid">
          <el-checkbox v-for="f in FIELD_DEFS" :key="f.key" v-model="config.fields[f.key]">{{ f.label }}</el-checkbox>
        </div>
      </div>

      <!-- 冠军排行 -->
      <div class="sec">
        <div class="sec-t">
          <span>{{ t('ded.champions') }}</span>
          <el-switch v-model="config.showRank" size="small" />
        </div>
        <el-checkbox-group v-model="config.rankMetrics" :disabled="!config.showRank">
          <el-checkbox v-for="m in ALL_RANK_METRICS" :key="m" :value="m" :label="m">{{
            t('ded.champOf', { metric: RANK_LABELS[m] })
          }}</el-checkbox>
        </el-checkbox-group>
      </div>

      <!-- 每站明细 -->
      <div class="sec">
        <div class="sec-t">
          <span>{{ t('ded.perSite') }}</span>
          <el-switch v-model="config.showSites" size="small" />
        </div>
        <div class="site-row">
          <el-select
            v-model="config.selectedSiteIds"
            multiple
            collapse-tags
            collapse-tags-tooltip
            :placeholder="t('ded.allSites')"
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
          <span class="text-muted num-hint">{{ t('ded.countHint') }}</span>
        </div>
        <el-checkbox v-model="config.showIdentity" :disabled="!config.showSites">{{
          t('ded.showIdentity')
        }}</el-checkbox>
      </div>

      <!-- 脱敏 -->
      <div class="sec">
        <div class="sec-t">{{ t('ded.masking') }}</div>
        <div class="switch-row">
          <el-checkbox v-model="config.blurSite">{{ t('ded.blurSite') }}</el-checkbox>
          <el-checkbox v-model="config.blurUser">{{ t('ded.blurUser') }}</el-checkbox>
        </div>
      </div>
    </div>

    <template #footer>
      <el-button @click="resetConfig">{{ t('ded.reset') }}</el-button>
      <el-button type="primary" :loading="exporting" :disabled="!data" @click="doExport">{{
        t('ded.exportGo')
      }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.export-cfg {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.sec {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.sec-t {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-weight: 600;
  font-size: var(--fs-md);
}
.hint {
  font-size: var(--fs-xs);
}
.bg-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.bg-preview {
  width: 56px;
  height: 36px;
  object-fit: cover;
  border-radius: var(--radius-xs);
  border: 1px solid rgba(255, 255, 255, 0.12);
}
.sliders {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  margin-top: 4px;
}
.slider-row {
  display: grid;
  grid-template-columns: 40px 48px 1fr;
  align-items: center;
  gap: var(--sp-2);
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.slider-row b {
  color: #c7ccdb;
  font-weight: 600;
}
.fields-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: var(--sp-1) var(--sp-3);
}
.site-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.num-hint {
  font-size: var(--fs-xs);
  white-space: nowrap;
}
.switch-row {
  display: flex;
  gap: var(--sp-4);
}
</style>
