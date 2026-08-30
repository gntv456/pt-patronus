<script setup lang="ts">
// 站点矩阵表格的行展开详情面板（定义/平台/地址/真实上传下载/魔力等）。
// 从 SiteMatrixTable.vue 迁出；纯展示，数据与格式化函数经 props 注入。
import { useI18n } from 'vue-i18n'
import { fmtBytes, fmtNumber } from '@/composables/useSiteMatrixData'
import type { Site } from '@/api/site'
import type { SiteStatItem } from '@/api/dashboard'

const { t } = useI18n()

defineProps<{
  row: SiteStatItem
  siteConfig: (id: number) => Site | undefined
  cookieAlerts: string[]
}>()
</script>

<template>
  <div class="site-expand">
    <div class="exp-cell">
      <span>{{ t('smt.definition') }}</span
      ><b>{{ siteConfig(row.site_id)?.definition || '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.platform') }}</span
      ><b>{{ siteConfig(row.site_id)?.platform || '-' }}</b>
    </div>
    <div class="exp-cell exp-address">
      <span>{{ t('smt.address') }}</span
      ><b>{{ siteConfig(row.site_id)?.base_url || '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.configState') }}</span>
      <b class="config-flags">
        <i :class="['config-state', siteConfig(row.site_id)?.is_active ? 'is-active' : 'is-disabled']">
          {{ siteConfig(row.site_id)?.is_active ? t('common.enabled') : t('common.disabled') }}
        </i>
        <i v-if="cookieAlerts.includes(row.site_name)" class="config-state is-warning">{{
          t('smt.cookieStale')
        }}</i>
      </b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.userId') }}</span
      ><b>{{ row.user_id || '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.email') }}</span
      ><b>{{ row.email || '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.trueUp') }}</span
      ><b>{{ row.true_uploaded > 0 ? fmtBytes(row.true_uploaded) : '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.trueDown') }}</span
      ><b>{{ row.true_downloaded > 0 ? fmtBytes(row.true_downloaded) : '-' }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.uniqueGroups') }}</span
      ><b>{{ fmtNumber(row.unique_groups) }}</b>
    </div>
    <div class="exp-cell">
      <span>Perfect FLAC</span><b>{{ fmtNumber(row.perfect_flac) }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.invited') }}</span
      ><b>{{ fmtNumber(row.invited) }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.seedingBonus') }}</span
      ><b>{{ fmtNumber(row.seeding_bonus, 2) }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.seedingBonusH') }}</span
      ><b>{{ fmtNumber(row.seeding_bonus_per_hour, 2) }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.rescueBonusH') }}</span
      ><b>{{ fmtNumber(row.rescue_seeding_bonus_per_hour, 2) }}</b>
    </div>
    <div class="exp-cell">
      <span>{{ t('smt.claims') }}</span
      ><b>{{ row.claim_count || 0 }} / {{ row.max_claim || 0 }}</b>
    </div>
  </div>
</template>
