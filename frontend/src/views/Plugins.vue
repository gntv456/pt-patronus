<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Bell, Delete, Document, Refresh, Setting, SwitchButton, VideoPlay } from '@element-plus/icons-vue'
import type { PluginInfo } from '@/api/plugin'
import PluginConfigDialog from './PluginConfigDialog.vue'
import PluginActionDialog from './PluginActionDialog.vue'
import PluginAuditDialog from './PluginAuditDialog.vue'
import PluginLogsDialog from './PluginLogsDialog.vue'
import PluginMarketSection from './PluginMarketSection.vue'
import { usePlugins } from './usePlugins'
import { usePluginTrust } from './usePluginTrust'
import {
  dependencySummary,
  optionalCapabilitySummary,
  runtimeState,
  runtimeType,
  runtimeLabel,
  fmtTime,
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
  filteredPlugins, enabledCount, actionHasSchema,
  marketCategories, filteredMarketPlugins,
  configTitledGroups, configUntitledFields,
  load, loadLogs, openLogs, openAudit, triggerRollback,
  loadMarket, openMarketSource, addMarketSourceRow, removeMarketSourceRow,
  restoreOfficialSource, saveMarketSources, installMarketPlugin,
  reloadPlugin, uninstallPlugin, togglePlugin,
  openConfig, saveConfig, openAction, handleOpenPluginView, runAction, publishTestEvent,
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
</script>

<template>
  <div class="page">
    <div class="head">
      <div>
        <h2 class="title">插件</h2>
        <span class="text-muted hint">内置插件、事件订阅、配置和动作执行</span>
      </div>
      <div class="head-actions">
        <el-radio-group v-model="filter" size="small">
          <el-radio-button label="all">全部 {{ plugins.length }}</el-radio-button>
          <el-radio-button label="enabled">已启用 {{ enabledCount }}</el-radio-button>
          <el-radio-button label="available">可启用</el-radio-button>
        </el-radio-group>
        <el-button :icon="Bell" @click="publishTestEvent">测试事件</el-button>
        <el-button :icon="Document" @click="openLogs">插件日志</el-button>
        <el-button :icon="Setting" @click="openTrust">信任配置</el-button>
        <template v-if="activeTab === 'market'">
          <el-button :icon="Setting" @click="openMarketSource">设置仓库</el-button>
          <el-button :icon="Refresh" :loading="marketLoading" @click="loadMarket">刷新市场</el-button>
        </template>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
    </div>

    <el-tabs v-model="activeTab" class="plugin-tabs">
      <el-tab-pane label="已安装" name="installed" />
      <el-tab-pane label="插件市场" name="market" />
    </el-tabs>

    <div v-if="activeTab === 'installed'" v-loading="loading" class="plugin-grid">
      <div v-for="p in filteredPlugins" :key="p.id" class="plugin-card solid-card solid-card-hover">
        <div class="plugin-top">
          <GlassIcon name="plugin" from="#0ea5e9" to="#22c55e" :size="40" />
          <div class="plugin-title">
            <div class="name-row">
              <span class="plugin-name">{{ p.name }}</span>
              <el-tag size="small" :type="p.enabled ? 'success' : 'info'">{{ p.enabled ? '运行中' : '未启用' }}</el-tag>
              <el-tag v-if="p.built_in" size="small">内置</el-tag>
              <el-tag v-if="!p.can_enable" size="small" type="danger">依赖阻塞</el-tag>
              <el-tag v-if="p.runtime_status" size="small" :type="runtimeType(runtimeState(p))">
                {{ runtimeLabel(runtimeState(p)) }}
              </el-tag>
            </div>
            <div class="meta">{{ p.id }} · v{{ p.version }} · {{ p.author }}</div>
          </div>
        </div>

        <div class="desc">{{ p.description }}</div>

        <div class="chips">
          <el-tag v-for="perm in p.permissions || []" :key="perm" size="small" effect="plain">{{ perm }}</el-tag>
        </div>

        <div class="detail-row">
          <span>事件</span>
          <span>{{ (p.events || []).map((e) => e.type).join(', ') || '-' }}</span>
        </div>
        <div class="detail-row">
          <span>上次事件</span>
          <span>{{ fmtTime(p.last_event_at) }}</span>
        </div>
        <div class="detail-row">
          <span>依赖</span>
          <span>{{ dependencySummary(p.dependencies_status) }}</span>
        </div>
        <div class="detail-row">
          <span>可选能力</span>
          <span>{{ optionalCapabilitySummary(p) }}</span>
        </div>
        <div v-if="p.last_error" class="error-line">{{ p.last_error }}</div>
        <div v-if="!p.can_enable && p.can_enable_reason" class="error-line">{{ p.can_enable_reason }}</div>

        <div class="actions">
          <el-button :icon="SwitchButton" :type="p.enabled ? 'warning' : 'primary'" @click="togglePlugin(p)">
            {{ p.enabled ? '停用' : '启用' }}
          </el-button>
          <el-button :icon="Setting" @click="openConfig(p)">配置</el-button>
          <el-button v-if="!p.built_in" :icon="Refresh" @click="reloadPlugin(p)">重载</el-button>
          <el-button v-if="!p.built_in" :icon="Delete" type="danger" plain @click="uninstallPlugin(p)">卸载</el-button>
          <el-button :icon="Document" plain @click="openAudit(p)">审计</el-button>
          <el-dropdown v-if="(p.contributes?.views || []).length" trigger="click">
            <el-button :icon="Document">Views</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="v in p.contributes?.views || []" :key="v.id" @click="handleOpenPluginView(p, v)">
                  {{ v.title }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
          <el-dropdown v-if="(p.actions || []).length" trigger="click">
            <el-button :icon="VideoPlay">动作</el-button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="a in p.actions || []" :key="a.name" @click="openAction(p, a)">
                  {{ a.label }}
                </el-dropdown-item>
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
    />

    <el-dialog v-model="trustVisible" title="插件签名信任配置" width="560px">
      <div v-loading="trustLoading">
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: 13px; margin-bottom: 4px;">官方公钥（只读，始终受信）</div>
          <el-input :model-value="trustOfficialKey" readonly type="textarea" :rows="2" />
        </div>
        <div style="margin-bottom: 14px;">
          <div class="text-muted" style="font-size: 13px; margin-bottom: 4px;">追加受信公钥（每行一个 base64 ed25519）</div>
          <el-input v-model="trustKeysText" type="textarea" :rows="4" placeholder="留空则仅信任官方公钥" />
        </div>
        <div style="margin-bottom: 8px;">
          <el-switch v-model="trustAllowUnsigned" />
          <span style="margin-left: 8px;">允许安装未签名插件（不推荐）</span>
        </div>
        <div class="text-muted" style="font-size: 12px; margin-top: 8px;">
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
