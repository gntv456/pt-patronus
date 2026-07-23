<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {

  filterApi,
  type FilterRule,
  type FilterRuleGroup,
  type CustomRuleInput,
  type RuleGroupInput,
  type FilterBindings,
  type FilterMatched,
  type FilterTestResult,
  type FilterTorrent,
} from '@/api/filter'
import EmptyState from '@/components/EmptyState.vue'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })


const activeTab = ref<'builtin' | 'custom' | 'group'>('builtin')

// 错误统一友好化（http 拦截器已改写 message 为中文，兜底防 undefined）。
function errMsg(e: unknown, fb: string): string {
  return (e as Error)?.message || fb
}

// ─────────────── 内置规则（只读） ───────────────
const builtins = ref<FilterRule[]>([])
const loadingBuiltins = ref(false)
async function loadBuiltins() {
  loadingBuiltins.value = true
  try {
    builtins.value = await filterApi.builtins()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '加载内置规则失败'))
  } finally {
    loadingBuiltins.value = false
  }
}

// ─────────────── 自定义规则（CRUD） ───────────────
const customs = ref<FilterRule[]>([])
const loadingCustom = ref(false)
async function loadCustom() {
  loadingCustom.value = true
  try {
    customs.value = await filterApi.custom()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '加载自定义规则失败'))
  } finally {
    loadingCustom.value = false
  }
}

const showCustomForm = ref(false)
const editingCustom = ref<FilterRule | null>(null)
const savingCustom = ref(false)
const customForm = ref<CustomRuleInput>(defaultCustom())

function defaultCustom(): CustomRuleInput {
  return { id: '', name: '', include: '', exclude: '', size_range: '', seeders: 0, publish_time: '' }
}
function openCreateCustom() {
  editingCustom.value = null
  customForm.value = defaultCustom()
  showCustomForm.value = true
}
function openEditCustom(r: FilterRule) {
  editingCustom.value = r
  // 自定义规则入库后 include/exclude 为单元素数组，回填为单个正则字符串。
  customForm.value = {
    id: r.id,
    name: r.name ?? '',
    include: r.include?.[0] ?? '',
    exclude: r.exclude?.[0] ?? '',
    size_range: r.size_range ?? '',
    seeders: r.seeders ?? 0,
    publish_time: r.publish_time ?? '',
  }
  showCustomForm.value = true
}
async function saveCustom() {
  if (!customForm.value.id.trim()) {
    ElMessage.warning('规则 id 必填（仅字母数字）')
    return
  }
  try {
    savingCustom.value = true
    if (editingCustom.value) {
      await filterApi.updateCustom(editingCustom.value.id, customForm.value)
      ElMessage.success('已更新')
    } else {
      await filterApi.createCustom(customForm.value)
      ElMessage.success('已创建')
    }
    showCustomForm.value = false
    await loadCustom()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    savingCustom.value = false
  }
}
async function deleteCustom(r: FilterRule) {
  try {
    await ElMessageBox.confirm(`删除自定义规则「${r.id}」？被规则组引用时将被拒绝。`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await filterApi.deleteCustom(r.id)
    ElMessage.success('已删除')
    await loadCustom()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ─────────────── 规则组（CRUD） ───────────────
const groups = ref<FilterRuleGroup[]>([])
const loadingGroups = ref(false)
async function loadGroups() {
  loadingGroups.value = true
  try {
    groups.value = await filterApi.groups()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '加载规则组失败'))
  } finally {
    loadingGroups.value = false
  }
}

const showGroupForm = ref(false)
const editingGroup = ref<FilterRuleGroup | null>(null)
const savingGroup = ref(false)
const groupForm = ref<RuleGroupInput>(defaultGroup())

function defaultGroup(): RuleGroupInput {
  return { name: '', rule_string: '', media_type: '', category: '' }
}
function openCreateGroup() {
  editingGroup.value = null
  groupForm.value = defaultGroup()
  showGroupForm.value = true
}
function openEditGroup(g: FilterRuleGroup) {
  editingGroup.value = g
  groupForm.value = {
    name: g.name,
    rule_string: g.rule_string,
    media_type: g.media_type ?? '',
    category: g.category ?? '',
  }
  showGroupForm.value = true
}
async function saveGroup() {
  if (!groupForm.value.name.trim() || !groupForm.value.rule_string.trim()) {
    ElMessage.warning('组名与规则表达式必填')
    return
  }
  try {
    savingGroup.value = true
    if (editingGroup.value) {
      await filterApi.updateGroup(editingGroup.value.name, groupForm.value)
      ElMessage.success('已更新')
    } else {
      await filterApi.createGroup(groupForm.value)
      ElMessage.success('已创建')
    }
    showGroupForm.value = false
    await loadGroups()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    savingGroup.value = false
  }
}
async function deleteGroup(g: FilterRuleGroup) {
  try {
    await ElMessageBox.confirm(`删除规则组「${g.name}」？全局绑定中的悬空引用会被一并清理。`, '确认', { type: 'warning' })
  } catch {
    return
  }
  try {
    await filterApi.deleteGroup(g.name)
    ElMessage.success('已删除')
    await loadGroups()
    await loadBindings() // 绑定可能被级联清理，刷新保持一致
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ─────────────── 规则试算 ───────────────
const testRuleString = ref('')
const testTitles = ref('')
const testing = ref(false)
const testResult = ref<FilterTestResult | null>(null)

function fillTest(g: FilterRuleGroup) {
  testRuleString.value = g.rule_string
  testResult.value = null
}

async function runTest() {
  if (!testRuleString.value.trim()) {
    ElMessage.warning('请填写规则表达式')
    return
  }
  const torrents: FilterTorrent[] = testTitles.value
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)
    .map((title) => ({ title }))
  if (!torrents.length) {
    ElMessage.warning('请至少输入一行种子标题')
    return
  }
  try {
    testing.value = true
    testResult.value = await filterApi.test({ rule_string: testRuleString.value, torrents })
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '试算失败'))
  } finally {
    testing.value = false
  }
}

