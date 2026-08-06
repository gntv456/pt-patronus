<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue'
import type { PluginInfo, PluginLog, PluginRelease } from '@/api/plugin'
import {
  runtimeState, runtimeType, runtimeLabel,
  permissionRisk,
  dependencyStatusType, dependencyStatusLabel,
  fmtTime,
} from './pluginsHelpers'
import { hostCapabilityLabel } from './pluginsLabels'

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
    :title="`插件审计：${target?.name || ''}`"
    width="980px"
    @update:model-value="emit('update:visible', $event)"
  >
    <div v-if="target" class="audit-body">
      <div class="audit-grid">
        <div class="audit-panel">
          <div class="audit-title">运行状态</div>
          <div class="audit-row">
            <span>状态</span>
            <el-tag size="small" :type="runtimeType(runtimeState(target))">
              {{ runtimeLabel(runtimeState(target)) }}
            </el-tag>
          </div>
          <div class="audit-row"><span>运行时</span><strong>{{ target.runtime }}</strong></div>
          <div class="audit-row"><span>进程</span><strong>{{ target.runtime_status?.pid || '-' }}</strong></div>
          <div class="audit-row"><span>托管进程</span><strong>{{ target.runtime_status?.managed ? '是' : '否' }}</strong></div>
          <div class="audit-row"><span>重启次数</span><strong>{{ target.runtime_status?.restart_count || 0 }}</strong></div>
          <div class="audit-row"><span>启动时间</span><strong>{{ fmtTime(target.runtime_status?.started_at) }}</strong></div>
          <div class="audit-row"><span>最近重启</span><strong>{{ fmtTime(target.runtime_status?.last_restart_at) }}</strong></div>
          <div class="audit-row"><span>下次重启</span><strong>{{ fmtTime(target.runtime_status?.next_restart_at) }}</strong></div>
          <div class="audit-row"><span>健康检查</span><strong>{{ fmtTime(target.runtime_status?.last_health_at) }}</strong></div>
          <div class="audit-row"><span>最近退出</span><strong>{{ target.runtime_status?.last_exit || '-' }}</strong></div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">权限</div>
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
          <div class="audit-title">宿主能力</div>
          <div class="chips">
            <el-tag v-for="capability in target.host_capabilities || []" :key="capability" size="small" effect="plain">
              {{ hostCapabilityLabel(capability) }}
            </el-tag>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">可选能力</div>
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
          <div class="audit-title">依赖</div>
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
            <div v-for="dependency in target.dependencies_status || []" :key="`${dependency.id}-note`" class="audit-note">
              {{ dependency.id }}{{ dependency.installed_version ? ` v${dependency.installed_version}` : '' }}{{ dependency.message ? `：${dependency.message}` : '' }}
            </div>
          </div>
        </div>
        <div class="audit-panel">
          <div class="audit-title">扩展点</div>
          <div class="audit-row"><span>动作</span><strong>{{ (target.actions || []).length }}</strong></div>
          <div class="audit-row"><span>事件</span><strong>{{ (target.events || []).length }}</strong></div>
          <div class="audit-row"><span>视图</span><strong>{{ (target.contributes?.views || []).length }}</strong></div>
          <div class="audit-row"><span>配置项</span><strong>{{ (target.config_schema || []).length }}</strong></div>
        </div>
      </div>
      <div class="audit-panel">
        <div class="logs-head">
          <div class="audit-title">最近日志</div>
          <el-button :icon="Refresh" size="small" :loading="loading" @click="emit('refresh')">刷新</el-button>
        </div>
        <el-table v-loading="loading" :data="logs" size="small" empty-text="暂无日志" max-height="320">
          <el-table-column label="时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="level" label="级别" width="90" />
          <el-table-column prop="event" label="事件/动作" min-width="130" />
          <el-table-column prop="message" label="消息" min-width="200" show-overflow-tooltip />
        </el-table>
      </div>
      <div class="audit-panel">
        <div class="audit-title">版本记录</div>
        <el-table :data="releases" size="small" empty-text="暂无版本记录" max-height="320">
          <el-table-column prop="version" label="版本" width="120" />
          <el-table-column label="发布时间" width="170">
            <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
          </el-table-column>
          <el-table-column prop="active" label="当前" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.active" size="small" type="success">当前</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="100">
            <template #default="{ row }">
              <el-button
                size="small"
                :disabled="row.active"
                :loading="rollbacking === row.id"
                @click="emit('rollback', row)"
              >
                回滚
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>
    </div>
  </el-dialog>
</template>
