<template>
  <div class="ocr-wrap">
    <p class="text-muted">
      配置 OCR 服务后，登录验证码将自动识别并预填（识别错误仍可手改）；未配置时保持人工输入。
      支持自建 PaddleOCR / ddddoc 或任意兼容 HTTP 服务（POST base64 图片返回识别文本）。
    </p>
    <div class="card glass">
      <div class="ch-head"><div class="row-title">OCR 服务</div>
        <el-tag v-if="cfg.enabled" type="success" size="small">已启用</el-tag>
        <el-tag v-else type="info" size="small">未配置（人工输入验证码）</el-tag>
      </div>
      <el-form label-width="90px" class="mt8">
        <el-form-item label="端点 URL">
          <el-input v-model="form.endpoint" placeholder="如 http://192.168.1.10:9876/ocr" />
        </el-form-item>
        <el-form-item label="API Key">
          <el-input v-model="form.api_key" :placeholder="cfg.api_key_set ? '已配置（留空=不修改）' : '可选，服务需要鉴权时填'" />
        </el-form-item>
        <el-form-item label="响应格式">
          <el-select v-model="form.mode">
            <el-option label="PaddleOCR（默认 {results:[[...]]}）" value="paddle" />
            <el-option label="simple（{text}）" value="simple" />
            <el-option label="ddddoc（{data:{data}}）" value="ddddoc" />
          </el-select>
        </el-form-item>
      </el-form>
      <div class="actions">
        <el-button :loading="saving" type="primary" @click="save">保存</el-button>
        <el-button :loading="testing" @click="test">测试连通</el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { http } from '@/api/http'

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
  } catch {
    // 未装配（旧后端）时静默：面板显示未配置态。
  }
}

async function save() {
  saving.value = true
  try {
    const c = await http.put<OcrConfig>('/system/ocr', form.value).then((r) => r.data)
    cfg.value = c
    form.value.api_key = ''
    ElMessage.success('OCR 配置已保存')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
  } finally {
    saving.value = false
  }
}

async function test() {
  testing.value = true
  try {
    const r = await http.post<{ ok: boolean; error?: string }>('/system/ocr/test').then((x) => x.data)
    if (r.ok) ElMessage.success('OCR 服务连通正常')
    else ElMessage.error(r.error || '测试失败')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '测试失败')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.ocr-wrap { display: flex; flex-direction: column; gap: 12px; }
.ch-head { display: flex; align-items: center; justify-content: space-between; }
.row-title { font-weight: 700; }
.mt8 { margin-top: 8px; }
.actions { display: flex; gap: 8px; padding: 4px 0 8px 8px; }
</style>
