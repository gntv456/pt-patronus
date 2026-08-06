<script setup lang="ts">
// Telegram Bot 配置面板（含 Cookie 保活频率，复用调度表单）。
// 状态与逻辑来自 useTgBot + useSchedule 组合式函数。
import { useTgBot } from '@/composables/useTgBot'
import { useSchedule } from '@/composables/useSchedule'

const { tgCfg, tgForm, tgLoading, tgSaving, saveTgBot } = useTgBot()
const { schedCfg, schedForm, keepalivePresets } = useSchedule()
</script>

<template>
        <div v-loading="tgLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            启用入站 bot 后，可在 Telegram 用自然语言驱动 PTPatronus（搜影视 / 订阅 / 整理…）。命令：<code>/help</code>、<code>/clear</code>。改动需重启进程生效。
          </p>
          <div class="card glass">
            <div class="ch-head"><div class="row-title">启用 Telegram Bot</div><el-switch v-model="tgForm.enabled" /></div>
            <el-input v-model="tgForm.bot_token" type="password" show-password class="mt8" :placeholder="tgCfg?.bot_token_configured ? '已配置（留空=不修改）' : 'BotFather token'"><template #prepend>Bot Token</template></el-input>
            <el-input v-model="tgForm.allowed_ids" class="mt8" placeholder="允许的用户 ID，逗号分隔；留空=不限"><template #prepend>白名单</template></el-input>
            <el-input v-model="tgForm.api_url" class="mt8" placeholder="自定义 Bot API 域名（国内反代）；留空=官方"><template #prepend>API URL</template></el-input>
            <div class="ch-head mt8"><div class="row-title">允许群聊响应</div><el-switch v-model="tgForm.allow_group_chat" /></div>
          </div>
          
          <div class="card glass mt8">
            <div class="ch-head"><div class="row-title">Cookie 保活频率</div></div>
            <p class="text-muted" style="font-size:12px;line-height:1.6;margin:0 0 8px">
              独立于聚合快照：对启用站做认证 GET，刷新短寿 session 并回写轮换 cookie。建议不超过 1 小时。
            </p>
            <el-select
              v-model="schedForm.keepalive_cron"
              filterable
              allow-create
              default-first-option
              class="mt8"
              style="width:100%"
              placeholder="选择或输入 cron"
            >
              <el-option v-for="p in keepalivePresets" :key="p.value" :label="p.label" :value="p.value" />
            </el-select>
            <el-input v-model="schedForm.keepalive_cron" class="mt8" placeholder="13 * * * *">
              <template #prepend>cron</template>
            </el-input>
            <p v-if="schedCfg" class="text-muted mt8" style="font-size:12px">
              内置默认：{{ schedCfg.keepalive_default }}
            </p>
          </div>
<div class="oidc-save"><el-button type="primary" :loading="tgSaving" @click="saveTgBot">保存</el-button></div>
        </div>
</template>

<style scoped>
.oidc-wrap { display: flex; flex-direction: column; gap: 14px; max-width: 640px; }
.oidc-help { font-size: 13px; line-height: 1.6; }
.oidc-save { display: flex; justify-content: flex-end; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
