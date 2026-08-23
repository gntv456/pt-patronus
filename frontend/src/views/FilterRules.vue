<script setup lang="ts">
import type { FilterRule, FilterRuleGroup, FilterTorrent } from '@/api/filter'
import EmptyState from '@/components/EmptyState.vue'
import FilterRuleDialogs from '@/components/FilterRuleDialogs.vue'
import { useFilterRules } from '@/views/useFilterRules'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const {
  activeTab, builtins, loadingBuiltins, loadBuiltins,
  customs, loadingCustom, loadCustom, showCustomForm, editingCustom, savingCustom, customForm,
  openCreateCustom, openEditCustom, saveCustom, deleteCustom,
  groups, loadingGroups, loadGroups, showGroupForm, editingGroup, savingGroup, groupForm,
  openCreateGroup, openEditGroup, saveGroup, deleteGroup,
  testRuleString, testTitles, testing, testResult, fillTest, runTest,
  bindings, savingBindings, saveBindings,
} = useFilterRules()
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <header class="header">
      <div>
        <h1 v-if="!props.embedded">资源过滤规则</h1>
        <h3 v-else class="title embedded-title">过滤规则</h3>
        <p v-if="!props.embedded" class="sub">用规则组表达式对种子分级筛选，命中越靠前的优先级越高。</p>
      </div>
    </header>

    <el-tabs v-model="activeTab" class="glass tabs">
      <!-- Tab1 内置规则 -->
      <el-tab-pane label="内置规则" name="builtin">
        <el-table :data="builtins" v-loading="loadingBuiltins" size="small" empty-text="暂无内置规则">
          <el-table-column prop="id" label="ID" width="120" />
          <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
          <el-table-column label="包含 (include)" min-width="220">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).include?.join(' | ') || '—' }}</code></template>
          </el-table-column>
          <el-table-column label="排除 (exclude)" min-width="180">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).exclude?.join(' | ') || '—' }}</code></template>
          </el-table-column>
          <el-table-column label="大小(MB)" width="120">
            <template #default="{ row }">{{ (row as FilterRule).size_range || '—' }}</template>
          </el-table-column>
          <el-table-column label="最少做种" width="90">
            <template #default="{ row }">{{ (row as FilterRule).seeders || '—' }}</template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingBuiltins && !builtins.length" icon="🧩" title="暂无内置规则" description="内置规则由系统提供，作为规则组表达式的基本积木" />
      </el-tab-pane>

      <!-- Tab2 自定义规则 -->
      <el-tab-pane label="自定义规则" name="custom">
        <div class="toolbar">
          <span class="tip">自定义规则按 id 覆盖同名内置规则；include/exclude 为单条正则（不区分大小写）。</span>
          <el-button size="small" type="primary" @click="openCreateCustom">+ 新建规则</el-button>
          <el-button size="small" @click="loadCustom" :loading="loadingCustom">刷新</el-button>
        </div>
        <el-table :data="customs" v-loading="loadingCustom" size="small" empty-text="暂无自定义规则">
          <el-table-column prop="id" label="ID" width="120" />
          <el-table-column prop="name" label="名称" min-width="120" show-overflow-tooltip />
          <el-table-column label="包含" min-width="180">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).include?.[0] || '—' }}</code></template>
          </el-table-column>
          <el-table-column label="排除" min-width="160">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).exclude?.[0] || '—' }}</code></template>
          </el-table-column>
          <el-table-column label="大小(MB)" width="110">
            <template #default="{ row }">{{ (row as FilterRule).size_range || '—' }}</template>
          </el-table-column>
          <el-table-column label="做种≥" width="80">
            <template #default="{ row }">{{ (row as FilterRule).seeders || '—' }}</template>
          </el-table-column>
          <el-table-column label="发布(分)" width="100">
            <template #default="{ row }">{{ (row as FilterRule).publish_time || '—' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="openEditCustom(row as FilterRule)">编辑</el-button>
              <el-button size="small" link type="danger" @click="deleteCustom(row as FilterRule)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingCustom && !customs.length" icon="✨" title="暂无自定义规则" description="新建一条自定义规则（如中文音频、特定发布组），即可在规则组表达式中引用" />
      </el-tab-pane>

      <!-- Tab3 规则组 -->
      <el-tab-pane label="规则组" name="group">
        <div class="toolbar">
          <span class="tip">规则组按 media_type / category 作用域生效；表达式引用上方规则 id。</span>
          <el-button size="small" type="primary" @click="openCreateGroup">+ 新建规则组</el-button>
          <el-button size="small" @click="loadGroups" :loading="loadingGroups">刷新</el-button>
        </div>
        <el-table :data="groups" v-loading="loadingGroups" size="small" empty-text="暂无规则组">
          <el-table-column prop="name" label="组名" min-width="120" show-overflow-tooltip />
          <el-table-column label="规则表达式" min-width="240">
            <template #default="{ row }"><code class="re">{{ (row as FilterRuleGroup).rule_string }}</code></template>
          </el-table-column>
          <el-table-column label="媒体类型" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).media_type || '通用' }}</template>
          </el-table-column>
          <el-table-column label="分类" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).category || '—' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="180" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="fillTest(row as FilterRuleGroup)">试算</el-button>
              <el-button size="small" link @click="openEditGroup(row as FilterRuleGroup)">编辑</el-button>
              <el-button size="small" link type="danger" @click="deleteGroup(row as FilterRuleGroup)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingGroups && !groups.length" icon="🗂" title="暂无规则组" description="新建规则组，用表达式组合上方规则 id，按媒体类型/分类生效" />

        <!-- 规则试算 -->
        <section class="glass test-panel">
          <h3 class="blk-title">规则试算</h3>
          <el-input v-model="testRuleString" placeholder="规则表达式，如：4K & !BLU > 1080P" />
          <el-input v-model="testTitles" type="textarea" :rows="4" placeholder="每行一个种子标题" />
          <div class="test-actions">
            <el-button size="small" type="primary" :loading="testing" @click="runTest">试算</el-button>
          </div>
          <div v-if="testResult" class="test-result">
            <div class="tr-block">
              <span class="tr-label">命中层级（{{ testResult.levels.length }}）</span>
              <div class="levels">
                <el-tag v-for="(lv, i) in testResult.levels" :key="i" type="success" size="small">{{ (lv as unknown as FilterTorrent).title }}</el-tag>
              </div>
            </div>
            <div class="tr-block">
              <span class="tr-label">剔除 ({{ testResult.dropped.length }})</span>
              <div class="dropped">
                <div v-for="(d, i) in testResult.dropped" :key="i" class="drop-item">{{ (d as FilterTorrent).title }}</div>
                <span v-if="!testResult.dropped.length" class="muted">无</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 全局绑定 -->
        <section class="glass bind-panel">
          <h3 class="blk-title">全局绑定</h3>
          <p class="tip">为三个业务场景选择生效的规则组（可多选）；未选则该场景不过滤。</p>
          <el-form label-width="90px" size="small">
            <el-form-item label="搜索">
              <el-select v-model="bindings.search" multiple collapse-tags collapse-tags-tooltip placeholder="搜索时生效的规则组" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="订阅">
              <el-select v-model="bindings.subscribe" multiple collapse-tags collapse-tags-tooltip placeholder="订阅下载时生效的规则组" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item label="洗版">
              <el-select v-model="bindings.best_version" multiple collapse-tags collapse-tags-tooltip placeholder="洗版时生效的规则组" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button size="small" type="primary" :loading="savingBindings" @click="saveBindings">保存绑定</el-button>
            </el-form-item>
          </el-form>
        </section>
      </el-tab-pane>
    </el-tabs>

    <!-- 对话框组 -->
    <FilterRuleDialogs
      v-model:show-custom-form="showCustomForm" v-model:editing-custom="editingCustom" v-model:custom-form="customForm" v-model:saving-custom="savingCustom"
      v-model:show-group-form="showGroupForm" v-model:editing-group="editingGroup" v-model:group-form="groupForm" v-model:saving-group="savingGroup"
      @save-custom="saveCustom" @save-group="saveGroup"
    />
  </div>
