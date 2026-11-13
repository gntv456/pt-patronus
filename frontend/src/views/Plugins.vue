<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useQueryState } from '@/composables/useUrlState'
import { useFeedback } from '@/composables/useFeedback'
import { Delete, Document, MoreFilled, Refresh, Search, Setting, VideoPlay } from '@element-plus/icons-vue'
import { pluginApi, type PluginInfo } from '@/api/plugin'
import PluginConfigDialog from './PluginConfigDialog.vue'
import PluginActionDialog from './PluginActionDialog.vue'
import PluginAuditDialog from './PluginAuditDialog.vue'
import PluginLogsDialog from './PluginLogsDialog.vue'
import PluginMarketSection from './PluginMarketSection.vue'
import { usePlugins } from './usePlugins'
import { usePluginTrust } from './usePluginTrust'
import { runtimeState, runtimeType, runtimeLabel, confirmPermissions } from './pluginsHelpers'
import { useAdminTabs } from '@/composables/useAdminTabs'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]

const router = useRouter()
const { t } = useI18n()
// 插件市场的读侧是 GET /plugins/market（RequireAdmin）：非管理员不给这个 Tab，
// 也就不会一点就吃 403、把市场区画成空白。
const { canSeeTab } = useAdminTabs(['market'])
const feedback = useFeedback()
// Tab / 筛选 / 搜索写回 URL query，刷新与分享可恢复。
const tabQuery = useQueryState('tab', 'installed')
const filterQuery = useQueryState('filter', 'all')
const searchQuery = useQueryState('q', '')

const {
  plugins,
  logs,
  loading,
  logsLoading,
  filter,
  logPlugin,
  activeTab,
  marketSources,
  marketSourceVisible,
  marketSourceDraft,
  marketSourceOfficial,
  market,
  marketLoading,
  installing,
  logsVisible,
  configVisible,
  configTarget,
  configForm,
  configExpanded,
  savingConfig,
  actionVisible,
  actionSpec,
  actionInputForm,
  actionInputText,
  actionOutput,
  actionRunning,
  auditVisible,
  auditTarget,
  auditLogs,
  auditLogsNotice,
  auditReleases,
  auditLoading,
  rollbacking,
  marketCategory,
  marketCharts,
  favoritePlugins,
  marketView,
  toggleFavorite,
  instancesVisible,
  instancePluginId,
  instances,
  singleInstance,
  openInstances,
  createInstance,
  toggleInstance,
  removeInstance,
  moveInstance,
  pluginSearch,
  marketSearch,
  filteredPlugins,
  enabledCount,
  actionHasSchema,
  marketCategories,
  filteredMarketPlugins,
  configTitledGroups,
  configUntitledFields,
  load,
  loadLogs,
  openLogs,
  openAudit,
  triggerRollback,
  loadMarket,
  openMarketSource,
  addMarketSourceRow,
  removeMarketSourceRow,
  restoreOfficialSource,
  saveMarketSources,
  installMarketPlugin,
  reloadPlugin,
  uninstallPlugin,
  togglePlugin,
  openConfig,
  saveConfig,
  openAction,
  handleOpenPluginView,
  runAction,
} = usePlugins({
  openPluginView: (p: PluginInfo, view: PluginViewContribution) => {
    router.push({ name: 'plugin-view', params: { pluginId: p.id, viewId: view.id }, query: { tab: tabQuery.value } })
  },
})

const {
  trustVisible,
  trustLoading,
  trustSaving,
  trustOfficialKey,
  trustKeysText,
  trustAllowUnsigned,
  openTrust,
  saveTrust,
} = usePluginTrust()

const pluginColors = [
  'var(--chart-1)',
  'var(--chart-2)',
  'var(--chart-3)',
  'var(--chart-4)',
  'var(--color-primary)',
  'var(--color-success)',
  'var(--color-warning)',
  'var(--color-danger)',
]

