<script setup lang="ts">
// 订阅卡片信息区：标题/三态开关簇/分辨率/促销/站点过滤/操作按钮。
// 从 Subscriptions.vue 迁出；s 为宿主列表项引用，行内编辑直接改 s 并调 API（乐观更新）。
/* eslint-disable vue/no-mutating-props */
import type { Subscription, GlobalDefaults } from '@/api/subscribe'
import { useI18n } from 'vue-i18n'
import MediaPoster from '@/components/MediaPoster.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()

// 状态 → pt-chip 语义变体（W4 标签统一：原 .chip.subscribed/.completed/.paused 三套自定义样式）
function statusChipClass(status: string): string {
  if (status === 'subscribed') return 'pt-chip--success'
  if (status === 'completed') return 'pt-chip--info'
  return 'pt-chip--muted' // paused / 未知
}

defineProps<{
  s: Subscription
  batchMode: boolean
  selected: boolean
  checking: number | null
  globalDefaults: GlobalDefaults
  sites: { id: number; name: string }[]
  promoOptions: { value: string; label: string }[]
  effRatingGuard: (s: Subscription) => boolean
  effBestVersion: (s: Subscription) => boolean
  effOfficialOnly: (s: Subscription) => boolean
  statusText: (s: string) => string
  fmtTime: (t?: string) => string
  promoLabel: (v: string) => string
}>()

const emit = defineEmits<{
  (e: 'toggleSelect', id: number): void
  (e: 'setResolution', s: Subscription): void
  (e: 'setRatingGuard', s: Subscription, v: boolean): void
  (e: 'resetRatingGuard', s: Subscription): void
  (e: 'setBestVersion', s: Subscription, v: boolean): void
  (e: 'resetBestVersion', s: Subscription): void
  (e: 'setOfficialOnly', s: Subscription, v: boolean): void
  (e: 'resetOfficialOnly', s: Subscription): void
  (e: 'setPromoTypes', s: Subscription, v: string[]): void
  (e: 'setSiteIds', s: Subscription, ids: number[]): void
  (e: 'checkNow', s: Subscription): void
  (e: 'toggleStatus', s: Subscription): void
  (e: 'remove', s: Subscription): void
}>()
</script>

