<script setup lang="ts">
import type { Site } from '@/api/site'
import type { MissingSummary, MissingSample } from '@/api/reseed'

const visible = defineModel<boolean>('visible', { required: true })
const targets = defineModel<number[]>('targets', { required: true })
const result = defineModel<MissingSummary | null>('result', { required: true })
const checking = defineModel<boolean>('checking', { required: true })

defineProps<{ sites: Site[] }>()

const emit = defineEmits<{ (e: 'check'): void }>()
</script>

<template>
  <el-dialog v-model="visible" title="未辅种检查" width="640px" top="8vh">
    <div style="display: flex; gap: var(--sp-2); margin-bottom: 12px">
      <el-select v-model="targets" multiple collapse-tags collapse-tags-tooltip filterable
        placeholder="选择目标站点（检查这些站是否已辅种）" style="flex: 1"
      >
        <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
      </el-select>
      <el-button type="primary" :loading="checking" @click="emit('check')">检查</el-button>
    </div>
    <div v-if="result">
      <p class="text-muted" style="margin: 0 0 8px">
        源做种 {{ result.total }} · 提交 IYUU {{ result.checked }} · 目标站 {{ result.targets }} 个已映射
      </p>
      <el-table :data="result.missing" size="small" empty-text="所选目标站均已辅种 ✅">
        <el-table-column prop="site_name" label="目标站" width="160" />
        <el-table-column label="未辅种数" width="100">
          <template #default="{ row }">
            <span v-if="row.count < 0" class="text-muted">无法判定</span>
            <el-tag v-else-if="row.count > 0" type="warning" size="small">{{ row.count }}</el-tag>
            <el-tag v-else type="success" size="small">已全覆盖</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="示例（最多 3 个）" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-muted">{{ (row.samples || []).map((x: MissingSample) => x.name).slice(0, 3).join(' / ') }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </el-dialog>
</template>
