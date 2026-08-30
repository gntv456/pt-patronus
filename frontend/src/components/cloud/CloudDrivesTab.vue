<script setup lang="ts">
// Cloud.vue 的「网盘」tab：驱动卡片列表 + 测试/配额/浏览/整理/修复操作（从 Cloud.vue 拆出）。
import { useI18n } from 'vue-i18n'
import { TYPE_LABEL, type CloudDrive } from '@/api/cloud'

defineProps<{
  drives: CloudDrive[]
  mode: string
  testing: number | null
  organizing: boolean
  repairing: boolean
  loading: boolean
  quotas: Record<number, { used?: number; total?: number }>
  fmtSize: (n?: number) => string
}>()

const emit = defineEmits<{
  test: [d: CloudDrive]
  quota: [d: CloudDrive]
  browse: [d: CloudDrive]
  create: []
  edit: [d: CloudDrive]
  remove: [d: CloudDrive]
  auth: []
  organize: []
  repair: []
  generate: [d: unknown]
}>()

const { t } = useI18n()
</script>

<template>
      <!-- 网盘 -->
      <el-tab-pane :label="t('cloud.tabDrives')" name="drives">
        <div v-loading="loading" class="grid">
          <div v-for="d in drives" :key="d.id" class="glass glass-hover card">
            <div class="card-head">
              <GlassIcon name="cloud" :size="34" from="#4cc9f0" to="#4361ee" />
              <div class="card-title">
                <div class="name">
                  {{ d.name }}
                  <el-tag size="small" effect="plain">{{ TYPE_LABEL[d.type] }}</el-tag>
                  <el-tag v-if="d.is_default" size="small" type="success">{{ t('cloud.defaultTag') }}</el-tag>
                </div>
                <div class="meta">{{ t('cloud.rootPath') }}：{{ d.root_path || '/' }}</div>
                <div v-if="quotas[d.id]" class="meta">
                  {{ t('cloud.quota') }}：{{ fmtSize(quotas[d.id].used) }} / {{ fmtSize(quotas[d.id].total) }}
                </div>
              </div>
            </div>
            <div class="actions">
              <el-button size="small" :loading="testing === d.id" @click="emit('test', d)">{{
                t('cloud.testConn')
              }}</el-button>
              <el-button size="small" @click="emit('quota', d)">{{ t('cloud.viewQuota') }}</el-button>
              <el-button size="small" @click="emit('browse', d)">{{ t('cloud.browse') }}</el-button>
              <el-button size="small" @click="emit('edit', d)">{{ t('common.edit') }}</el-button>
              <el-button size="small" type="danger" plain @click="emit('remove', d)">{{ t('common.delete') }}</el-button>
            </div>
          </div>
          <EmptyState
            v-if="!drives.length && !loading"
            icon="☁️"
            :title="t('cloud.emptyTitle')"
            :description="t('cloud.emptyDesc')"
          >
            <el-button type="primary" @click="emit('create')">{{ t('cloud.addDrive2') }}</el-button>
            <el-button @click="emit('auth')">{{ t('cloud.qrAuth115') }}</el-button>
            <el-button @click="emit('organize')" :loading="organizing">{{ t('cloud.organize') }}</el-button>
            <el-button @click="emit('repair')" :loading="repairing">{{ t('cloud.strmRepair') }}</el-button>
          </EmptyState>
        </div>
      </el-tab-pane>
</template>
