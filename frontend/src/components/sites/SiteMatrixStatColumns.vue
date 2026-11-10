<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 站点矩阵表格的数据统计列组（用户/上传下载/分享率/做种/魔力/最近等，compact|standard|wide 三形态）。
// 从 SiteMatrixTable.vue 迁出；纯展示列，格式化函数与布局经 props 注入。
import type { MatrixLayout } from '@/composables/useSiteMatrixData'
import { fmtBytes, fmtTime, fmtDate, fmtNumber, fmtRatio } from '@/composables/useSiteMatrixData'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{ layout: MatrixLayout }>()
</script>

<template>
    <el-table-column v-if="layout === 'compact'" :label="t('smt.userLevel')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('smt.user') }}</span
            ><b :title="row.username">{{ row.username || '-' }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.level') }}</span
            ><b :title="row.level">{{ row.level || '-' }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-else
      prop="username"
      :label="t('smt.username')"
      :min-width="layout === 'wide' ? 90 : 70"
      show-overflow-tooltip
      sortable="custom"
    />
    <el-table-column
      v-if="layout !== 'compact'"
      prop="level"
      :label="t('smt.level')"
      :min-width="layout === 'wide' ? 72 : 64"
      show-overflow-tooltip
      sortable="custom"
    >
      <template #default="{ row }"
        ><span class="level-tag">{{ row.level || '-' }}</span></template
      >
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="uploaded"
      :label="t('sitedetail.uploaded')"
      min-width="96"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="downloaded"
      :label="t('sitedetail.downloaded')"
      min-width="96"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.upDown')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('sitedetail.uploaded') }}</span
            ><b>{{ fmtBytes(row.uploaded) }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('sitedetail.downloaded') }}</span
            ><b>{{ fmtBytes(row.downloaded) }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="true_ratio"
      :label="t('smt.trueRatio')"
      min-width="82"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }"
        ><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{
          fmtRatio(row.true_ratio)
        }}</b></template
      >
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="ratio"
      :label="t('smt.siteRatio')"
      min-width="82"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }"
        ><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{
          fmtRatio(row.ratio)
        }}</b></template
      >
    </el-table-column>
    <el-table-column v-else :label="t('sitedetail.ratio')" min-width="90">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('smt.true') }}</span
            ><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{
              fmtRatio(row.true_ratio)
            }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.site') }}</span
            ><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" :label="t('smt.seedingSize')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('sitedetail.seeding') }}</span
            ><b>{{ fmtNumber(row.seeding) }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.size') }}</span
            ><b>{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-else
      prop="seeding"
      :label="t('sitedetail.seeding')"
      :min-width="layout === 'wide' ? 50 : 44"
      align="right"
      sortable="custom"
    />
    <el-table-column
      v-if="layout !== 'compact'"
      prop="seeding_size"
      :label="t('smt.seedingSizeCol')"
      :min-width="layout === 'wide' ? 90 : 80"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" :label="t('smt.uploadsJoined')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('smt.uploads') }}</span
            ><b>{{ fmtNumber(row.uploads) }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.joined') }}</span
            ><b>{{ fmtDate(row.join_date) }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-else
      prop="uploads"
      :label="t('smt.uploads')"
      :min-width="layout === 'wide' ? 50 : 44"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtNumber(row.uploads) }}</template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="bonus"
      :label="t('sitedetail.bonus')"
      min-width="100"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtNumber(row.bonus, 2) }}</template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="bonus_per_hour"
      :label="t('smt.bonusH')"
      min-width="72"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtNumber(row.bonus_per_hour, 2) }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.bonusPair')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('sitedetail.bonus') }}</span
            ><b>{{ fmtNumber(row.bonus, 2) }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.bonusH') }}</span
            ><b>{{ fmtNumber(row.bonus_per_hour, 2) }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout !== 'compact'"
      prop="join_date"
      :label="t('smt.joined')"
      :min-width="layout === 'wide' ? 88 : 80"
      show-overflow-tooltip
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtDate(row.join_date) }}</template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="last_access_at"
      :label="t('smt.lastAccess')"
      min-width="135"
      sortable="custom"
    >
      <template #default="{ row }">{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</template>
    </el-table-column>
    <el-table-column
      v-if="layout === 'wide'"
      prop="updated_at"
      :label="t('smt.updatedAt')"
      min-width="135"
      sortable="custom"
    >
      <template #default="{ row }">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.accessUpdate')" min-width="140">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line">
            <span>{{ t('smt.access') }}</span
            ><b>{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</b>
          </div>
          <div class="stacked-line">
            <span>{{ t('smt.update') }}</span
            ><b>{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</b>
          </div>
        </div>
      </template>
    </el-table-column>
</template>
