<script setup lang="ts">
import type { SiteStatItem } from '@/api/dashboard'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import GlassIcon from '@/components/GlassIcon.vue'
import SiteConnectivityStatusBadge from '@/components/SiteConnectivityStatus.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import SiteMatrixExpand from './SiteMatrixExpand.vue'
import SiteMatrixStatColumns from './SiteMatrixStatColumns.vue'
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
        <SiteMatrixExpand :row="(row as SiteStatItem)" :site-config="siteConfig" :cookie-alerts="[...cookieAlerts]" />
      </template>
    </el-table-column>

    <el-table-column v-if="layout === 'compact'" :label="t('smt.siteStatus')" width="145" fixed="left">
      <template #default="{ row }">
        <div
          v-a11y-button
          class="site-identity is-compact site-name-btn"
          role="button"
          @click="emit('detail', row as SiteStatItem)"
        >
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
            <small>{{
              siteBaseUrl(row.site_id)
                .replace(/^https?:\/\//, '')
                .replace(/\/$/, '') || t('smt.noAddress')
            }}</small>
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
    <SiteMatrixStatColumns :layout="layout" />
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
                <el-dropdown-item v-if="siteConfig(row.site_id)" command="edit">{{
                  t('smt.editConfig')
                }}</el-dropdown-item>
                <el-dropdown-item v-if="canDelete && siteConfig(row.site_id)" command="remove" divided>{{
                  t('smt.removeSite')
                }}</el-dropdown-item>
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
