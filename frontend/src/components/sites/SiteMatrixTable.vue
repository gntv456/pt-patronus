<script setup lang="ts">
import type { SiteStatItem } from '@/api/dashboard'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import GlassIcon from '@/components/GlassIcon.vue'
import SiteConnectivityStatusBadge from '@/components/SiteConnectivityStatus.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import { fmtBytes, fmtTime, fmtDate, fmtNumber, fmtRatio } from '@/composables/useSiteMatrixData'
import type { MatrixLayout } from '@/composables/useSiteMatrixData'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  data: SiteStatItem[]
  layout: MatrixLayout
  connectivity: Record<number, SiteConnectivityStatus>
  connectivityLoading: boolean
  cookieAlerts: readonly string[]
  canDelete: boolean
  refreshing: Set<number>
  testingSiteId: number | null
  checkiningSiteId: number | null
  siteBaseUrl: (id: number) => string
  siteConfig: (id: number) => Site | undefined
}>()

const emit = defineEmits<{
  (e: 'sort-change', payload: { prop: string | null; order: 'ascending' | 'descending' | null }): void
  (e: 'selection-change', rows: SiteStatItem[]): void
  (e: 'detail', row: SiteStatItem): void
  (e: 'refresh', row: SiteStatItem): void
  (e: 'history', row: SiteStatItem): void
  (e: 'command', command: string, row: SiteStatItem): void
  (e: 'reason', row: SiteStatItem): void
  (e: 'create'): void
}>()

</script>

