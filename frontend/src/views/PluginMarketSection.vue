<script setup lang="ts">
import { Delete, Download, Plus, Refresh, Setting } from '@element-plus/icons-vue'
import type { PluginMarketIndex } from '@/api/plugin'
import {
  shortSource,
  canInstallMarketItem,
  compatibilityType,
  compatibilityLabel,
  marketInstallLabel,
  trustType,
  trustLabel,
} from './pluginsHelpers'

defineProps<{
  market: PluginMarketIndex | null
  loading: boolean
  installing: string
  marketSources: string[]
  marketSourceVisible: boolean
  marketSourceDraft: string[]
  marketSourceOfficial: string
  marketCategory: string
  marketCategories: string[]
  filteredMarketPlugins: PluginMarketIndex['plugins']
}>()

const emit = defineEmits<{
  (e: 'refresh'): void
  (e: 'install', id: string): void
  (e: 'openSource'): void
  (e: 'update:marketSourceVisible', v: boolean): void
  (e: 'update:marketSourceDraft', v: string[]): void
  (e: 'update:marketCategory', v: string): void
  (e: 'addSourceRow'): void
  (e: 'removeSourceRow', index: number): void
  (e: 'restoreOfficial'): void
  (e: 'saveSources'): void
}>()
</script>

<template>
  <div class="market glass">
    <div v-if="market" class="market-host">
      当前宿主：App {{ market.app_version || '-' }} / API {{ market.api_version || '-' }} / 能力 {{ (market.host_capabilities || []).length }}
      <span v-if="marketSources.length" class="market-source-count">· 已配置 {{ marketSources.length }} 个市场源</span>
    </div>
    <el-radio-group
      v-if="marketCategories.length"
      :model-value="marketCategory"
      size="small"
      style="margin: 8px 0 12px;"
      @update:model-value="emit('update:marketCategory', $event as string)"
    >
      <el-radio-button label="">全部 {{ (market?.plugins || []).length }}</el-radio-button>
      <el-radio-button v-for="c in marketCategories" :key="c" :label="c">{{ c }}</el-radio-button>
    </el-radio-group>
    <div v-loading="loading" class="market-grid">
      <div
        v-for="item in filteredMarketPlugins"
        :key="item.id"
        class="market-card solid-card solid-card-hover"
      >
        <div class="market-card-head">
          <div class="market-name">{{ item.name }}</div>
          <el-tag size="small" effect="plain">v{{ item.version }}</el-tag>
        </div>
        <div class="market-desc">{{ item.description || '无说明' }}</div>
        <div class="market-card-meta">
          <el-tag size="small" :type="trustType(item.trust_level)">{{ trustLabel(item.trust_level) }}</el-tag>
          <el-tag v-if="item.category" size="small" effect="plain">{{ item.category }}</el-tag>
          <span class="meta">{{ item.publisher?.name || item.author || shortSource(item.source) || '未知' }}</span>
        </div>
        <div class="market-card-foot">
          <el-tag size="small" :type="compatibilityType(item)" effect="plain">{{ compatibilityLabel(item) }}</el-tag>
          <el-button
            size="small"
            type="primary"
            :icon="Download"
            :disabled="!canInstallMarketItem(item)"
            :loading="installing === item.id"
            @click="emit('install', item.id)"
          >
            {{ marketInstallLabel(item) }}
          </el-button>
        </div>
      </div>
      <el-empty v-if="!loading && !filteredMarketPlugins.length" description="暂无市场数据" />
    </div>
  </div>

  <el-dialog
    :model-value="marketSourceVisible"
    title="市场源管理"
    width="560px"
    @update:model-value="emit('update:marketSourceVisible', $event)"
  >
    <div class="market-source-tip">
      默认已内置官方仓库，可删除或追加第三方仓库。支持 GitHub 仓库地址（如 https://github.com/user/repo，可用 /tree/分支/目录 指定）或任意 plugin-market.json URL。列表顺序即优先级：刷新市场时按此处顺序聚合，相同插件 ID 先列者优先。
    </div>
    <div class="market-source-list">
      <div v-for="(_, i) in marketSourceDraft" :key="i" class="market-source-row">
        <el-tag v-if="marketSourceDraft[i] === marketSourceOfficial" type="warning" size="small" effect="light">官方</el-tag>
        <el-input
          :model-value="marketSourceDraft[i]"
          clearable
          :placeholder="i === 0 ? 'https://github.com/owner/repo' : 'https://github.com/owner/repo'"
          @update:model-value="emit('update:marketSourceDraft', marketSourceDraft.map((s, idx) => idx === i ? $event : s))"
        />
        <el-button :icon="Delete" circle plain @click="emit('removeSourceRow', i)" />
      </div>
    </div>
    <div class="market-source-add">
      <el-button :icon="Plus" plain @click="emit('addSourceRow')">添加源</el-button>
      <el-button v-if="marketSourceOfficial && !marketSourceDraft.includes(marketSourceOfficial)" plain @click="emit('restoreOfficial')">恢复官方源</el-button>
    </div>
    <template #footer>
      <el-button @click="emit('update:marketSourceVisible', false)">取消</el-button>
      <el-button type="primary" @click="emit('saveSources')">保存</el-button>
    </template>
  </el-dialog>
</template>
