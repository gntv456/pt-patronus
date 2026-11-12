// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// LLM 网关（把 AI Agent 以 OpenAI/Anthropic 兼容协议对外暴露）。
// 从 Settings.vue 抽出：管理网关开关、访问令牌、允许模型与并发。
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage } from 'element-plus'
import { llmGatewayApi, type LLMGatewayConfig } from '@/api/llmGateway'

const gwCfg = ref<LLMGatewayConfig | null>(null)
const gwForm = ref<{ enabled: boolean; access_token: string; allowed_models: string; max_concurrency: number }>({
  enabled: false,
  access_token: '',
  allowed_models: '',
  max_concurrency: 4,
})
const gwLoading = ref(false)
const gwSaving = ref(false)
const gwBaseURL = `${window.location.origin}/api/v1/openai/v1`

async function loadLLMGateway() {
  gwLoading.value = true
  try {
    gwCfg.value = await llmGatewayApi.getConfig()
    gwForm.value = {
      enabled: gwCfg.value.enabled,
      access_token: '',
      allowed_models: (gwCfg.value.allowed_models ?? []).join(','),
      max_concurrency: gwCfg.value.max_concurrency || 4,
    }
  } catch {
    ElMessage.error(t('opusellmgateway.k1'))
  } finally {
    gwLoading.value = false
  }
}

async function saveLLMGateway() {
  gwSaving.value = true
  try {
    const models = gwForm.value.allowed_models
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean)
    await llmGatewayApi.setConfig({
      enabled: gwForm.value.enabled,
      access_token: gwForm.value.access_token,
      allowed_models: models,
      max_concurrency: gwForm.value.max_concurrency,
    })
    ElMessage.success(t('common.saved'))
    await loadLLMGateway()
  } catch {
    ElMessage.error(t('opusellmgateway.k2'))
  } finally {
    gwSaving.value = false
  }
}


const t = (k: string) => i18n.global.t(k)

export function useLlmGateway() {
  return { gwCfg, gwForm, gwLoading, gwSaving, gwBaseURL, loadLLMGateway, saveLLMGateway }
}
