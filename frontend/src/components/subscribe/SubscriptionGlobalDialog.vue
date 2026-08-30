<script setup lang="ts">
// Subscriptions 的「全局默认设置」对话框（从 Subscriptions.vue 拆出；行为不变）。
import { useI18n } from 'vue-i18n'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'
import type { GlobalDefaults } from '@/api/subscribe'

defineModel<boolean>('open', { default: false })

defineProps<{
  form: GlobalDefaults
  saving: boolean
  allSites: { id: number; name: string }[]
}>()

const emit = defineEmits<{
  'update:open': [v: boolean]
  save: []
}>()

const PROMO_OPTIONS = [
  { value: 'free', label: 'Free' },
  { value: '2xfree', label: '2xFree' },
  { value: '2x', label: '2x' },
  { value: 'discount', label: 'Discount' },
  { value: '2xdiscount', label: '2xDiscount' },
]

const { t } = useI18n()
</script>

<template>
                <el-dialog
          :model-value="open"
          @update:model-value="emit('update:open', $event)"
          :title="t('subscriptions.globalTitle')"
          width="540px"
          append-to-body
          top="6vh"
          class="global-subscribe-dialog"
        >
          <div class="text-muted small" style="margin-bottom: 12px">
            {{ t('subscriptions.globalDesc') }}
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.resolution') }}</span>
            <el-input
              v-model="form.resolution"
              size="small"
              style="width: 160px"
              :placeholder="t('subscriptions.unlimited')"
            />
            <span class="small text-muted">{{ t('subscriptions.globalResHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.ratingGuard') }}</span>
            <el-switch v-model="form.enable_rating_guard" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalRatingHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.bestVersion') }}</span>
            <el-switch v-model="form.best_version" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalBestHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.officialOnly') }}</span>
            <el-switch v-model="form.official_only" size="small" />
            <span class="small text-muted">{{ t('subscriptions.globalOfficialHint') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.promoTypes') }}</span>
            <el-select
              v-model="form.promo_types"
              multiple
              collapse-tags
              collapse-tags-tooltip
              size="small"
              style="width: 240px"
              :placeholder="t('subscriptions.unlimited')"
            >
              <el-option v-for="o in PROMO_OPTIONS" :key="o.value" :label="o.label" :value="o.value" />
            </el-select>
            <span class="small text-muted">{{ t('subscriptions.emptyUnlimited') }}</span>
          </div>
          <div class="gd-row">
            <span class="small gd-label">{{ t('subscriptions.sites') }}</span>
            <SiteFilterDialog
              :model-value="form.site_ids"
              :sites="allSites"
              :title="t('subscriptions.globalSitesTitle')"
              @update:model-value="(ids: number[]) => (form.site_ids = ids)"
            >
              <span class="small">{{
                form.site_ids.length
                  ? t('subscriptions.nSites', { n: form.site_ids.length })
                  : t('subscriptions.allSites')
              }}</span>
            </SiteFilterDialog>
            <span class="small text-muted">{{ t('subscriptions.sitesEmptyHint') }}</span>
          </div>
          <template #footer>
            <el-button @click="emit('update:open', false)">{{ t('common.cancel') }}</el-button>
            <el-button type="primary" :loading="saving" @click="emit('save')">{{ t('common.save') }}</el-button>
          </template>
        </el-dialog>
</template>