<template>
  <el-table
    :key="layout"
    :data="data"
    class="matrix-table"
    :class="`is-${layout}`"
    style="width: 100%"
    :empty-text="t('smt.noMatch')"
    :default-sort="{ prop: 'updated_at', order: 'descending' }"
    @sort-change="emit('sort-change', $event)"
    row-key="site_id"
    @selection-change="emit('selection-change', $event as SiteStatItem[])"
  >
    <el-table-column v-if="canDelete" type="selection" width="36" :reserve-selection="true" fixed="left" />
    <el-table-column type="expand" width="32" fixed="left">
      <template #default="{ row }">
        <div class="site-expand">
          <div class="exp-cell"><span>{{ t('smt.definition') }}</span><b>{{ siteConfig(row.site_id)?.definition || '-' }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.platform') }}</span><b>{{ siteConfig(row.site_id)?.platform || '-' }}</b></div>
          <div class="exp-cell exp-address"><span>{{ t('smt.address') }}</span><b>{{ siteConfig(row.site_id)?.base_url || '-' }}</b></div>
          <div class="exp-cell">
            <span>{{ t('smt.configState') }}</span>
            <b class="config-flags">
              <i :class="['config-state', siteConfig(row.site_id)?.is_active ? 'is-active' : 'is-disabled']">
                {{ siteConfig(row.site_id)?.is_active ? t('common.enabled') : t('common.disabled') }}
              </i>
              <i v-if="cookieAlerts.includes(row.site_name)" class="config-state is-warning">{{ t('smt.cookieStale') }}</i>
            </b>
          </div>
          <div class="exp-cell"><span>{{ t('smt.userId') }}</span><b>{{ row.user_id || '-' }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.email') }}</span><b>{{ row.email || '-' }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.trueUp') }}</span><b>{{ row.true_uploaded > 0 ? fmtBytes(row.true_uploaded) : '-' }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.trueDown') }}</span><b>{{ row.true_downloaded > 0 ? fmtBytes(row.true_downloaded) : '-' }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.uniqueGroups') }}</span><b>{{ fmtNumber(row.unique_groups) }}</b></div>
          <div class="exp-cell"><span>Perfect FLAC</span><b>{{ fmtNumber(row.perfect_flac) }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.invited') }}</span><b>{{ fmtNumber(row.invited) }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.seedingBonus') }}</span><b>{{ fmtNumber(row.seeding_bonus, 2) }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.seedingBonusH') }}</span><b>{{ fmtNumber(row.seeding_bonus_per_hour, 2) }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.rescueBonusH') }}</span><b>{{ fmtNumber(row.rescue_seeding_bonus_per_hour, 2) }}</b></div>
          <div class="exp-cell"><span>{{ t('smt.claims') }}</span><b>{{ row.claim_count || 0 }} / {{ row.max_claim || 0 }}</b></div>
        </div>
      </template>
    </el-table-column>

    <el-table-column v-if="layout === 'compact'" :label="t('smt.siteStatus')" width="145" fixed="left">
      <template #default="{ row }">
        <div class="site-identity is-compact site-name-btn" role="button" @click="emit('detail', row as SiteStatItem)">
          <SiteFavicon :name="row.site_name" :base-url="siteBaseUrl(row.site_id)" :size="26" />
          <span>
            <b>{{ row.site_name }}</b>
            <SiteConnectivityStatusBadge
              :status="connectivity[row.site_id]"
              :loading="connectivityLoading"
              @reason.stop="emit('reason', row as SiteStatItem)"
            />
          </span>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout !== 'compact'"
      prop="site_name"
      :label="t('smt.site')"
      min-width="160"
      fixed="left"
      sortable="custom"
    >
      <template #default="{ row }">
        <button type="button" class="site-identity site-name-btn" @click="emit('detail', row as SiteStatItem)">
          <SiteFavicon :name="row.site_name" :base-url="siteBaseUrl(row.site_id)" :size="26" />
          <span>
            <b>{{ row.site_name }}</b>
            <small>{{ siteBaseUrl(row.site_id).replace(/^https?:\/\//, '').replace(/\/$/, '') || t('smt.noAddress') }}</small>
          </span>
        </button>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout !== 'compact'"
      prop="status"
      :label="t('cloud.status')"
      :width="layout === 'wide' ? 84 : 80"
      fixed="left"
      sortable="custom"
    >
      <template #default="{ row }">
        <SiteConnectivityStatusBadge
          :status="connectivity[row.site_id]"
          :loading="connectivityLoading"
          @reason="emit('reason', row as SiteStatItem)"
        />
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" :label="t('smt.userLevel')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('smt.user') }}</span><b :title="row.username">{{ row.username || '-' }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.level') }}</span><b :title="row.level">{{ row.level || '-' }}</b></div>
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
      <template #default="{ row }"><span class="level-tag">{{ row.level || '-' }}</span></template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="uploaded" :label="t('sitedetail.uploaded')" min-width="96" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="downloaded" :label="t('sitedetail.downloaded')" min-width="96" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.upDown')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('sitedetail.uploaded') }}</span><b>{{ fmtBytes(row.uploaded) }}</b></div>
          <div class="stacked-line"><span>{{ t('sitedetail.downloaded') }}</span><b>{{ fmtBytes(row.downloaded) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="true_ratio" :label="t('smt.trueRatio')" min-width="82" align="right" sortable="custom">
      <template #default="{ row }"><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="ratio" :label="t('smt.siteRatio')" min-width="82" align="right" sortable="custom">
      <template #default="{ row }"><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></template>
    </el-table-column>
    <el-table-column v-else :label="t('sitedetail.ratio')" min-width="90">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('smt.true') }}</span><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.site') }}</span><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" :label="t('smt.seedingSize')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('sitedetail.seeding') }}</span><b>{{ fmtNumber(row.seeding) }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.size') }}</span><b>{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-else prop="seeding" :label="t('sitedetail.seeding')" :min-width="layout === 'wide' ? 50 : 44" align="right" sortable="custom" />
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
          <div class="stacked-line"><span>{{ t('smt.uploads') }}</span><b>{{ fmtNumber(row.uploads) }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.joined') }}</span><b>{{ fmtDate(row.join_date) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-else prop="uploads" :label="t('smt.uploads')" :min-width="layout === 'wide' ? 50 : 44" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.uploads) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="bonus" :label="t('sitedetail.bonus')" min-width="100" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.bonus, 2) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="bonus_per_hour" :label="t('smt.bonusH')" min-width="72" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.bonus_per_hour, 2) }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.bonusPair')" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('sitedetail.bonus') }}</span><b>{{ fmtNumber(row.bonus, 2) }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.bonusH') }}</span><b>{{ fmtNumber(row.bonus_per_hour, 2) }}</b></div>
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
    <el-table-column v-if="layout === 'wide'" prop="last_access_at" :label="t('smt.lastAccess')" min-width="135" sortable="custom">
      <template #default="{ row }">{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="updated_at" :label="t('smt.updatedAt')" min-width="135" sortable="custom">
      <template #default="{ row }">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</template>
    </el-table-column>
    <el-table-column v-else :label="t('smt.accessUpdate')" min-width="140">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>{{ t('smt.access') }}</span><b>{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</b></div>
          <div class="stacked-line"><span>{{ t('smt.update') }}</span><b>{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column :label="t('common.action')" :min-width="layout === 'wide' ? 68 : 60">
      <template #default="{ row }">
        <div class="row-actions">
          <button class="row-act" :title="t('smt.userHistory')" @click="emit('history', row as SiteStatItem)">
            <GlassIcon name="history" from="#2563eb" to="#0891b2" :size="16" />
          </button>
          <el-dropdown trigger="click" @command="emit('command', $event, row as SiteStatItem)">
            <button class="row-act" :title="t('smt.siteOps')">
              <GlassIcon name="settings" from="#475569" to="#2563eb" :size="16" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="detail">{{ t('smt.siteDetail') }}</el-dropdown-item>
                <el-dropdown-item command="refresh" :disabled="refreshing.has(row.site_id)">
                  {{ refreshing.has(row.site_id) ? t('smt.refreshing') : t('smt.refreshStats') }}
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="siteConfig(row.site_id)?.platform === 'nexusphp'"
                  command="checkin"
                  :disabled="checkiningSiteId === row.site_id"
                >
                  {{ checkiningSiteId === row.site_id ? t('smt.checkining') : t('smt.checkin') }}
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="siteConfig(row.site_id)"
                  command="test"
                  :disabled="testingSiteId === row.site_id"
                >
                  {{ testingSiteId === row.site_id ? t('smt.testing') : t('cloud.testConn') }}
                </el-dropdown-item>
                <el-dropdown-item v-if="siteConfig(row.site_id)" command="edit">{{ t('smt.editConfig') }}</el-dropdown-item>
                <el-dropdown-item v-if="canDelete && siteConfig(row.site_id)" command="remove" divided>{{ t('smt.removeSite') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </template>
    </el-table-column>
    <template #empty>
      <div class="matrix-empty">
        <strong>{{ t('smt.emptyTitle') }}</strong>
        <span>{{ t('smt.emptyDesc') }}</span>
        <el-button type="primary" size="small" @click="$emit('create')">{{ t('smt.addSite') }}</el-button>
      </div>
    </template>
  </el-table>
</template>

<style scoped src="./siteOperationsMatrix.css"></style>