</template>

<style scoped>
.page { padding: 16px; }
.header { margin-bottom: 12px; }
.header h1 { font-size: var(--fs-title); margin: 0; }
.sub { font-size: var(--fs-xs); opacity: 0.7; margin: 4px 0 0; }
.tabs { padding: 12px; border-radius: var(--radius-md); }
.toolbar { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: 10px; flex-wrap: wrap; }
.toolbar .tip { flex: 1; min-width: 200px; font-size: var(--fs-xs); color: var(--text-muted); }
.tip { font-size: var(--fs-xs); color: var(--text-muted); }
code.re { font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace; font-size: var(--fs-2xs); background: var(--surface-hover, rgba(127, 127, 127, 0.15)); padding: 1px 6px; border-radius: var(--radius-2xs); word-break: break-all; }
.blk-title { margin: 0 0 6px; font-size: var(--fs-lg); font-weight: 600; }
.test-panel, .bind-panel { margin-top: 14px; padding: 14px 16px; border-radius: var(--radius-md); display: flex; flex-direction: column; gap: var(--sp-2); }
.test-actions { display: flex; }
.test-result { margin-top: 4px; display: flex; flex-direction: column; gap: var(--sp-3); }
.tr-block { display: flex; flex-direction: column; gap: var(--sp-1); }
.tr-label { font-size: var(--fs-body); font-weight: 600; }
.levels { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.dropped { display: flex; flex-direction: column; gap: var(--sp-1); }
.drop-item { font-size: var(--fs-xs); padding: 4px 8px; border-radius: var(--radius-xs); background: var(--pt-danger-bg); color: var(--pt-danger, #e74c3c); }
.muted { color: var(--text-muted); font-size: var(--fs-xs); }
.page.embedded { padding: 0; max-width: none; }
.page.embedded .embedded-title { font-size: var(--fs-lg); font-weight: 600; margin: 0; }
.page.embedded .embedded-sub { margin-top: 0; margin-bottom: 10px; font-size: var(--fs-xs); }
.page.embedded > .sub { display: none; }
</style>
