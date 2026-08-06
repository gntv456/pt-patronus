<script setup lang="ts">
import type { SiteStatItem } from '@/api/dashboard'
import type { Site, SiteConnectivityStatus } from '@/api/site'
import GlassIcon from '@/components/GlassIcon.vue'
import SiteConnectivityStatusBadge from '@/components/SiteConnectivityStatus.vue'
import SiteFavicon from '@/components/SiteFavicon.vue'
import { fmtBytes, fmtTime, fmtDate, fmtNumber, fmtRatio } from '@/composables/useSiteMatrixData'
import type { MatrixLayout } from '@/composables/useSiteMatrixData'

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
    empty-text="暂无匹配站点统计"
    :default-sort="{ prop: 'updated_at', order: 'descending' }"
    @sort-change="emit('sort-change', $event)"
    row-key="site_id"
    @selection-change="emit('selection-change', $event as SiteStatItem[])"
  >
    <el-table-column v-if="canDelete" type="selection" width="36" :reserve-selection="true" fixed="left" />
    <el-table-column type="expand" width="32" fixed="left">
      <template #default="{ row }">
        <div class="site-expand">
          <div class="exp-cell"><span>站点定义</span><b>{{ siteConfig(row.site_id)?.definition || '-' }}</b></div>
          <div class="exp-cell"><span>平台</span><b>{{ siteConfig(row.site_id)?.platform || '-' }}</b></div>
          <div class="exp-cell exp-address"><span>地址</span><b>{{ siteConfig(row.site_id)?.base_url || '-' }}</b></div>
          <div class="exp-cell">
            <span>配置状态</span>
            <b class="config-flags">
              <i :class="['config-state', siteConfig(row.site_id)?.is_active ? 'is-active' : 'is-disabled']">
                {{ siteConfig(row.site_id)?.is_active ? '启用' : '停用' }}
              </i>
              <i v-if="cookieAlerts.includes(row.site_name)" class="config-state is-warning">Cookie 失效</i>
            </b>
          </div>
          <div class="exp-cell"><span>用户 ID</span><b>{{ row.user_id || '-' }}</b></div>
          <div class="exp-cell"><span>邮箱</span><b>{{ row.email || '-' }}</b></div>
          <div class="exp-cell"><span>真实上传</span><b>{{ row.true_uploaded > 0 ? fmtBytes(row.true_uploaded) : '-' }}</b></div>
          <div class="exp-cell"><span>真实下载</span><b>{{ row.true_downloaded > 0 ? fmtBytes(row.true_downloaded) : '-' }}</b></div>
          <div class="exp-cell"><span>唯一作品组</span><b>{{ fmtNumber(row.unique_groups) }}</b></div>
          <div class="exp-cell"><span>Perfect FLAC</span><b>{{ fmtNumber(row.perfect_flac) }}</b></div>
          <div class="exp-cell"><span>已邀请</span><b>{{ fmtNumber(row.invited) }}</b></div>
          <div class="exp-cell"><span>做种积分</span><b>{{ fmtNumber(row.seeding_bonus, 2) }}</b></div>
          <div class="exp-cell"><span>做种积分/h</span><b>{{ fmtNumber(row.seeding_bonus_per_hour, 2) }}</b></div>
          <div class="exp-cell"><span>抢救积分/h</span><b>{{ fmtNumber(row.rescue_seeding_bonus_per_hour, 2) }}</b></div>
          <div class="exp-cell"><span>认领数 / 上限</span><b>{{ row.claim_count || 0 }} / {{ row.max_claim || 0 }}</b></div>
        </div>
      </template>
    </el-table-column>

    <el-table-column v-if="layout === 'compact'" label="站点 / 状态" width="145" fixed="left">
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
      label="站点"
      min-width="160"
      fixed="left"
      sortable="custom"
    >
      <template #default="{ row }">
        <button type="button" class="site-identity site-name-btn" @click="emit('detail', row as SiteStatItem)">
          <SiteFavicon :name="row.site_name" :base-url="siteBaseUrl(row.site_id)" :size="26" />
          <span>
            <b>{{ row.site_name }}</b>
            <small>{{ siteBaseUrl(row.site_id).replace(/^https?:\/\//, '').replace(/\/$/, '') || '未配置地址' }}</small>
          </span>
        </button>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout !== 'compact'"
      prop="status"
      label="状态"
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
    <el-table-column v-if="layout === 'compact'" label="用户 / 等级" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>用户</span><b :title="row.username">{{ row.username || '-' }}</b></div>
          <div class="stacked-line"><span>等级</span><b :title="row.level">{{ row.level || '-' }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-else
      prop="username"
      label="用户名"
      :min-width="layout === 'wide' ? 90 : 70"
      show-overflow-tooltip
      sortable="custom"
    />
    <el-table-column
      v-if="layout !== 'compact'"
      prop="level"
      label="等级"
      :min-width="layout === 'wide' ? 72 : 64"
      show-overflow-tooltip
      sortable="custom"
    >
      <template #default="{ row }"><span class="level-tag">{{ row.level || '-' }}</span></template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="uploaded" label="上传" min-width="96" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtBytes(row.uploaded) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="downloaded" label="下载" min-width="96" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtBytes(row.downloaded) }}</template>
    </el-table-column>
    <el-table-column v-else label="上传 / 下载" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>上传</span><b>{{ fmtBytes(row.uploaded) }}</b></div>
          <div class="stacked-line"><span>下载</span><b>{{ fmtBytes(row.downloaded) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="true_ratio" label="真实分享率" min-width="82" align="right" sortable="custom">
      <template #default="{ row }"><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="ratio" label="站点分享率" min-width="82" align="right" sortable="custom">
      <template #default="{ row }"><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></template>
    </el-table-column>
    <el-table-column v-else label="分享率" min-width="90">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>真实</span><b :class="row.true_ratio >= 1 ? 'ratio-good' : row.true_ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.true_ratio) }}</b></div>
          <div class="stacked-line"><span>站点</span><b :class="row.ratio >= 1 ? 'ratio-good' : row.ratio > 0 ? 'ratio-risk' : ''">{{ fmtRatio(row.ratio) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" label="做种 / 体积" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>做种</span><b>{{ fmtNumber(row.seeding) }}</b></div>
          <div class="stacked-line"><span>体积</span><b>{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-else prop="seeding" label="做种" :min-width="layout === 'wide' ? 50 : 44" align="right" sortable="custom" />
    <el-table-column
      v-if="layout !== 'compact'"
      prop="seeding_size"
      label="做种体积"
      :min-width="layout === 'wide' ? 90 : 80"
      align="right"
      sortable="custom"
    >
      <template #default="{ row }">{{ row.seeding_size > 0 ? fmtBytes(row.seeding_size) : '-' }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'compact'" label="发布 / 加入" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>发布</span><b>{{ fmtNumber(row.uploads) }}</b></div>
          <div class="stacked-line"><span>加入</span><b>{{ fmtDate(row.join_date) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column v-else prop="uploads" label="发布" :min-width="layout === 'wide' ? 50 : 44" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.uploads) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="bonus" label="魔力" min-width="100" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.bonus, 2) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="bonus_per_hour" label="时魔/h" min-width="72" align="right" sortable="custom">
      <template #default="{ row }">{{ fmtNumber(row.bonus_per_hour, 2) }}</template>
    </el-table-column>
    <el-table-column v-else label="魔力 / 时魔" min-width="100">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>魔力</span><b>{{ fmtNumber(row.bonus, 2) }}</b></div>
          <div class="stacked-line"><span>时魔/h</span><b>{{ fmtNumber(row.bonus_per_hour, 2) }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column
      v-if="layout !== 'compact'"
      prop="join_date"
      label="加入"
      :min-width="layout === 'wide' ? 88 : 80"
      show-overflow-tooltip
      sortable="custom"
    >
      <template #default="{ row }">{{ fmtDate(row.join_date) }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="last_access_at" label="最后访问" min-width="135" sortable="custom">
      <template #default="{ row }">{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</template>
    </el-table-column>
    <el-table-column v-if="layout === 'wide'" prop="updated_at" label="更新时间" min-width="135" sortable="custom">
      <template #default="{ row }">{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</template>
    </el-table-column>
    <el-table-column v-else label="访问 / 更新" min-width="140">
      <template #default="{ row }">
        <div class="stacked-stat">
          <div class="stacked-line"><span>访问</span><b>{{ row.last_access_at ? fmtTime(row.last_access_at) : '-' }}</b></div>
          <div class="stacked-line"><span>更新</span><b>{{ row.updated_at ? fmtTime(row.updated_at) : '-' }}</b></div>
        </div>
      </template>
    </el-table-column>
    <el-table-column label="操作" :min-width="layout === 'wide' ? 68 : 60">
      <template #default="{ row }">
        <div class="row-actions">
          <button class="row-act" title="用户历史数据" @click="emit('history', row as SiteStatItem)">
            <GlassIcon name="history" from="#2563eb" to="#0891b2" :size="16" />
          </button>
          <el-dropdown trigger="click" @command="emit('command', $event, row as SiteStatItem)">
            <button class="row-act" title="站点操作">
              <GlassIcon name="settings" from="#475569" to="#2563eb" :size="16" />
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="detail">站点详情</el-dropdown-item>
                <el-dropdown-item command="refresh" :disabled="refreshing.has(row.site_id)">
                  {{ refreshing.has(row.site_id) ? '刷新中…' : '刷新统计' }}
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="siteConfig(row.site_id)?.platform === 'nexusphp'"
                  command="checkin"
                  :disabled="checkiningSiteId === row.site_id"
                >
                  {{ checkiningSiteId === row.site_id ? '签到中…' : '签到' }}
                </el-dropdown-item>
                <el-dropdown-item
                  v-if="siteConfig(row.site_id)"
                  command="test"
                  :disabled="testingSiteId === row.site_id"
                >
                  {{ testingSiteId === row.site_id ? '测试中…' : '连接测试' }}
                </el-dropdown-item>
                <el-dropdown-item v-if="siteConfig(row.site_id)" command="edit">编辑配置</el-dropdown-item>
                <el-dropdown-item v-if="canDelete && siteConfig(row.site_id)" command="remove" divided>删除站点</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </template>
    </el-table-column>
    <template #empty>
      <div class="matrix-empty">
        <strong>暂无站点</strong>
        <span>添加 PT 站点并配置 Cookie 或 API Key 后即可查看运营统计。</span>
        <el-button type="primary" size="small" @click="$emit('create')">添加站点</el-button>
      </div>
    </template>
  </el-table>
</template>

<style scoped src="./siteOperationsMatrix.css"></style>