watch(
  tabQuery,
  (v) => {
    if (v === 'market' && !canSeeTab('market')) return
    if (v === 'installed' || v === 'market') activeTab.value = v
  },
  { immediate: true },
)
watch(activeTab, (v) => {
  if (v !== tabQuery.value) tabQuery.value = v
})
watch(
  filterQuery,
  (v) => {
    if (v === 'all' || v === 'enabled' || v === 'available') filter.value = v as 'all' | 'enabled' | 'available'
  },
  { immediate: true },
)
watch(filter, (v) => {
  if (v !== filterQuery.value) filterQuery.value = v
})
watch(
  searchQuery,
  (v) => {
    if (v !== pluginSearch.value) pluginSearch.value = v
  },
  { immediate: true },
)
watch(pluginSearch, (v) => {
  if (v !== searchQuery.value) searchQuery.value = v
})

// 批量启停：勾选后统一操作，避免逐个确认 / 逐个刷新。
const selectedIds = ref<string[]>([])
const selectedPlugins = computed(() => plugins.value.filter((p) => selectedIds.value.includes(p.id)))
const allSelected = computed(
  () => filteredPlugins.value.length > 0 && filteredPlugins.value.every((p) => selectedIds.value.includes(p.id)),
)
function toggleSelect(id: string) {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}
function toggleSelectAll() {
  selectedIds.value = allSelected.value ? [] : filteredPlugins.value.map((p) => p.id)
}
async function batchToggle(enabled: boolean) {
  const targets = selectedPlugins.value.filter((p) => p.can_enable && p.enabled !== enabled)
  if (!targets.length) {
    feedback.toast(t('subscriptions.batch.selectFirst'), 'warning')
    return
  }
  const perms = Array.from(new Set(targets.flatMap((p) => p.permissions || [])))
  if (enabled) {
    if (!(await confirmPermissions(t('common.enabled'), perms))) return
  } else {
    const ok = await feedback.confirm({
      title: t('common.confirm'),
      detail: t('subscriptions.batch.selected', { n: targets.length }) + '，' + t('common.disabled'),
      danger: true,
    })
    if (!ok) return
  }
  try {
    for (const p of targets) {
      if (enabled) await pluginApi.enable(p.id, p.permissions || [])
      else await pluginApi.disable(p.id)
    }
    await load()
    feedback.toast(t('common.actionOk'))
  } catch (e: unknown) {
    feedback.error((e as Error).message || t('common.actionFailed'))
  } finally {
    selectedIds.value = []
  }
}

function avatarColor(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) hash = name.charCodeAt(i) + ((hash << 5) - hash)
  return pluginColors[Math.abs(hash) % pluginColors.length]
}

function pluginInitial(name: string): string {
  return name ? name.charAt(0).toUpperCase() : '?'
}
</script>

