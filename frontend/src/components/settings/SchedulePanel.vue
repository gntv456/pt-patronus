<script setup lang="ts">
// 聚合快照调度配置面板。
// 状态与逻辑来自 useSchedule 组合式函数。
import { useSchedule } from '@/composables/useSchedule'

const { schedCfg, schedForm, schedLoading, schedSaving, schedPresets, saveSchedule } = useSchedule()
</script>

<template>
        <div v-loading="schedLoading" class="oidc-wrap">
          <p class="text-muted oidc-help">
            聚合站点快照：合并 <b>统计 / 站免 / 公告 / 消息 / Cookie 保活 / 考核</b> 为一次串行抓取（H&amp;R 与小时级 cookie-keepalive 仍独立调度）。
            间隔越大越隐蔽（对站点越友好，不易触发风控），但仪表盘数据与 H&amp;R/站免/新邮件告警延迟越高。
          </p>
          <div class="card glass">
            <div class="ch-head">
              <div class="row-title">聚合快照频率</div>
              <el-select
                v-model="schedForm.snapshot_cron"
                placeholder="选择预设或自定义"
                style="width: 240px"
                filterable
                allow-create
              >
                <el-option v-for="p in schedPresets" :key="p.value" :label="p.label" :value="p.value" />
              </el-select>
            </div>
            <el-input v-model="schedForm.snapshot_cron" class="mt8" placeholder="7 */4 * * *">
              <template #prepend>cron 表达式</template>
            </el-input>
            <p v-if="schedCfg?.hint" class="text-muted mt8" style="font-size: var(--fs-xs); line-height: 1.6">
              {{ schedCfg.hint }}
            </p>
          </div>
          <div class="oidc-save">
            <span v-if="schedCfg" class="text-muted" style="margin-right: 12px">
              快照默认：{{ schedCfg.snapshot_default }}
            </span>
            <el-button type="primary" :loading="schedSaving" @click="saveSchedule">保存</el-button>
          </div>
        </div>
</template>

<style scoped>
.oidc-wrap { display: flex; flex-direction: column; gap: var(--sp-3); max-width: 640px; }
.oidc-help { font-size: var(--fs-body); line-height: 1.6; }
.oidc-save { display: flex; justify-content: flex-end; }
.ch-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.mt8 { margin-top: 8px; }
.card { padding: 16px 18px; margin-bottom: 12px; }
</style>
