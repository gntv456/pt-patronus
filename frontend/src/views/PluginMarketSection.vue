<script setup lang="ts">
import { Delete, Download, Plus, Refresh, Setting, Star, StarFilled, Top, Bottom } from '@element-plus/icons-vue'
import type { PluginMarketIndex, PluginMarketItem, ChartRow, PluginInstance } from '@/api/plugin'
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
  avatarColor: (name: string) => string
  pluginInitial: (name: string) => string
  // 运营层（百鸟朝凤/百宝袋/分神/天序）
  marketView: 'all' | 'favorites' | 'charts'
  marketCharts: { disciples: ChartRow[]; newcomers: ChartRow[] } | null
  favoritePlugins: PluginMarketItem[]
  instancesVisible: boolean
  instancePluginId: string
  instances: PluginInstance[]
  singleInstance: boolean
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
  (e: 'update:marketView', v: 'all' | 'favorites' | 'charts'): void
  (e: 'toggleFavorite', item: PluginMarketItem): void
  (e: 'openInstances', id: string): void
  (e: 'update:instancesVisible', v: boolean): void
  (e: 'createInstance', key: string): void
  (e: 'toggleInstance', inst: PluginInstance): void
  (e: 'removeInstance', inst: PluginInstance): void
  (e: 'moveInstance', index: number, delta: -1 | 1): void
}>()

// 新分身名输入（分神对话框）。
const newInstanceKey = defineModel<string>('newInstanceKey', { default: '' })
</script>