<template>
  <div class="sub glass-hover" :class="{ selected: batchMode && selected }">
    <div class="poster">
      <el-checkbox v-if="batchMode" :model-value="selected" class="batch-check" @change="emit('toggleSelect', s.id)" />
      <MediaPoster :url="s.poster_url" :kind="s.type" />
    </div>
    <div class="info">
      <div class="row1">
        <span class="s-title">{{ s.title }}</span>
        <span v-if="s.year" class="text-muted">({{ s.year }})</span>
        <span class="pt-chip" :class="statusChipClass(s.status)">{{ statusText(s.status) }}</span>
        <span v-if="s.resolution" class="pt-chip pt-chip--info">{{ s.resolution }}</span>
      </div>
      <div class="text-muted small">
        {{ t('subscriptions.lastChecked', { time: fmtTime(s.last_checked_at) }) }} ·
        {{ t('subscriptions.typeLabel') }}
        {{ s.type === 'tv' ? t('subscriptions.type.tv') : t('subscriptions.type.movie') }} ·
        {{ t('subscriptions.sourceLabel') }} {{ s.source }}
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.resFilter') }}</span>
        <el-input
          v-model="s.resolution"
          size="small"
          style="width: 120px"
          :placeholder="t('subscriptions.unlimited')"
          @change="emit('setResolution', s)"
        />
        <span v-if="!s.resolution && globalDefaults.resolution" class="small text-muted">{{
          t('subscriptions.followGlobal', { v: globalDefaults.resolution })
        }}</span>
        <span v-else class="small text-muted">{{ t('subscriptions.resHint') }}</span>
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.ratingGuardColon') }}</span>
        <el-switch
          :model-value="effRatingGuard(s)"
          :disabled="s.enable_rating_guard === null"
          size="small"
          @update:model-value="(v: any) => emit('setRatingGuard', s, !!v)"
        />
        <span v-if="s.enable_rating_guard === null" class="small text-muted">{{
          t('subscriptions.followGlobal', {
            v: globalDefaults.enable_rating_guard ? t('common.on') : t('common.off'),
          })
        }}</span>
        <template v-else>
          <el-button link type="primary" class="small" @click="emit('resetRatingGuard', s)">{{
            t('subscriptions.resetGlobal')
          }}</el-button>
          <span class="small text-muted">{{ t('subscriptions.ratingThresholdHint') }}</span>
        </template>
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.bestVersionColon') }}</span>
        <el-switch
          :model-value="effBestVersion(s)"
          :disabled="s.best_version === null"
          size="small"
          @update:model-value="(v: any) => emit('setBestVersion', s, !!v)"
        />
        <span v-if="s.best_version === null" class="small text-muted">{{
          t('subscriptions.followGlobal', {
            v: globalDefaults.best_version ? t('common.on') : t('common.off'),
          })
        }}</span>
        <template v-else>
          <el-button link type="primary" class="small" @click="emit('resetBestVersion', s)">{{
            t('subscriptions.resetGlobal')
          }}</el-button>
          <span class="small text-muted">{{ t('subscriptions.bestHint') }}</span>
        </template>
        <span v-if="effBestVersion(s) && s.type === 'movie' && s.current_priority" class="pt-chip pt-chip--info">
          Pri {{ s.current_priority }}
        </span>
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.officialOnlyColon') }}</span>
        <el-switch
          :model-value="effOfficialOnly(s)"
          :disabled="s.official_only === null"
          size="small"
          @update:model-value="(v: any) => emit('setOfficialOnly', s, !!v)"
        />
        <span v-if="s.official_only === null" class="small text-muted">{{
          t('subscriptions.followGlobal', {
            v: globalDefaults.official_only ? t('common.on') : t('common.off'),
          })
        }}</span>
        <template v-else>
          <el-button link type="primary" class="small" @click="emit('resetOfficialOnly', s)">{{
            t('subscriptions.resetGlobal')
          }}</el-button>
          <span class="small text-muted">{{ t('subscriptions.officialHint') }}</span>
        </template>
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.promoTypesColon') }}</span>
        <el-select
          :model-value="s.promo_types"
          multiple
          collapse-tags
          collapse-tags-tooltip
          size="small"
          style="width: 240px"
          :placeholder="t('subscriptions.unlimited')"
          @change="(v: any) => emit('setPromoTypes', s, v as string[])"
        >
          <el-option v-for="o in promoOptions" :key="o.value" :label="o.label" :value="o.value" />
        </el-select>
        <span v-if="!s.promo_types.length && globalDefaults.promo_types.length" class="small text-muted">{{
          t('subscriptions.followGlobal', { v: globalDefaults.promo_types.map(promoLabel).join('、') })
        }}</span>
        <span v-else class="small text-muted">{{ t('subscriptions.promoEmptyHint') }}</span>
      </div>
      <div class="res-row">
        <span class="small">{{ t('subscriptions.sitesColon') }}</span>
        <SiteFilterDialog
          :model-value="s.site_ids"
          :sites="sites"
          :title="t('subscriptions.sitesDialogTitle')"
          @update:model-value="(ids: number[]) => emit('setSiteIds', s, ids)"
        >
          <span class="small">{{
            s.site_ids.length
              ? t('subscriptions.nSites', { n: s.site_ids.length })
              : globalDefaults.site_ids.length
                ? t('subscriptions.followGlobalNSites', { n: globalDefaults.site_ids.length })
                : t('subscriptions.allSites')
          }}</span>
        </SiteFilterDialog>
        <span v-if="!s.site_ids.length && globalDefaults.site_ids.length" class="small text-muted">{{
          t('subscriptions.sitesFollowGlobalHint')
        }}</span>
        <span v-else class="small text-muted">{{ t('subscriptions.sitesEmptyHint') }}</span>
      </div>
      <div class="actions">
        <el-button size="small" type="primary" :loading="checking === s.id" @click="emit('checkNow', s)">{{
          t('subscriptions.checkNow')
        }}</el-button>
        <el-button size="small" @click="emit('toggleStatus', s)">{{
          s.status === 'paused' ? t('subscriptions.resume') : t('subscriptions.pause')
        }}</el-button>
        <el-button size="small" type="danger" plain @click="emit('remove', s)">{{ t('common.delete') }}</el-button>
      </div>
    </div>
  </div>
</template>