<template>
  <div class="page">
    <div class="head">
      <nav class="nav-tabs" role="tablist" :aria-label="t('plugins.tabInstalled')">
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'installed'"
          class="nav-tab"
          :class="{ active: activeTab === 'installed' }"
          @click="activeTab = 'installed'"
        >
          {{ t('plugins.tabInstalled') }} <span class="nav-count">{{ plugins.length }}</span>
        </button>
        <button
          v-if="canSeeTab('market')"
          type="button"
          role="tab"
          :aria-selected="activeTab === 'market'"
          class="nav-tab"
          :class="{ active: activeTab === 'market' }"
          @click="activeTab = 'market'"
        >
          {{ t('plugins.tabMarket') }}
        </button>
      </nav>
      <div class="head-actions">
        <template v-if="activeTab === 'installed'">
          <el-input
            v-model="pluginSearch"
            :placeholder="t('plugins.searchPh')"
            :prefix-icon="Search"
            size="small"
            clearable
            style="width: 160px"
          />
          <el-select v-model="filter" size="small" style="width: 100px">
            <el-option :label="t('cloud.all')" value="all" />
            <el-option :label="t('plugins.fEnabled')" value="enabled" />
            <el-option :label="t('plugins.fAvailable')" value="available" />
          </el-select>
          <el-button :icon="Document" size="small" @click="openLogs">{{ t('plugins.logs') }}</el-button>
          <el-button :icon="Setting" size="small" @click="openTrust">{{ t('plugins.trust') }}</el-button>
        </template>
        <template v-else>
          <el-input
            v-model="marketSearch"
            :placeholder="t('plugins.searchMarketPh')"
            :prefix-icon="Search"
            size="small"
            clearable
            style="width: 180px"
          />
          <el-button :icon="Setting" size="small" @click="openMarketSource">{{ t('plugins.repo') }}</el-button>
          <el-button :icon="Refresh" size="small" :loading="marketLoading" @click="loadMarket">{{
            t('plugins.refreshMarket')
          }}</el-button>
        </template>
        <el-button v-if="activeTab !== 'market'" :icon="Refresh" size="small" :loading="loading" @click="load">{{
          t('common.refresh')
        }}</el-button>
      </div>
    </div>

    <div
      v-if="activeTab === 'installed' && selectedIds.length"
      class="batch-bar solid-card"
      role="region"
      :aria-label="t('subscriptions.batch.button')"
    >
      <el-checkbox
        :model-value="allSelected"
        :indeterminate="selectedIds.length > 0 && !allSelected"
        @change="toggleSelectAll"
        >{{ t('subscriptions.batch.selectAll') }}</el-checkbox
      >
      <span class="batch-count">{{ t('subscriptions.batch.selected', { n: selectedIds.length }) }}</span>
      <el-button size="small" @click="batchToggle(true)">{{ t('common.enabled') }}</el-button>
      <el-button size="small" @click="batchToggle(false)">{{ t('common.disabled') }}</el-button>
      <el-button size="small" plain @click="selectedIds = []">{{ t('subscriptions.batch.exit') }}</el-button>
    </div>

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card solid-card solid-card-hover">
        <div class="plugin-card-top">
          <el-checkbox
            :model-value="selectedIds.includes(p.id)"
            :aria-label="p.name"
            @change="toggleSelect(p.id)"
          />
          <div class="plugin-avatar" :style="{ background: avatarColor(p.name) }">{{ pluginInitial(p.name) }}</div>
          <div class="plugin-title-area">
            <div class="plugin-name-row">
              <span class="plugin-name">{{ p.name }}</span>
              <span v-if="p.built_in" class="badge badge-neutral">{{ t('plugins.builtIn') }}</span>
              <span class="badge" :class="p.enabled ? 'badge-success' : 'badge-muted'">{{
                p.enabled ? t('plugins.running') : t('plugins.notEnabled')
              }}</span>
              <span v-if="!p.can_enable" class="badge badge-danger">{{ t('plugins.depBlocked') }}</span>
              <span v-if="p.runtime_status" class="badge" :class="'badge-' + runtimeType(runtimeState(p))">{{
                runtimeLabel(runtimeState(p))
              }}</span>
            </div>
            <div class="plugin-sub">v{{ p.version }} · {{ p.author }}</div>
          </div>
          <el-switch :model-value="p.enabled" :disabled="!p.can_enable" size="small" @change="togglePlugin(p)" />
        </div>

        <div v-if="p.description" class="plugin-desc">{{ p.description }}</div>
        <div v-if="p.last_error" class="plugin-error">{{ p.last_error }}</div>

        <div class="plugin-card-foot">
          <el-button size="small" type="primary" :icon="Setting" @click="openConfig(p)">{{
            t('plugins.config')
          }}</el-button>
          <div class="foot-spacer" />
          <el-dropdown trigger="click">
            <el-button size="small" circle plain class="more-btn" :aria-label="t('mediadetail.more')"
              ><el-icon><MoreFilled /></el-icon
            ></el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-if="!p.built_in" :icon="Refresh" @click="reloadPlugin(p)">{{
                  t('plugins.reload')
                }}</el-dropdown-item>
                <el-dropdown-item :icon="Document" @click="openAudit(p)">{{ t('plugins.audit') }}</el-dropdown-item>
                <el-dropdown-item
                  v-for="v in p.contributes?.views || []"
                  :key="v.id"
                  :icon="Document"
                  divided
                  @click="handleOpenPluginView(p, v)"
                >
                  {{ v.title }}
                </el-dropdown-item>
                <el-dropdown-item
                  v-for="a in p.actions || []"
                  :key="a.name"
                  :icon="VideoPlay"
                  @click="openAction(p, a)"
                >
                  {{ a.label }}
                </el-dropdown-item>
                <el-dropdown-item v-if="!p.built_in" :icon="Delete" divided @click="uninstallPlugin(p)">{{
                  t('plugins.uninstall')
                }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </div>
    </div>

    <PluginMarketSection
      v-if="activeTab === 'market'"
      :market="market"
      :loading="marketLoading"
      :installing="installing"
      :market-sources="marketSources"
      :market-source-visible="marketSourceVisible"
      :market-source-draft="marketSourceDraft"
      :market-source-official="marketSourceOfficial"
      :market-category="marketCategory"
      :market-categories="marketCategories"
      :filtered-market-plugins="filteredMarketPlugins"
      :avatar-color="avatarColor"
      :plugin-initial="pluginInitial"
      :market-view="marketView"
      :market-charts="marketCharts"
      :favorite-plugins="favoritePlugins"
      :instances-visible="instancesVisible"
      :instance-plugin-id="instancePluginId"
      :instances="instances"
      :single-instance="singleInstance"
      @refresh="loadMarket"
      @install="installMarketPlugin"
      @open-source="openMarketSource"
      @update:market-source-visible="marketSourceVisible = $event"
      @update:market-source-draft="marketSourceDraft = $event"
      @update:market-category="marketCategory = $event"
      @add-source-row="addMarketSourceRow"
      @remove-source-row="removeMarketSourceRow"
      @restore-official="restoreOfficialSource"
      @save-sources="saveMarketSources"
      @update:market-view="marketView = $event"
      @toggle-favorite="toggleFavorite"
      @open-instances="openInstances"
      @update:instances-visible="instancesVisible = $event"
      @create-instance="createInstance"
      @toggle-instance="toggleInstance"
      @remove-instance="removeInstance"
      @move-instance="moveInstance"
    />

    <el-dialog v-model="trustVisible" :title="t('plugins.trustTitle')" width="560px">
      <div v-loading="trustLoading">
        <div style="margin-bottom: 14px">
          <div class="text-muted" style="font-size: var(--fs-body); margin-bottom: 4px">
            {{ t('plugins.officialKey') }}
          </div>
          <el-input :model-value="trustOfficialKey" readonly type="textarea" :rows="2" />
        </div>
        <div style="margin-bottom: 14px">
          <div class="text-muted" style="font-size: var(--fs-body); margin-bottom: 4px">
            {{ t('plugins.extraKeys') }}
          </div>
          <el-input v-model="trustKeysText" type="textarea" :rows="4" :placeholder="t('plugins.keysPh')" />
        </div>
        <div style="margin-bottom: 8px">
          <el-switch v-model="trustAllowUnsigned" />
          <span style="margin-left: 8px">{{ t('plugins.allowUnsigned') }}</span>
        </div>
        <div class="text-muted" style="font-size: var(--fs-xs); margin-top: 8px">
          {{ t('plugins.trustTip') }}
        </div>
      </div>
      <template #footer>
        <el-button @click="trustVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="trustSaving" @click="saveTrust">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <PluginLogsDialog
      :visible="logsVisible"
      :plugins="plugins"
      :logs="logs"
      :log-plugin="logPlugin"
      :loading="logsLoading"
      @update:visible="logsVisible = $event"
      @update:log-plugin="logPlugin = $event"
      @refresh="loadLogs"
    />

    <PluginAuditDialog
      :visible="auditVisible"
      :target="auditTarget"
      :logs="auditLogs"
      :releases="auditReleases"
      :loading="auditLoading"
      :rollbacking="rollbacking"
      :notice="auditLogsNotice"
      @update:visible="auditVisible = $event"
      @refresh="auditTarget && openAudit(auditTarget)"
      @rollback="triggerRollback"
    />

    <PluginConfigDialog
      :visible="configVisible"
      :target="configTarget"
      :form="configForm"
      :expanded="configExpanded"
      :titled-groups="configTitledGroups"
      :untitled-fields="configUntitledFields"
      :saving="savingConfig"
      @update:visible="configVisible = $event"
      @update:form="configForm = $event"
      @update:expanded="configExpanded = $event"
      @save="saveConfig"
    />

    <PluginActionDialog
      :visible="actionVisible"
      :spec="actionSpec"
      :input-form="actionInputForm"
      :input-text="actionInputText"
      :output="actionOutput"
      :running="actionRunning"
      :has-schema="actionHasSchema"
      @update:visible="actionVisible = $event"
      @update:input-form="actionInputForm = $event"
      @update:input-text="actionInputText = $event"
      @run="runAction"
    />
  </div>
</template>

<style scoped src="./Plugins.css"></style>
