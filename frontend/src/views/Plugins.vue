<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Delete, Document, MoreFilled, Refresh, Search, Setting, VideoPlay } from '@element-plus/icons-vue'
import type { PluginInfo } from '@/api/plugin'
import PluginConfigDialog from './PluginConfigDialog.vue'
import PluginActionDialog from './PluginActionDialog.vue'
import PluginAuditDialog from './PluginAuditDialog.vue'
import PluginLogsDialog from './PluginLogsDialog.vue'
import PluginMarketSection from './PluginMarketSection.vue'
import { usePlugins } from './usePlugins'
import { usePluginTrust } from './usePluginTrust'
import { runtimeState, runtimeType, runtimeLabel } from './pluginsHelpers'
import { useAdminTabs } from '@/composables/useAdminTabs'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]

const router = useRouter()
const { t } = useI18n()
// 插件市场的读侧是 GET /plugins/market（RequireAdmin）：非管理员不给这个 Tab，
// 也就不会一点就吃 403、把市场区画成空白。
const { canSeeTab } = useAdminTabs(['market'])

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
    router.push({ name: 'plugin-view', params: { pluginId: p.id, viewId: view.id } })
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

const pluginColors = ['var(--pt-primary)', '#0ea5e9', '#22c55e', '#f59e0b', '#ef4444', '#ec4899', '#14b8a6', '#8b5cf6']

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
      <nav class="nav-tabs">
        <button class="nav-tab" :class="{ active: activeTab === 'installed' }" @click="activeTab = 'installed'">
          {{ t('plugins.tabInstalled') }} <span class="nav-count">{{ plugins.length }}</span>
        </button>
        <button
          v-if="canSeeTab('market')"
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

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card solid-card solid-card-hover">
        <div class="plugin-card-top">
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
            <el-button size="small" circle plain class="more-btn"
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
