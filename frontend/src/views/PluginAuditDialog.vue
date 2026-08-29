<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import type { PluginInfo, PluginLog, PluginRelease } from '@/api/plugin'
import {
  runtimeState,
  runtimeType,
  runtimeLabel,
  permissionRisk,
  dependencyStatusType,
  dependencyStatusLabel,
  fmtTime,
} from './pluginsHelpers'
import { hostCapabilityLabel } from './pluginsLabels'

const { t } = useI18n()

defineProps<{
  visible: boolean
  target: PluginInfo | null
  logs: PluginLog[]
  releases: PluginRelease[]
  loading: boolean
  rollbacking: number | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
  (e: 'refresh'): void
  (e: 'rollback', row: unknown): void
}>()

function permissionRiskType(perm: string) {
  const risk = permissionRisk(perm)
  return risk === 'high' ? 'danger' : risk === 'medium' ? 'warning' : 'success'
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="t('pad.title', { name: target?.name || '' })"
    width="980px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div v-if="target" class="audit-body">
      <div class="audit-grid">
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.runtime') }}</div>
          <div class="audit-row">
            <span>{{ t('pad.state') }}</span>
            <el-tag size="small" :type="runtimeType(runtimeState(target))">
              {{ runtimeLabel(runtimeState(target)) }}
            </el-tag>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.rt') }}</span
            ><strong>{{ target.runtime }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.pid') }}</span
            ><strong>{{ target.runtime_status?.pid || '-' }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.managed') }}</span
            ><strong>{{ target.runtime_status?.managed ? t('common.yes') : t('common.no') }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.restarts') }}</span
            ><strong>{{ target.runtime_status?.restart_count || 0 }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.startedAt') }}</span
            ><strong>{{ fmtTime(target.runtime_status?.started_at) }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.lastRestart') }}</span
            ><strong>{{ fmtTime(target.runtime_status?.last_restart_at) }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.nextRestart') }}</span
            ><strong>{{ fmtTime(target.runtime_status?.next_restart_at) }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.health') }}</span
            ><strong>{{ fmtTime(target.runtime_status?.last_health_at) }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.lastExit') }}</span
            ><strong>{{ target.runtime_status?.last_exit || '-' }}</strong>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.perms') }}</div>
          <div class="chips">
            <el-tag
              v-for="perm in target.permissions || []"
              :key="perm"
              size="small"
              effect="plain"
              :type="permissionRiskType(perm)"
            >
              {{ perm }}
            </el-tag>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.hostCaps') }}</div>
          <div class="chips">
            <el-tag v-for="capability in target.host_capabilities || []" :key="capability" size="small" effect="plain">
              {{ hostCapabilityLabel(capability) }}
            </el-tag>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.optCaps') }}</div>
          <div class="chips">
            <el-tag
              v-for="capability in target.available_optional_host_capabilities || []"
              :key="`available-${capability}`"
              size="small"
              type="success"
              effect="plain"
            >
              {{ hostCapabilityLabel(capability) }}
            </el-tag>
            <el-tag
              v-for="capability in target.missing_optional_host_capabilities || []"
              :key="`missing-${capability}`"
              size="small"
              type="warning"
              effect="plain"
            >
              {{ hostCapabilityLabel(capability) }}
            </el-tag>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.deps') }}</div>
          <div class="chips">
            <el-tag
              v-for="dependency in target.dependencies_status || []"
              :key="dependency.id"
              size="small"
              effect="plain"
              :type="dependencyStatusType(dependency)"
            >
              {{ dependency.id }} · {{ dependencyStatusLabel(dependency) }}
            </el-tag>
          </div>
          <div v-if="(target.dependencies_status || []).length" class="audit-notes">
            <div
              v-for="dependency in target.dependencies_status || []"
              :key="`${dependency.id}-note`"
              class="audit-note"
            >
              {{ dependency.id }}{{ dependency.installed_version ? ` v${dependency.installed_version}` : ''
              }}{{ dependency.message ? `：${dependency.message}` : '' }}
            </div>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">{{ t('pad.extPoints') }}</div>
          <div class="audit-row">
            <span>{{ t('pad.actions') }}</span
            ><strong>{{ (target.actions || []).length }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.events') }}</span
            ><strong>{{ (target.events || []).length }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.views') }}</span
            ><strong>{{ (target.contributes?.views || []).length }}</strong>
          </div>
          <div class="audit-row">
            <span>{{ t('pad.configs') }}</span
            ><strong>{{ (target.config_schema || []).length }}</strong>
          </div>
        </div>
      </div>
      <div class="audit-panel">
        <div class="logs-head">
          <div class="audit-title">{{ t('pad.logs') }}</div>
          <el-button :icon="Refresh" size="small" :loading="loading" @click="emit('refresh')">{{
            t('common.refresh')
          }}</el-button>
        </div>
        <el-table v-loading="loading" :data="logs" size="small" :empty-text="t('pad.noLogs')" max-height="320">
          <el-table-column :label="t('pad.time')" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="level" :label="t('pad.level')" width="90" />
          <el-table-column prop="event" :label="t('pad.event')" min-width="130" />
          <el-table-column prop="message" :label="t('pad.msg')" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
      <div class="audit-panel">
        <div class="audit-title">{{ t('pad.releases') }}</div>
        <el-table :data="releases" size="small" :empty-text="t('pad.noReleases')" max-height="320">
          <el-table-column prop="version" :label="t('pad.version')" width="120" />
          <el-table-column :label="t('pad.releasedAt')" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="active" :label="t('pad.current')" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.active" size="small" type="success">{{ t('pad.current') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.action')" width="100">
            <template #default="{ row }">
              <el-button
                size="small"
                :disabled="row.active"
                :loading="rollbacking === row.id"
                @click="emit('rollback', row)"
              >
                {{ t('pad.rollback') }}
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </el-dialog>
</template>
