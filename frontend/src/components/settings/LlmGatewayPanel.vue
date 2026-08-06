<script setup lang="ts">
// AI 网关配置面板。
// 状态与逻辑来自 useLlmGateway 组合式函数。
import { useLlmGateway } from '@/composables/useLlmGateway'

const { gwCfg, gwForm, gwLoading, gwSaving, gwBaseURL, saveLLMGateway } = useLlmGateway()
</script>

<template>
        <div v-loading="gwLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            启用后，外部客户端（Cursor / Claude Desktop / continue.dev 等）可经 OpenAI / Anthropic 兼容协议驱动 PTPatronus 的 AI 助手与工具。
            OpenAI Base URL：<code>{{ gwBaseURL }}</code>，model 填 <code>ptpatronus-agent</code>。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">启用 AI 网关</div>
              <el-switch v-model="gwForm.enabled" />
            </div>
            <el-input
              v-model="gwForm.access_token"
              type="password"
              show-password
              class="mt8"
              :placeholder="gwCfg?.access_token_configured ? '已配置（留空=不修改）' : '访问令牌'"
            >
              <template #prepend>访问令牌</template>
            </el-input>
            <el-input v-model="gwForm.allowed_models" class="mt8" placeholder="留空=接受任意 model；逗号分隔">
              <template #prepend>允许的 Model</template>
            </el-input>
            <el-input-number v-model="gwForm.max_concurrency" :min="0" :max="64" class="mt8" />
          </div>
          <div class="oidc-save">
            <el-button type="primary" :loading="gwSaving" @click="saveLLMGateway">保存</el-button>
          </div>
        </div>
</template>

<style scoped>
.oidc-wrap { display: flex; flex-direction: column; gap: 14px; max-width: 640px; }
.oidc-help { font-size: 13px; line-height: 1.6; }
.oidc-help code { background: var(--card); padding: 2px 6px; border-radius: 4px; }
.oidc-save { display: flex; justify-content: flex-end; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
