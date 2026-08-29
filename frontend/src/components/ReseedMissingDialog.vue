<script setup lang="ts">
import type { Site } from '@/api/site'
import { useI18n } from 'vue-i18n'
import type { MissingSummary, MissingSample } from '@/api/reseed'

const { t } = useI18n()

const visible = defineModel<boolean>('visible', { required: true })
const targets = defineModel<number[]>('targets', { required: true })
const result = defineModel<MissingSummary | null>('result', { required: true })
const checking = defineModel<boolean>('checking', { required: true })

defineProps<{ sites: Site[] }>()

const emit = defineEmits<{ (e: 'check'): void }>()
</script>

<template>
  <el-dialog v-model="visible" :title="t('rmd.title')" width="640px" top="8vh">
    <div style="display: flex; gap: var(--sp-2); margin-bottom: 12px">
      <el-select v-model="targets" multiple collapse-tags collapse-tags-tooltip filterable
        :placeholder="t('rmd.targetsPh')" style="flex: 1"
      >
        <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id" />
      </el-select>
      <el-button type="primary" :loading="checking" @click="emit('check')">{{ t('rmd.check') }}</el-button>
    </div>
    <div v-if="result">
      <p class="text-muted" style="margin: 0 0 8px">
        {{ t('rmd.summary', { total: result.total, checked: result.checked, targets: result.targets }) }}
      </p>
      <el-table :data="result.missing" size="small" :empty-text="t('rmd.allCovered')">
        <el-table-column prop="site_name" :label="t('rmd.target')" width="160" />
        <el-table-column :label="t('rmd.missingN')" width="100">
          <template #default="{ row }">
            <span v-if="row.count < 0" class="text-muted">{{ t('rmd.unknown') }}</span>
            <el-tag v-else-if="row.count > 0" type="warning" size="small">{{ row.count }}</el-tag>
            <el-tag v-else type="success" size="small">{{ t('rmd.allCoveredShort') }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('rmd.samples')" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="text-muted">{{ (row.samples || []).map((x: MissingSample) => x.name).slice(0, 3).join(' / ') }}</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </el-dialog>
</template>
