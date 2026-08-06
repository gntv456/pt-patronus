<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue'
import type { PluginInfo, PluginLog } from '@/api/plugin'
import { fmtTime, jsonText } from './pluginsHelpers'

defineProps<{
  visible: boolean
  plugins: PluginInfo[]
  logs: PluginLog[]
  logPlugin: string
  loading: boolean
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:logPlugin', v: string): void
  (e: 'refresh'): void
}>()
</script>

<template>
  <el-dialog
    :model-value="visible"
    title="插件日志"
    width="900px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="logs-head">
      <el-select
        :model-value="logPlugin"
        clearable
        placeholder="全部插件"
        size="small"
        style="width: 200px"
        @update:model-value="emit('update:logPlugin', $event as string)"
        @change="emit('refresh')"
      >
        <el-option v-for="p in plugins" :key="p.id" :label="p.name" :value="p.id" />
      </el-select>
      <el-button :icon="Refresh" size="small" :loading="loading" @click="emit('refresh')">刷新</el-button>
    </div>
    <el-table v-loading="loading" :data="logs" size="small" class="log-table" empty-text="暂无日志" max-height="520">
      <el-table-column label="时间" width="170">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="plugin_id" label="插件" min-width="140" />
      <el-table-column prop="level" label="级别" width="90" />
      <el-table-column prop="event" label="事件/动作" min-width="130" />
      <el-table-column prop="message" label="消息" min-width="180" />
      <el-table-column label="数据" min-width="220">
        <template #default="{ row }">
          <span class="mono">{{ jsonText(row.data) }}</span>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>
