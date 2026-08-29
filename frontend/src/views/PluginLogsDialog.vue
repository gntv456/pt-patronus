<script setup lang="ts">
import { ref, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { Refresh } from '@element-plus/icons-vue'
import type { PluginInfo, PluginLog } from '@/api/plugin'
import { fmtTime, jsonText } from './pluginsHelpers'

const { t } = useI18n()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'update:logPlugin', v: string): void
  (e: 'refresh'): void
}>()

// P3: 自动刷新
const props = defineProps<{ visible: boolean; plugins: PluginInfo[]; logs: PluginLog[]; logPlugin: string; loading: boolean }>()
const autoRefresh = ref(false)
let timer: ReturnType<typeof setInterval> | null = null
function applyAuto() {
  if (autoRefresh.value && props.visible) {
    if (!timer) timer = setInterval(() => emit('refresh'), 5000)
  } else {
    if (timer) { clearInterval(timer); timer = null }
  }
}
watch(() => [autoRefresh.value, props.visible], applyAuto, { immediate: true })
onBeforeUnmount(() => { if (timer) clearInterval(timer) })
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('pld.title')"
    width="900px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div class="logs-head">
      <el-select
        :model-value="logPlugin"
        clearable
        :placeholder="t('pld.allPlugins')"
        size="small"
        style="width: 200px"
        @update:model-value="emit('update:logPlugin', $event as string)"
        @change="emit('refresh')"
      >
        <el-option v-for="p in plugins" :key="p.id" :label="p.name" :value="p.id" />
      </el-select>
      <el-button :icon="Refresh" size="small" :loading="loading" @click="emit('refresh')">{{ t('common.refresh') }}</el-button>
      <el-tooltip :content="t('pld.autoTip')" placement="top">
        <el-switch v-model="autoRefresh" inline-prompt :active-text="t('pld.auto')" :inactive-text="t('pld.manual')" />
      </el-tooltip>
    </div>
    <el-table v-loading="loading" :data="logs" size="small" class="log-table" :empty-text="t('pad.noLogs')" max-height="520">
      <el-table-column :label="t('pad.time')" width="170">
        <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="plugin_id" :label="t('pld.plugin')" min-width="140" />
      <el-table-column prop="level" :label="t('pad.level')" width="90" />
      <el-table-column prop="event" :label="t('pad.event')" min-width="130" />
      <el-table-column prop="message" :label="t('pad.msg')" min-width="180" />
      <el-table-column :label="t('pld.data')" min-width="220">
        <template #default="{ row }">
          <span class="mono">{{ jsonText(row.data) }}</span>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>
