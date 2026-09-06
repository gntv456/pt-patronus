<script setup lang="ts">
// PT Patronus 守护神名片：把各站聚合数据推送到好学（ptlife.php 展示）。
// 设置读写 /hxcard/settings，手动推送 /hxcard/push，预览 /hxcard/preview。
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { useI18n } from 'vue-i18n'
import {
  getHxCardSettings,
  setHxCardSettings,
  pushHxCard,
  previewHxCard,
  type HxCardSettings,
  type HxCardPreview,
} from '@/api/hxcard'

const { t } = useI18n()

const cfg = ref<HxCardSettings>({ enabled: false, endpoint: '', secret: '' })
const secretInput = ref('')
const loading = ref(false)
const saving = ref(false)
const pushing = ref(false)
const preview = ref<HxCardPreview | null>(null)

function fmtBytes(b: number): string {
  if (!b) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = b
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(2)} ${units[i]}`
}

async function load() {
  loading.value = true
  try {
    const res = await getHxCardSettings()
    cfg.value = res.data?.data ?? (res.data as unknown as HxCardSettings)
    secretInput.value = ''
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const res = await setHxCardSettings({
      enabled: cfg.value.enabled,
      endpoint: cfg.value.endpoint,
      secret: secretInput.value, // 空=保持不变；"-"=清除
    })
    cfg.value = res.data?.data ?? (res.data as unknown as HxCardSettings)
    secretInput.value = ''
    ElMessage.success(t('common.saved'))
  } finally {
    saving.value = false
  }
}

async function pushNow() {
  pushing.value = true
  try {
    const res = await pushHxCard()
    const payload = res.data as { success?: boolean; message?: string }
    if (payload?.success === false) {
      ElMessage.error(payload.message || t('hxcard.pushFail'))
    } else {
      ElMessage.success(t('hxcard.pushOk'))
    }
  } finally {
    pushing.value = false
  }
}

async function loadPreview() {
  const res = await previewHxCard()
  preview.value = res.data?.data ?? (res.data as unknown as HxCardPreview)
}

onMounted(load)
</script>

<template>
  <div v-loading="loading" class="oidc-wrap">
    <p class="text-muted oidc-help">{{ t('hxcard.help') }}</p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('hxcard.enable') }}</div>
        <el-switch v-model="cfg.enabled" />
      </div>
      <el-input v-model="cfg.endpoint" class="mt8" placeholder="https://www.hxpt.org/api/ptlife/receive.php">
        <template #prepend>{{ t('hxcard.endpoint') }}</template>
      </el-input>
      <el-input
        v-model="secretInput"
        type="password"
        show-password
        class="mt8"
        :placeholder="cfg.secret ? t('hxcard.secretConfigured') : t('hxcard.secretPlaceholder')"
      >
        <template #prepend>{{ t('hxcard.secret') }}</template>
      </el-input>
      <p class="text-muted" style="margin: 6px 0 0; font-size: var(--fs-xs)">
        {{ t('hxcard.secretHint') }}
      </p>
    </div>

    <div v-if="preview" class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('hxcard.previewTitle') }}</div>
        <span class="text-muted" style="font-size: var(--fs-xs)">
          {{ t('hxcard.siteCount', { n: preview.site_count }) }}
        </span>
      </div>
      <div class="prev-row"><span>{{ t('hxcard.totalUp') }}</span><b>{{ fmtBytes(preview.totals.uploaded) }}</b></div>
      <div class="prev-row"><span>{{ t('hxcard.totalDown') }}</span><b>{{ fmtBytes(preview.totals.downloaded) }}</b></div>
      <div class="prev-row"><span>{{ t('hxcard.totalSeed') }}</span><b>{{ fmtBytes(preview.totals.seeding_size) }}</b></div>
      <div class="prev-row"><span>{{ t('hxcard.totalBonus') }}</span><b>{{ preview.totals.bonus.toLocaleString() }}</b></div>
    </div>

    <div class="oidc-save">
      <el-button @click="loadPreview">{{ t('hxcard.preview') }}</el-button>
      <el-button :loading="pushing" @click="pushNow">{{ t('hxcard.pushNow') }}</el-button>
      <el-button type="primary" :loading="saving" @click="save">{{ t('common.save') }}</el-button>
    </div>
  </div>
</template>

<style scoped>
.oidc-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
  max-width: 640px;
}
.oidc-help {
  font-size: var(--fs-body);
  line-height: 1.6;
}
.oidc-save {
  display: flex;
  justify-content: flex-end;
}
.ch-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.mt8 {
  margin-top: 8px;
}
.card {
  padding: 16px 18px;
  margin-bottom: 12px;
}
.prev-row {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  font-size: var(--fs-sm);
}
</style>
