<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Delete, Document, MoreFilled, Refresh, Search, Setting, VideoPlay } from '@element-plus/icons-vue'
import type { PluginInfo } from '@/api/plugin'
import PluginConfigDialog from './PluginConfigDialog.vue'
import PluginActionDialog from './PluginActionDialog.vue'
import PluginAuditDialog from './PluginAuditDialog.vue'
import PluginLogsDialog from './PluginLogsDialog.vue'
import PluginMarketSection from './PluginMarketSection.vue'
import { usePlugins } from './usePlugins'
import { usePluginTrust } from './usePluginTrust'
import {
  runtimeState,
  runtimeType,
  runtimeLabel,
} from './pluginsHelpers'

type PluginViewContribution = NonNullable<NonNullable<PluginInfo['contributes']>['views']>[number]

const router = useRouter()

const {
  plugins, logs, loading, logsLoading,
  filter, logPlugin, activeTab,
  marketSources, marketSourceVisible, marketSourceDraft, marketSourceOfficial,
  market, marketLoading, installing,
  logsVisible,
  configVisible, configTarget, configForm, configExpanded, savingConfig,
  actionVisible, actionSpec, actionInputForm, actionInputText, actionOutput, actionRunning,
  auditVisible, auditTarget, auditLogs, auditReleases, auditLoading, rollbacking,
  marketCategory,
  marketCharts, favoritePlugins, marketView, toggleFavorite,
  instancesVisible, instancePluginId, instances, singleInstance,
  openInstances, createInstance, toggleInstance, removeInstance, moveInstance,
  pluginSearch, marketSearch,
  filteredPlugins, enabledCount, actionHasSchema,
  marketCategories, filteredMarketPlugins,
  configTitledGroups, configUntitledFields,
  load, loadLogs, openLogs, openAudit, triggerRollback,
  loadMarket, openMarketSource, addMarketSourceRow, removeMarketSourceRow,
  restoreOfficialSource, saveMarketSources, installMarketPlugin,
  reloadPlugin, uninstallPlugin, togglePlugin,
  openConfig, saveConfig, openAction, handleOpenPluginView, runAction,
} = usePlugins({
  openPluginView: (p: PluginInfo, view: PluginViewContribution) => {
    router.push({ name: 'plugin-view', params: { pluginId: p.id, viewId: view.id } })
  },
})

const {
  trustVisible, trustLoading, trustSaving,
  trustOfficialKey, trustKeysText, trustAllowUnsigned,
  openTrust, saveTrust,
} = usePluginTrust()

const pluginColors = ['#6c5ce7', '#0ea5e9', '#22c55e', '#f59e0b', '#ef4444', '#ec4899', '#14b8a6', '#8b5cf6']

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
          已安装 <span class="nav-count">{{ plugins.length }}</span>
        </button>
        <button class="nav-tab" :class="{ active: activeTab === 'market' }" @click="activeTab = 'market'">
          插件市场
        </button>
      </nav>
      <div class="head-actions">
        <template v-if="activeTab === 'installed'">
          <el-input v-model="pluginSearch" placeholder="搜索插件" :prefix-icon="Search" size="small" clearable style="width: 160px;" />
          <el-select v-model="filter" size="small" style="width: 100px;">
            <el-option label="全部" value="all" />
            <el-option label="已启用" value="enabled" />
            <el-option label="可启用" value="available" />
          </el-select>
          <el-button :icon="Document" size="small" @click="openLogs">日志</el-button>
          <el-button :icon="Setting" size="small" @click="openTrust">信任</el-button>
        </template>
        <template v-else>
          <el-input v-model="marketSearch" placeholder="搜索市场插件" :prefix-icon="Search" size="small" clearable style="width: 180px;" />
          <el-button :icon="Setting" size="small" @click="openMarketSource">仓库</el-button>
          <el-button :icon="Refresh" size="small" :loading="marketLoading" @click="loadMarket">刷新市场</el-button>
        </template>
        <el-button v-if="activeTab !== 'market'" :icon="Refresh" size="small" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card solid-card solid-card-hover">
        <div class="plugin-card-top">
          <div class="plugin-avatar" :style="{ background: avatarColor(p.name) }">{{ pluginInitial(p.name) }}</div>
          <div class="plugin-title-area">
            <div class="plugin-name-row">
              <span class="plugin-name">{{ p.name }}</span>
              <span v-if="p.built_in" class="badge badge-neutral">内置</span>
              <span class="badge" :class="p.enabled ? 'badge-success' : 'badge-muted'">{{ p.enabled ? '运行中' : '未启用' }}</span>
              <span v-if="!p.can_enable" class="badge badge-danger">依赖阻塞</span>
              <span v-if="p.runtime_status" class="badge" :class="'badge-' + runtimeType(runtimeState(p))">{{ runtimeLabel(runtimeState(p)) }}</span>
            </div>
            <div class="plugin-sub">v{{ p.version }} · {{ p.author }}</div>
          </div>
          <el-switch :model-value="p.enabled" :disabled="!p.can_enable" size="small" @change="togglePlugin(p)" />
        </div>

        <div v-if="p.description" class="plugin-desc">{{ p.description }}</div>
        <div v-if="p.last_error" class="plugin-error">{{ p.last_error }}</div>

        <div class="plugin-card-foot">
          <el-button size="small" type="primary" :icon="Setting" @click="openConfig(p)">配置</el-button>
          <div class="foot-spacer" />
          <el-dropdown trigger="click">
            <el-button size="small" circle plain class="more-btn"><el-icon><MoreFilled /></el-icon></el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item :icon="Refresh" @click="reloadPlugin(p)">重载</el-dropdown-item>
                <el-dropdown-item :icon="Document" @click="openAudit(p)">审计</el-dropdown-item>
                <el-dropdown-item v-for="v in p.contributes?.views || []" :key="v.id" :icon="Document" divided @click="handleOpenPluginView(p, v)">
                  {{ v.title }}
                </el-dropdown-item>
                <el-dropdown-item v-for="a in p.actions || []" :key="a.name" :icon="VideoPlay" @click="openAction(p, a)">
                  {{ a.label }}
                </el-dropdown-item>
                <el-dropdown-item v-if="!p.built_in" :icon="Delete" divided @click="uninstallPlugin(p)">卸载</el-dropdown-item>
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

    <el-dialog v-model="trustVisible" title="插件签名信任配置" width="560px">
      <div v-loading="trustLoading">
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: var(--fs-body); margin-bottom: 4px;">官方公钥（只读，始终受信）</div>
          <el-input :model-value="trustOfficialKey" readonly type="textarea" :rows="2" />
        </div>
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: var(--fs-body); margin-bottom: 4px;">追加受信公钥（每行一个 base64 ed25519）</div>
          <el-input v-model="trustKeysText" type="textarea" :rows="4" placeholder="留空则仅信任官方公钥" />
        </div>
        <div style="margin-bottom: 8px;">
          <el-switch v-model="trustAllowUnsigned" />
          <span style="margin-left: 8px;">允许安装未签名插件（不推荐）</span>
        </div>
        <div class="text-muted" style="font-size: var(--fs-xs); margin-top: 8px;">
          市场插件安装时按此处公钥校验签名；未签名插件仅在开启上方开关后允许安装。
        </div>
      </div>
      <template #footer>
        <el-button @click="trustVisible = false">取消</el-button>
        <el-button type="primary" :loading="trustSaving" @click="saveTrust">保存</el-button>
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