<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 站点矩阵的移动端视图（panelWidth < 560 时替换 el-table）：
// 卡片设计对齐 iOS 端站点页（favicon + 站名/站龄/状态 pill → 上传/下载双大块 →
// 四格小指标 → 同步时间 + 操作行），一屏可读、操作一击直达。
// props/emits 与 SiteMatrixTable 对齐——矩阵层只按 layout 切换组件，数据逻辑零重复。
import { useI18n } from 'vue-i18n'
import SiteFavicon from '../SiteFavicon.vue'
import SiteConnectivityStatusBadge from '../SiteConnectivityStatus.vue'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import type { SiteStatItem } from '@/api/dashboard'
import { fmtBytes, fmtNumber, fmtRatio } from '@/composables/useSiteMatrixData'

const { t } = useI18n()

const props = defineProps<{
  data: SiteStatItem[]
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
  (e: 'detail', row: SiteStatItem): void
  (e: 'refresh', row: SiteStatItem): void
  (e: 'history', row: SiteStatItem): void
  (e: 'command', command: string, row: SiteStatItem): void
  (e: 'reason', row: SiteStatItem): void
  (e: 'create'): void
}>()

function isNexus(row: SiteStatItem) {
  return props.siteConfig(row.site_id)?.platform === 'nexusphp'
}
function cookieAlert(row: SiteStatItem) {
  return props.cookieAlerts.includes(row.site_name)
}

// 站龄（join_date → 「N 周」）：不足 1 周显示「不足 1 周」，解析失败返回空。
function ageLabel(joinDate: string): string {
  const d = new Date(joinDate)
  if (Number.isNaN(d.getTime())) return ''
  const weeks = Math.floor((Date.now() - d.getTime()) / (7 * 86400_000))
  if (weeks < 1) return t('smt.lessWeek')
  return t('smt.ageWeeks', { n: weeks })
}

// 同步相对时间（updated_at → 「2 天前」/「5 小时前」/「刚刚」）。
// 超过 30 天显示短日期——「73993 分钟前」这类大数没有任何可读性（UX 实测）。
function relLabel(iso: string): string {
  const d = new Date(iso)
  // Go time.Time 零值（0001-01-01）解析成功但年份为 1——格式化出「1-01-01」
  // 毫无意义（UX 实测：新站点统计行就是零值），按未同步处理。
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1971) return ''
  const min = Math.floor((Date.now() - d.getTime()) / 60_000)
  if (min < 1) return t('smt.justNow')
  if (min < 60) return t('smt.minAgo', { n: min })
  if (min < 60 * 24) return t('smt.hourAgo', { n: Math.floor(min / 60) })
  const days = Math.floor(min / (60 * 24))
  if (min < 60 * 24 * 30) return t('smt.dayAgo', { n: days })
  const pad = (x: number) => String(x).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
</script>