<template>
  <div class="market-section">
    <div v-if="market" class="market-meta-bar">
      <span>App {{ market.app_version || '-' }} · API {{ market.api_version || '-' }} · 能力 {{ (market.host_capabilities || []).length }}</span>
      <span v-if="marketSources.length" class="market-src-count">· 已配置 {{ marketSources.length }} 个市场源</span>
    </div>

    <!-- 视图切换：万灵阁(全部) / 百宝袋(收藏) / 百鸟朝凤(老树洞/蹦蹦新窝) -->
    <div class="cat-bar">
      <button class="cat-pill" :class="{ active: marketView === 'all' }" @click="emit('update:marketView', 'all')">全部</button>
      <button class="cat-pill" :class="{ active: marketView === 'favorites' }" @click="emit('update:marketView', 'favorites')">⭐ 百宝袋</button>
      <button class="cat-pill" :class="{ active: marketView === 'charts' }" @click="emit('update:marketView', 'charts')">🔥 百鸟朝凤</button>
    </div>

    <!-- 百鸟朝凤视图 -->
    <div v-if="marketView === 'charts'" class="charts-wrap">
      <div class="chart-col solid-card">
        <div class="chart-title">🥇 老树洞 · 累计安装 Top30</div>
        <div v-for="(row, i) in marketCharts?.disciples ?? []" :key="row.plugin_id" class="chart-row">
          <span class="chart-rank" :class="['gold', 'silver', 'bronze'][i] ?? ''">{{ i + 1 }}</span>
          <span class="chart-name">{{ row.plugin_id }}</span>
          <span class="chart-num">{{ row.installs }} 次安装</span>
        </div>
        <el-empty v-if="!(marketCharts?.disciples ?? []).length" description="暂无安装数据（安装插件后生成榜单）" :image-size="60" />
      </div>
      <div class="chart-col solid-card">
        <div class="chart-title">🌱 蹦蹦新窝 · 最近安装 Top30</div>
        <div v-for="(row, i) in marketCharts?.newcomers ?? []" :key="row.plugin_id" class="chart-row">
          <span class="chart-rank">{{ i + 1 }}</span>
          <span class="chart-name">{{ row.plugin_id }}</span>
          <span class="chart-num">{{ row.installs }} 次安装</span>
        </div>
        <el-empty v-if="!(marketCharts?.newcomers ?? []).length" description="暂无数据" :image-size="60" />
      </div>
    </div>

    <!-- 百宝袋（收藏）视图 -->
    <div v-else-if="marketView === 'favorites'" v-loading="loading" class="market-grid">
      <div v-for="item in favoritePlugins" :key="item.id" class="market-card solid-card solid-card-hover">
        <div class="mc-top">
          <div class="mc-avatar" :style="{ background: avatarColor(item.name) }">{{ pluginInitial(item.name) }}</div>
          <div class="mc-info">
            <div class="mc-name">{{ item.name }}</div>
            <div class="mc-publisher">{{ item.publisher?.name || item.author || '未知' }}</div>
          </div>
          <el-tag size="small" effect="plain">{{ item.installed ? '已安装' : '未安装' }}</el-tag>
        </div>
        <div class="mc-desc">{{ item.description || '暂无描述' }}</div>
        <div class="mc-foot">
          <el-button size="small" :icon="item.favorited ? StarFilled : Star" @click="emit('toggleFavorite', item)">
            {{ item.favorited ? '断缘' : '收藏' }}
          </el-button>
        </div>
      </div>
      <el-empty v-if="!loading && !favoritePlugins.length" description="百宝袋空空如也——在「全部」里点收藏（Star）收插件" />
    </div>

    <!-- 万灵阁（全部）视图 -->
    <template v-else>
    <div v-if="marketCategories.length" class="cat-bar">
      <button
        class="cat-pill"
        :class="{ active: marketCategory === '' }"
        @click="emit('update:marketCategory', '')"
      >全部 {{ (market?.plugins || []).length }}</button>
      <button
        v-for="c in marketCategories"
        :key="c"
        class="cat-pill"
        :class="{ active: marketCategory === c }"
        @click="emit('update:marketCategory', c)"
      >{{ c }}</button>
    </div>

    <div v-loading="loading" class="market-grid">
      <div
        v-for="item in filteredMarketPlugins"
        :key="item.id"
        class="market-card solid-card solid-card-hover"
      >
        <div class="mc-top">
          <div class="mc-avatar" :style="{ background: avatarColor(item.name) }">{{ pluginInitial(item.name) }}</div>
          <div class="mc-info">
            <div class="mc-name">{{ item.name }}</div>
            <div class="mc-publisher">{{ item.publisher?.name || item.author || shortSource(item.source) || '未知' }}</div>
          </div>
          <el-tag size="small" effect="plain">v{{ item.version }}</el-tag>
        </div>

        <div class="mc-desc">{{ item.description || '暂无描述' }}</div>

        <div class="mc-tags">
          <el-tag size="small" :type="trustType(item.trust_level)">{{ trustLabel(item.trust_level) }}</el-tag>
          <el-tag v-if="item.category" size="small" effect="plain">{{ item.category }}</el-tag>
          <el-tag size="small" :type="compatibilityType(item)" effect="plain">{{ compatibilityLabel(item) }}</el-tag>
          <el-tag v-if="item.install_count" size="small" type="info" effect="plain">{{ item.install_count }} 次安装</el-tag>
        </div>

        <div class="mc-foot">
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
          <el-button size="small" :icon="item.favorited ? StarFilled : Star" @click="emit('toggleFavorite', item)" />
          <el-button v-if="item.installed" size="small" :icon="Setting" @click="emit('openInstances', item.id)">分身</el-button>
        </div>
      </div>
      <el-empty v-if="!loading && !filteredMarketPlugins.length" description="暂无市场数据" />
    </div>
    </template>
  </div>

  <!-- 分神实例管理（天序：上移/下移持久化到服务端跨端同步） -->
  <el-dialog
    :model-value="instancesVisible"
    :title="`分身管理 · ${instancePluginId}`"
    width="560px"
    @update:model-value="emit('update:instancesVisible', $event)"
  >
    <el-alert v-if="singleInstance" type="info" :closable="false" show-icon
      title="单实例插件"
      description="该插件由宿主单点管理，不支持分身。" style="margin-bottom: 12px" />
    <template v-else>
      <div style="display: flex; gap: 8px; margin-bottom: 12px">
        <el-input v-model="newInstanceKey" placeholder="新分身名称（如 刷流-账号2）" size="small" />
        <el-button size="small" type="primary" @click="emit('createInstance', newInstanceKey); newInstanceKey = ''">分神</el-button>
      </div>
      <el-table :data="instances" size="small" empty-text="暂无分身（主配置在插件配置页）">
        <el-table-column label="分身" min-width="120">
          <template #default="{ row }">{{ (row as PluginInstance).instance_key }}</template>
        </el-table-column>
        <el-table-column label="启停" width="70">
          <template #default="{ row }">
            <el-switch size="small" :model-value="(row as PluginInstance).enabled" @change="emit('toggleInstance', row as PluginInstance)" />
          </template>
        </el-table-column>
        <el-table-column label="天序" width="110">
          <template #default="{ $index }">
            <el-button size="small" :icon="Top" circle plain :disabled="$index === 0" @click="emit('moveInstance', $index, -1)" />
            <el-button size="small" :icon="Bottom" circle plain :disabled="$index === instances.length - 1" @click="emit('moveInstance', $index, 1)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="70">
          <template #default="{ row }">
            <el-button size="small" link type="danger" @click="emit('removeInstance', row as PluginInstance)">归墟</el-button>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </el-dialog>

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

<style src="./Plugins.css"></style>
