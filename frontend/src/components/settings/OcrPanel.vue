<template>
  <div class="ocr-wrap">
    <p class="text-muted">
      {{ t('ocrp.intro1') }}
      {{ t('ocrp.intro2') }}
    </p>
    <div class="card glass">
      <div class="ch-head">
        <div class="row-title">{{ t('ocrp.service') }}</div>
        <el-tag v-if="cfg.enabled" type="success" size="small">{{ t('common.enabled') }}</el-tag>
        <el-tag v-else type="info" size="small">{{ t('ocrp.notConfigured') }}</el-tag>
      </div>
      <el-form label-width="90px" class="mt8">
        <el-form-item :label="t('ocrp.endpoint')">
          <el-input v-model="form.endpoint" :placeholder="t('ocrp.endpointPh')" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.api_key" :placeholder="cfg.api_key_set ? t('ocrp.keySet') : t('ocrp.keyPh')" />
        </el-form-item>
        <el-form-item :label="t('ocrp.fmt')">
          <el-select v-model="form.mode">
            <el-option :label="t('ocrp.fmtPaddle')" value="paddle" />
            <el-option label="simple（{text}）" value="simple" />
            <el-option label="ddddoc（{data:{data}}）" value="ddddoc" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="actions">
        <el-button :loading="saving" type="primary" @click="save">{{ t('common.save') }}</el-button>
        <el-button :loading="testing" @click="test">{{ t('ocrp.testConn') }}</el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { http } from '@/api/http'

const { t } = useI18n()

interface OcrConfig {
  endpoint: string
  api_key: string
  api_key_set: boolean
  mode: string
  enabled: boolean
}

const cfg = ref<OcrConfig>({ endpoint: '', api_key: '', api_key_set: false, mode: 'paddle', enabled: false })
const form = ref({ endpoint: '', api_key: '', mode: 'paddle' })
const saving = ref(false)
const testing = ref(false)

async function load() {
  try {
    const c = await http.get<OcrConfig>('/system/ocr').then((r) => r.data)
    cfg.value = c
    form.value = { endpoint: c.endpoint, api_key: '', mode: c.mode || 'paddle' }
  } catch (e: unknown) {
    // 后端这个 GET 现在恒回 200（没配也给你一份空配置），所以走到这里只剩三种可能：
    // 登录失效/无权限、网络断了、或极旧版本没这条路由。
    // 静默回落到默认值 = 把这三件都说成「您还没配置 OCR」，用户会去重填一遍（#298）。
    ElMessage.warning(t('ocrp.loadFailed', { reason: (e as Error).message || '?' }))
  }
}

async function save() {
  saving.value = true
  try {
    const c = await http.put<OcrConfig>('/system/ocr', form.value).then((r) => r.data)
    cfg.value = c
    form.value.api_key = ''
    ElMessage.success(t('ocrp.saved'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.saveFailed'))
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  try {
    const r = await http.post<{ ok: boolean; error?: string }>('/system/ocr/test').then((x) => x.data)
    if (r.ok) ElMessage.success(t('ocrp.connOk'))
    else ElMessage.error(r.error || t('ocrp.testFailed'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('ocrp.testFailed'))
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.ocr-wrap {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.ch-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.row-title {
  font-weight: 700;
}
.mt8 {
  margin-top: 8px;
}
.actions {
  display: flex;
  gap: 8px;
  padding: 4px 0 8px 8px;
}
</style>