<template>
  <div class="smc">
    <div v-if="!data.length" class="smc-empty">
      <strong>{{ t('smt.emptyTitle') }}</strong>
      <span>{{ t('smt.emptyDesc') }}</span>
      <el-button type="primary" size="small" @click="emit('create')">
        {{ t('smt.addSite') }}
      </el-button>
    </div>

    <article
      v-for="row in data"
      :key="row.site_id"
      class="smc-card glass"
      :class="{ alert: cookieAlert(row) }"
    >
      <header
        class="smc-head"
        role="button"
        tabindex="0"
        @click="emit('detail', row)"
        @keydown.enter="emit('detail', row)"
      >
        <SiteFavicon
          :name="row.site_name"
          :base-url="siteBaseUrl(row.site_id)"
          :size="40"
        />
        <div class="smc-title">
          <b>{{ row.site_name }}</b>
          <small v-if="ageLabel(row.join_date)">{{
            ageLabel(row.join_date)
          }}</small>
        </div>
        <div class="smc-badges">
          <span
            v-if="cookieAlert(row)"
            class="smc-alert-dot"
            :title="t('gnb.cookieExpired', { site: row.site_name })"
            aria-hidden="true"
          ></span>
          <SiteConnectivityStatusBadge
            :status="connectivity[row.site_id]"
            :loading="connectivityLoading"
            @reason.stop="emit('reason', row)"
          />
          <span v-if="row.level" class="smc-level">{{ row.level }}</span>
        </div>
      </header>

      <div class="smc-hero">
        <div class="smc-hero-cell up">
          <GlassIcon name="upload" :size="18" />
          <div>
            <i>{{ t('smt.up') }}</i>
            <b>{{ fmtBytes(row.true_uploaded || row.uploaded) }}</b>
          </div>
        </div>
        <div class="smc-hero-cell down">
          <GlassIcon name="download" :size="18" />
          <div>
            <i>{{ t('smt.down') }}</i>
            <b>{{ fmtBytes(row.true_downloaded || row.downloaded) }}</b>
          </div>
        </div>
      </div>

      <div class="smc-stats">
        <span class="smc-stat">
          <i>{{ t('smt.seedingCount') }}</i><b>{{ fmtNumber(row.seeding) }}</b>
        </span>
        <span class="smc-stat">
          <i>{{ t('smt.bonus') }}</i><b>{{ fmtNumber(row.bonus) }}</b>
        </span>
        <span class="smc-stat">
          <i>{{ t('smt.ratio') }}</i><b>{{ fmtRatio(row.ratio) }}</b>
        </span>
        <span class="smc-stat">
          <i>{{ t('smt.seedingSizeCol') }}</i>
          <b>{{ fmtBytes(row.seeding_size) }}</b>

        </span>
      </div>

      <footer class="smc-foot">
        <span class="smc-synced">{{
          t('smt.syncedAt', { t: relLabel(row.updated_at) })
        }}</span>
        <div class="smc-actions">
          <button class="smc-act primary" @click.stop="emit('detail', row)">
            {{ t('smt.siteDetail') }}
          </button>
          <button
            v-if="isNexus(row)"
            class="smc-act"
            :disabled="checkiningSiteId === row.site_id"
            @click.stop="emit('command', 'checkin', row)"
          >
            {{
              checkiningSiteId === row.site_id
                ? t('smt.checkining')
                : t('smt.checkin')
            }}
          </button>
          <button
            class="smc-act"
            :disabled="
              testingSiteId === row.site_id || !siteConfig(row.site_id)
            "
            @click.stop="emit('command', 'test', row)"
          >
            {{
              testingSiteId === row.site_id
                ? t('smt.testing')
                : t('cloud.testConn')
            }}
          </button>
          <el-dropdown trigger="click" @command="emit('command', $event, row)">
            <button class="smc-act ghost" :aria-label="t('smt.siteOps')">
              <GlassIcon name="settings" :size="14" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-if="siteConfig(row.site_id)"
                  command="edit"
                >{{

                  t('smt.editConfig')
                }}</el-dropdown-item>
                <el-dropdown-item
                  command="refresh"
                  :disabled="refreshing.has(row.site_id)"
                >
                  {{
                    refreshing.has(row.site_id)
                      ? t('smt.refreshing')
                      : t('smt.refreshStats')
                  }}
                </el-dropdown-item>
                <el-dropdown-item command="history">{{
                  t('smt.userHistory')
                }}</el-dropdown-item>
                <el-dropdown-item
                  v-if="canDelete && siteConfig(row.site_id)"
                  command="remove"
                  divided
                >{{
                  t('smt.removeSite')
                }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </footer>
    </article>
  </div>
</template>

<style scoped>
.smc {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.smc-empty {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
  align-items: center;
  padding: 40px var(--sp-4);
  text-align: center;
}
.smc-card {
  border-radius: 14px;
  padding: var(--sp-3);
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.smc-card.alert {
  outline: 1px solid var(--pt-danger-bg);
}
.smc-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  cursor: pointer;
  border: 0;
  background: none;
  text-align: left;
  padding: 0;
  width: 100%;
}
.smc-title {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}
.smc-title b {
  font-size: var(--fs-base, 14px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.smc-title small {
  color: var(--pt-text-muted);
  font-size: var(--fs-xs);
}
.smc-badges {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
}
.smc-alert-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--pt-danger);
}
.smc-level {
  padding: 1px 8px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
  background: var(--pt-primary-bg, rgba(99, 102, 241, 0.12));
  color: var(--pt-primary);
}
/* 上传/下载双大块：iOS 站点卡同构（浅底 + 图标 + 大数值） */
.smc-hero {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--sp-2);
}
.smc-hero-cell {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  border-radius: 10px;
  padding: 8px 10px;
}
.smc-hero-cell.up {
  background: rgba(59, 130, 246, 0.09);
}
.smc-hero-cell.down {
  background: rgba(16, 185, 129, 0.09);
}
.smc-hero-cell i {
  display: block;
  font-style: normal;
  font-size: var(--fs-xs);
  color: var(--pt-text-muted);
}
.smc-hero-cell b {
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.smc-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--sp-2);
}
.smc-stat {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  background: var(--color-surface-2);
  border-radius: 10px;
  padding: 6px 4px;
}
.smc-stat i {
  font-style: normal;
  font-size: var(--fs-xs);
  color: var(--pt-text-muted);
}
.smc-stat b {
  font-size: var(--fs-sm);
  font-variant-numeric: tabular-nums;
}
.smc-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
  border-top: 1px solid var(--divider);
  padding-top: var(--sp-2);
}
.smc-synced {
  font-size: var(--fs-xs);
  color: var(--pt-text-muted);
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.smc-actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: none;
}
.smc-act {
  border: 1px solid var(--divider);
  background: var(--color-surface-2, transparent);
  color: inherit;
  border-radius: var(--radius-pill);
  padding: 3px 10px;
  font-size: var(--fs-xs);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
}

</style>
