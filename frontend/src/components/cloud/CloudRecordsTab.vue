<script setup lang="ts">
// Cloud.vue 的「STRM 记录」与「云端任务队列」两个 tab（从 Cloud.vue 拆出；行为不变）。
import { Refresh } from '@element-plus/icons-vue'
import { useI18n } from 'vue-i18n'
import { TASK_KIND_LABEL, TASK_STATUS_LABEL, type StrmRecord, type CloudTask } from '@/api/cloud'

defineProps<{
  records: StrmRecord[]
  tasks: CloudTask[]
  tasksMeta: { total?: number; limit: number; truncated: boolean }
  loading: boolean
  driveName: (id: number) => string
  fmtSize: (n?: number) => string
}>()

defineModel<string>('taskStatusFilter', { default: '' })

const emit = defineEmits<{
  removeRecord: [r: StrmRecord]
  retryTask: [t: CloudTask]
  removeTask: [t: CloudTask]
  loadTasks: []
  showImportTree: []
}>()

const { t } = useI18n()
</script>

<template>
  <!-- STRM 记录 -->
  <el-tab-pane :label="t('cloud.tabRecords')" name="records">
    <el-table :data="records" size="small" :empty-text="t('cloud.recordsEmpty')">
      <el-table-column :label="t('cloud.drive')" width="110">
        <template #default="{ row }">{{ driveName((row as StrmRecord).drive_id) }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.cloudFile')" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as StrmRecord).cloud_path }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.localStrm')" min-width="200" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as StrmRecord).strm_path }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.size')" width="90">
        <template #default="{ row }">{{ fmtSize((row as StrmRecord).size) }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.source')" width="90">
        <template #default="{ row }">
          <el-tag v-if="(row as StrmRecord).subscription_id" size="small" type="success">{{
            t('cloud.srcSub')
          }}</el-tag>
          <el-tag v-else size="small" effect="plain">{{ t('cloud.srcRule') }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.action')" width="80">
        <template #default="{ row }">
          <el-button size="small" link type="danger" @click="emit('removeRecord', row as StrmRecord)">{{
            t('common.delete')
          }}</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-tab-pane>

  <!-- 云端任务队列 -->
  <el-tab-pane :label="t('cloud.tabTasks')" name="tasks">
    <div class="tab-toolbar">
      <el-select :model-value="taskStatusFilter" size="small" style="width: 120px" @change="emit('loadTasks')">
        <el-option :label="t('cloud.all')" value="" />
        <el-option :label="t('cloud.stPending')" value="pending" />
        <el-option :label="t('cloud.stRunning')" value="running" />
        <el-option :label="t('cloud.stSuccess')" value="success" />
        <el-option :label="t('cloud.stFailed')" value="failed" />
      </el-select>
      <el-button size="small" :icon="Refresh" @click="emit('loadTasks')">{{ t('common.refresh') }}</el-button>
      <el-button size="small" type="primary" @click="emit('showImportTree')">{{ t('cloud.importTree') }}</el-button>
    </div>
    <div v-if="tasksMeta.truncated" class="tasks-note">
      {{
        tasksMeta.total == null
          ? t('cloud.tasksTruncatedUnknown', { n: tasks.length })
          : t('cloud.tasksTruncated', { n: tasks.length, total: tasksMeta.total })
      }}
    </div>
    <el-table :data="tasks" size="small" :empty-text="t('cloud.tasksEmpty')">
      <el-table-column :label="t('cloud.kind')" width="100">
        <template #default="{ row }">{{ TASK_KIND_LABEL[(row as CloudTask).kind] || row.kind }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.drive')" width="110">
        <template #default="{ row }">{{ driveName((row as CloudTask).drive_id) }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.targetSrc')" min-width="220" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as CloudTask).cloud_path || (row as CloudTask).local_path }}</template>
      </el-table-column>
      <el-table-column :label="t('cloud.progress')" width="140">
        <template #default="{ row }">
          <el-progress
            :percentage="(row as CloudTask).progress || 0"
            :stroke-width="8"
            :status="
              (row as CloudTask).status === 'success'
                ? 'success'
                : (row as CloudTask).status === 'failed'
                  ? 'exception'
                  : undefined
            "
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('cloud.status')" width="90">
        <template #default="{ row }">
          <el-tag
            size="small"
            :type="
              (row as CloudTask).status === 'success'
                ? 'success'
                : (row as CloudTask).status === 'failed'
                  ? 'danger'
                  : (row as CloudTask).status === 'running'
                    ? 'warning'
                    : 'info'
            "
          >
            {{ TASK_STATUS_LABEL[(row as CloudTask).status] || row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('cloud.error')" min-width="160" show-overflow-tooltip>
        <template #default="{ row }">{{ (row as CloudTask).error || '-' }}</template>
      </el-table-column>
      <el-table-column :label="t('common.action')" width="130" fixed="right">
        <template #default="{ row }">
          <el-button
            v-if="(row as CloudTask).status === 'failed'"
            size="small"
            link
            type="primary"
            @click="emit('retryTask', row as CloudTask)"
            >{{ t('common.retry') }}</el-button
          >
          <el-button size="small" link type="danger" @click="emit('removeTask', row as CloudTask)">{{
            t('common.delete')
          }}</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-tab-pane>
</template>

<style scoped>
.tasks-note {
  margin: 0 0 var(--sp-2);
  font-size: var(--fs-xs);
  color: var(--el-color-warning);
}
</style>
