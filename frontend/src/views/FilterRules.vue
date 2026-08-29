<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { FilterRule, FilterRuleGroup, FilterTorrent } from '@/api/filter'
import EmptyState from '@/components/EmptyState.vue'
import FilterRuleDialogs from '@/components/FilterRuleDialogs.vue'
import { useFilterRules } from '@/views/useFilterRules'

const { t } = useI18n()

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
        <h1 v-if="!props.embedded">{{ t('filterrules.title') }}</h1>
        <h3 v-else class="title embedded-title">{{ t('filterrules.titleShort') }}</h3>
        <p v-if="!props.embedded" class="sub">{{ t('filterrules.subtitle') }}</p>
      </div>
    </header>

    <el-tabs v-model="activeTab" class="glass tabs">
      <!-- Tab1 内置规则 -->
      <el-tab-pane :label="t('filterrules.tab.builtin')" name="builtin">
        <el-table :data="builtins" v-loading="loadingBuiltins" size="small" :empty-text="t('filterrules.noBuiltins')">
          <el-table-column prop="id" label="ID" width="120" />
          <el-table-column prop="name" :label="t('rss.f.name')" min-width="140" show-overflow-tooltip />
          <el-table-column :label="t('filterrules.col.includeFull')" min-width="220">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).include?.join(' | ') || '—' }}</code></template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.excludeFull')" min-width="180">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).exclude?.join(' | ') || '—' }}</code></template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.sizeMB')" width="120">
            <template #default="{ row }">{{ (row as FilterRule).size_range || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.minSeeders')" width="90">
            <template #default="{ row }">{{ (row as FilterRule).seeders || '—' }}</template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingBuiltins && !builtins.length" icon="🧩" :title="t('filterrules.noBuiltins')" :description="t('filterrules.noBuiltinsDesc')" />
      </el-tab-pane>

      <!-- Tab2 自定义规则 -->
      <el-tab-pane :label="t('filterrules.tab.custom')" name="custom">
        <div class="toolbar">
          <span class="tip">{{ t('filterrules.customTip') }}</span>
          <el-button size="small" type="primary" @click="openCreateCustom">{{ t('filterrules.newRule') }}</el-button>
          <el-button size="small" @click="loadCustom" :loading="loadingCustom">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table :data="customs" v-loading="loadingCustom" size="small" :empty-text="t('filterrules.noCustoms')">
          <el-table-column prop="id" label="ID" width="120" />
          <el-table-column prop="name" :label="t('rss.f.name')" min-width="120" show-overflow-tooltip />
          <el-table-column :label="t('filterrules.col.include')" min-width="180">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).include?.[0] || '—' }}</code></template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.exclude')" min-width="160">
            <template #default="{ row }"><code class="re">{{ (row as FilterRule).exclude?.[0] || '—' }}</code></template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.sizeMB')" width="110">
            <template #default="{ row }">{{ (row as FilterRule).size_range || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.seedersGte')" width="80">
            <template #default="{ row }">{{ (row as FilterRule).seeders || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.publishMin')" width="100">
            <template #default="{ row }">{{ (row as FilterRule).publish_time || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('reseed.col.actions')" width="130" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="openEditCustom(row as FilterRule)">{{ t('common.edit') }}</el-button>
              <el-button size="small" link type="danger" @click="deleteCustom(row as FilterRule)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingCustom && !customs.length" icon="✨" :title="t('filterrules.noCustoms')" :description="t('filterrules.noCustomsDesc')" />
      </el-tab-pane>

      <!-- Tab3 规则组 -->
      <el-tab-pane :label="t('filterrules.tab.group')" name="group">
        <div class="toolbar">
          <span class="tip">{{ t('filterrules.groupTip') }}</span>
          <el-button size="small" type="primary" @click="openCreateGroup">{{ t('filterrules.newGroup') }}</el-button>
          <el-button size="small" @click="loadGroups" :loading="loadingGroups">{{ t('common.refresh') }}</el-button>
        </div>
        <el-table :data="groups" v-loading="loadingGroups" size="small" :empty-text="t('filterrules.noGroups')">
          <el-table-column prop="name" :label="t('filterrules.col.groupName')" min-width="120" show-overflow-tooltip />
          <el-table-column :label="t('filterrules.col.ruleExpr')" min-width="240">
            <template #default="{ row }"><code class="re">{{ (row as FilterRuleGroup).rule_string }}</code></template>
          </el-table-column>
          <el-table-column :label="t('filterrules.col.mediaType')" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).media_type || t('filterrules.generic') }}</template>
          </el-table-column>
          <el-table-column :label="t('rss.f.category')" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).category || '—' }}</template>
          </el-table-column>
          <el-table-column :label="t('reseed.col.actions')" width="180" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="fillTest(row as FilterRuleGroup)">{{ t('filterrules.testRun') }}</el-button>
              <el-button size="small" link @click="openEditGroup(row as FilterRuleGroup)">{{ t('common.edit') }}</el-button>
              <el-button size="small" link type="danger" @click="deleteGroup(row as FilterRuleGroup)">{{ t('common.delete') }}</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState v-if="!loadingGroups && !groups.length" icon="🗂" :title="t('filterrules.noGroups')" :description="t('filterrules.noGroupsDesc')" />

        <!-- 规则试算 -->
        <section class="glass test-panel">
          <h3 class="blk-title">{{ t('filterrules.testPanel') }}</h3>
          <el-input v-model="testRuleString" :placeholder="t('filterrules.f.exprPlaceholder')" />
          <el-input v-model="testTitles" type="textarea" :rows="4" :placeholder="t('filterrules.f.titlesPlaceholder')" />
          <div class="test-actions">
            <el-button size="small" type="primary" :loading="testing" @click="runTest">{{ t('filterrules.testRun') }}</el-button>
          </div>
          <div v-if="testResult" class="test-result">
            <div class="tr-block">
              <span class="tr-label">{{ t('filterrules.hitLevels', { n: testResult.levels.length }) }}</span>
              <div class="levels">
                <el-tag v-for="(lv, i) in testResult.levels" :key="i" type="success" size="small">{{ (lv as unknown as FilterTorrent).title }}</el-tag>
              </div>
            </div>
            <div class="tr-block">
              <span class="tr-label">{{ t('filterrules.dropped', { n: testResult.dropped.length }) }}</span>
              <div class="dropped">
                <div v-for="(d, i) in testResult.dropped" :key="i" class="drop-item">{{ (d as FilterTorrent).title }}</div>
                <span v-if="!testResult.dropped.length" class="muted">{{ t('filterrules.none') }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 全局绑定 -->
        <section class="glass bind-panel">
          <h3 class="blk-title">{{ t('filterrules.bindPanel') }}</h3>
          <p class="tip">{{ t('filterrules.bindTip') }}</p>
          <el-form label-width="90px" size="small">
            <el-form-item :label="t('common.search')">
              <el-select v-model="bindings.search" multiple collapse-tags collapse-tags-tooltip :placeholder="t('filterrules.f.bindSearch')" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('subscriptions.title')">
              <el-select v-model="bindings.subscribe" multiple collapse-tags collapse-tags-tooltip :placeholder="t('filterrules.f.bindSubscribe')" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item :label="t('subscriptions.bestVersion')">
              <el-select v-model="bindings.best_version" multiple collapse-tags collapse-tags-tooltip :placeholder="t('filterrules.f.bindBestVersion')" style="width: 100%">
                <el-option v-for="g in groups" :key="g.name" :label="g.name" :value="g.name" />
              </el-select>
            </el-form-item>
            <el-form-item>
              <el-button size="small" type="primary" :loading="savingBindings" @click="saveBindings">{{ t('filterrules.saveBindings') }}</el-button>
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