// ─────────────── 全局绑定 ───────────────
const bindings = ref<FilterBindings>({ search: [], subscribe: [], best_version: [] })
const savingBindings = ref(false)
async function loadBindings() {
  try {
    bindings.value = await filterApi.bindings()
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '加载全局绑定失败'))
  }
}
async function saveBindings() {
  try {
    savingBindings.value = true
    await filterApi.setBindings(bindings.value)
    ElMessage.success('全局绑定已保存')
  } catch (e: unknown) {
    ElMessage.error(errMsg(e, '保存绑定失败'))
  } finally {
    savingBindings.value = false
  }
}

onMounted(() => {
  loadBuiltins()
  loadCustom()
  loadGroups()
  loadBindings()
})
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
      <!-- ────── Tab1 内置规则 ────── -->
      <el-tab-pane label="内置规则" name="builtin">
        <el-table :data="builtins" v-loading="loadingBuiltins" size="small" empty-text="暂无内置规则">
          <el-table-column prop="id" label="ID" width="120" />
          <el-table-column prop="name" label="名称" min-width="140" show-overflow-tooltip />
          <el-table-column label="包含 (include)" min-width="220">
            <template #default="{ row }">
              <code class="re">{{ (row as FilterRule).include?.join(' | ') || '—' }}</code>
            </template>
          </el-table-column>
          <el-table-column label="排除 (exclude)" min-width="180">
            <template #default="{ row }">
              <code class="re">{{ (row as FilterRule).exclude?.join(' | ') || '—' }}</code>
            </template>
          </el-table-column>
          <el-table-column label="大小(MB)" width="120">
            <template #default="{ row }">{{ (row as FilterRule).size_range || '—' }}</template>
          </el-table-column>
          <el-table-column label="最少做种" width="90">
            <template #default="{ row }">{{ (row as FilterRule).seeders || '—' }}</template>
          </el-table-column>
        </el-table>
        <EmptyState
          v-if="!loadingBuiltins && !builtins.length"
          icon="🧩"
          title="暂无内置规则"
          description="内置规则由系统提供，作为规则组表达式的基本积木"
        />
      </el-tab-pane>

      <!-- ────── Tab2 自定义规则 ────── -->
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
            <template #default="{ row }">
              <code class="re">{{ (row as FilterRule).include?.[0] || '—' }}</code>
            </template>
          </el-table-column>
          <el-table-column label="排除" min-width="160">
            <template #default="{ row }">
              <code class="re">{{ (row as FilterRule).exclude?.[0] || '—' }}</code>
            </template>
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
        <EmptyState
          v-if="!loadingCustom && !customs.length"
          icon="✨"
          title="暂无自定义规则"
          description="新建一条自定义规则（如中文音频、特定发布组），即可在规则组表达式中引用"
        />
      </el-tab-pane>

      <!-- ────── Tab3 规则组 ────── -->
      <el-tab-pane label="规则组" name="group">
        <div class="toolbar">
          <span class="tip">规则组按 media_type / category 作用域生效；表达式引用上方规则 id。</span>
          <el-button size="small" type="primary" @click="openCreateGroup">+ 新建规则组</el-button>
          <el-button size="small" @click="loadGroups" :loading="loadingGroups">刷新</el-button>
        </div>
        <el-table :data="groups" v-loading="loadingGroups" size="small" empty-text="暂无规则组">
          <el-table-column prop="name" label="组名" min-width="120" show-overflow-tooltip />
          <el-table-column label="规则表达式" min-width="240">
            <template #default="{ row }">
              <code class="re">{{ (row as FilterRuleGroup).rule_string }}</code>
            </template>
          </el-table-column>
          <el-table-column label="媒体类型" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).media_type || '通用' }}</template>
          </el-table-column>
          <el-table-column label="分类" width="100">
            <template #default="{ row }">{{ (row as FilterRuleGroup).category || '—' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="170" fixed="right">
            <template #default="{ row }">
              <el-button size="small" link @click="fillTest(row as FilterRuleGroup)">填入测试</el-button>
              <el-button size="small" link @click="openEditGroup(row as FilterRuleGroup)">编辑</el-button>
              <el-button size="small" link type="danger" @click="deleteGroup(row as FilterRuleGroup)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <EmptyState
          v-if="!loadingGroups && !groups.length"
          icon="🗂️"
          title="暂无规则组"
          description="规则组把多条规则组合成带优先级的表达式，供搜索/订阅/洗版复用"
        />

        <!-- 试算面板 -->
        <section class="glass test-panel">
          <h3 class="blk-title">规则试算</h3>
          <p class="syntax">
            语法：<code>&gt;</code> 分优先级层（左优先级最高）；<code>&amp;</code> 与、<code>|</code> 或、<code>!</code> 非、<code>()</code> 分组，操作数是规则 id。
            示例：<code>4K &amp; !BLU &gt; 1080P</code>（4K 且非蓝光最优先，否则 1080P 次之，都不命中则剔除）。
          </p>
          <el-input v-model="testRuleString" placeholder="如：4K & !BLU > 1080P" />
          <el-input
            v-model="testTitles"
            type="textarea"
            :rows="5"
            placeholder="每行粘贴一个种子标题，用于试算匹配结果"
          />
          <div class="test-actions">
            <el-button size="small" type="primary" :loading="testing" @click="runTest">开始试算</el-button>
          </div>

          <div v-if="testResult" class="test-result">
            <div class="tr-block">
              <span class="tr-label">优先级层</span>
              <div class="levels">
                <el-tag v-for="(lv, i) in testResult.levels" :key="i" size="small" :type="i === 0 ? 'success' : 'info'">
                  {{ i + 1 }}. {{ lv }}
                </el-tag>
                <span v-if="!testResult.levels.length" class="muted">无</span>
              </div>
            </div>
            <div class="tr-block">
              <span class="tr-label">命中 ({{ testResult.matched.length }})</span>
              <el-table :data="testResult.matched" size="small" empty-text="无命中">
                <el-table-column label="优先级" width="80">
                  <template #default="{ row }">
                    <el-tag size="small" type="success">{{ (row as FilterMatched).pri_order }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="标题" show-overflow-tooltip>
                  <template #default="{ row }">{{ (row as FilterMatched).torrent.title }}</template>
                </el-table-column>
              </el-table>
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

    <!-- 自定义规则表单 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showCustomForm" :title="editingCustom ? '编辑自定义规则' : '新建自定义规则'" width="560px">
      <el-form :model="customForm" label-width="110px" size="small">
        <el-form-item label="规则 ID" required>
          <el-input v-model="customForm.id" placeholder="仅字母数字，如 CN_AUDIO" :disabled="!!editingCustom" />
        </el-form-item>
        <el-form-item label="名称"><el-input v-model="customForm.name" placeholder="如：中文音轨" /></el-form-item>
        <el-form-item label="包含 (include)">
          <el-input v-model="customForm.include" placeholder="正则，命中标题/副标题/标签即保留，如 国语|普通话" />
        </el-form-item>
        <el-form-item label="排除 (exclude)">
          <el-input v-model="customForm.exclude" placeholder="正则，命中即拒绝，如 原盘|REMUX" />
        </el-form-item>
        <el-form-item label="大小范围(MB)">
          <el-input v-model="customForm.size_range" placeholder="1000-5000 / >1000 / <5000" />
        </el-form-item>
        <el-form-item label="最少做种">
          <el-input-number v-model="customForm.seeders" :min="0" controls-position="right" />
        </el-form-item>
        <el-form-item label="发布时间(分)">
          <el-input v-model="customForm.publish_time" placeholder="60 / 60-1440（种子发布距今分钟数）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCustomForm = false">取消</el-button>
        <el-button type="primary" :loading="savingCustom" @click="saveCustom">保存</el-button>
      </template>
    </el-dialog>

    <!-- 规则组表单 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showGroupForm" :title="editingGroup ? '编辑规则组' : '新建规则组'" width="560px">
      <el-form :model="groupForm" label-width="110px" size="small">
        <el-form-item label="组名" required>
          <el-input v-model="groupForm.name" placeholder="如：默认规则组" :disabled="!!editingGroup" />
        </el-form-item>
        <el-form-item label="规则表达式" required>
          <el-input v-model="groupForm.rule_string" type="textarea" :rows="2" placeholder="如：4K & !BLU > 1080P" />
        </el-form-item>
        <el-form-item label="媒体类型">
          <el-select v-model="groupForm.media_type" style="width: 100%">
            <el-option label="通用（不限）" value="" />
            <el-option label="电影" value="电影" />
            <el-option label="电视剧" value="电视剧" />
          </el-select>
        </el-form-item>
        <el-form-item label="分类"><el-input v-model="groupForm.category" placeholder="留空=不限，如 动画" /></el-form-item>
      </el-form>
      <p class="syntax">
        语法：<code>&gt;</code> 分优先级层（左优先级最高）；<code>&amp;</code> 与、<code>|</code> 或、<code>!</code> 非、<code>()</code> 分组；操作数为规则 id。
      </p>
      <template #footer>
        <el-button @click="showGroupForm = false">取消</el-button>
        <el-button type="primary" :loading="savingGroup" @click="saveGroup">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  padding: 16px;
}
.header {
  margin-bottom: 12px;
}
.header h1 {
  font-size: 20px;
  margin: 0;
}
.sub {
  font-size: 12px;
  opacity: 0.7;
  margin: 4px 0 0;
}
.tabs {
  padding: 12px;
  border-radius: 14px;
}
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.toolbar .tip {
  flex: 1;
  min-width: 200px;
  font-size: 12px;
  color: var(--text-muted);
}
.tip {
  font-size: 12px;
  color: var(--text-muted);
}
code.re {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: 11px;
  background: var(--surface-hover, rgba(127, 127, 127, 0.15));
  padding: 1px 6px;
  border-radius: 4px;
  word-break: break-all;
}
.blk-title {
  margin: 0 0 6px;
  font-size: 15px;
  font-weight: 600;
}
.test-panel,
.bind-panel {
  margin-top: 14px;
  padding: 14px 16px;
  border-radius: 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.syntax {
  font-size: 12px;
  color: var(--text-muted);
  line-height: 1.6;
  margin: 0;
}
.syntax code {
  background: var(--surface-hover, rgba(127, 127, 127, 0.15));
  padding: 1px 5px;
  border-radius: 4px;
}
.test-actions {
  display: flex;
}
.test-result {
  margin-top: 4px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.tr-block {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.tr-label {
  font-size: 13px;
  font-weight: 600;
}
.levels {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.dropped {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.drop-item {
  font-size: 12px;
  padding: 4px 8px;
  border-radius: 6px;
  background: rgba(231, 76, 60, 0.1);
  color: var(--pt-danger, #e74c3c);
}
.muted {
  color: var(--text-muted);
  font-size: 12px;
}

/* embedded：嵌进设置页时去掉外页边距，标题交给分区 Tab */
.page.embedded { padding: 0; max-width: none; }
.page.embedded .embedded-title { font-size: 15px; font-weight: 600; margin: 0; }
.page.embedded .embedded-sub { margin-top: 0; margin-bottom: 10px; font-size: 12px; }
.page.embedded > .sub { display: none; }

</style>
