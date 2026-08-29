<script setup lang="ts">
// 站点筛选公共组件：紧凑多列 checkbox 网格 + 搜索 + 全选/清空，适配一两百站点。
// 各页（搜索/公告/消息/勋章墙/站免/考核/后宫）复用，统一多选体验。
// modelValue 空 = 全部站点；非空 = 仅选中站点。el-dialog teleport 到 body（玻璃化用非 scoped 样式）。
import { useI18n } from 'vue-i18n'
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Search, CaretBottom } from '@element-plus/icons-vue'

const { t } = useI18n()

interface SiteItem {
  id: number
  name: string
}

const props = withDefaults(
  defineProps<{
    modelValue: number[]
    sites: SiteItem[]
    title?: string
    managePath?: string
  }>(),
  // title 缺省时模板兜底 t('sfd.title')；managePath 缺省走 /sites
  { managePath: '/sites' },
)

const emit = defineEmits<{ 'update:modelValue': [number[]] }>()

const router = useRouter()
const visible = ref(false)
const query = ref('')

const count = computed(() => props.sites.length)
const filteredSites = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? props.sites.filter((s) => s.name.toLowerCase().includes(q)) : props.sites
})
const isAllSelected = computed(
  () => count.value > 0 && props.modelValue.length === count.value,
)
const isIndeterminate = computed(
  () => props.modelValue.length > 0 && props.modelValue.length < count.value,
)

function toggleAll(v: boolean | string | number) {
  emit('update:modelValue', v ? props.sites.map((s) => s.id) : [])
}
function onGroupChange(v: (string | number | boolean)[]) {
  emit('update:modelValue', v as number[])
}
function goManage() {
  visible.value = false
  router.push(props.managePath)
}
</script>

<template>
  <button type="button" class="scope-filter-btn" @click="visible = true">
    <slot>{{ t('sfd.trigger') }}<el-icon><CaretBottom /></el-icon></slot>
  </button>
  <el-dialog
    v-model="visible"
    :title="title || t('sfd.title')"
    width="760px"
    append-to-body
    class="site-filter-dialog"
  >
    <div class="site-filter">
      <div class="site-filter-head">
        <span class="text-muted">{{ t('sfd.selected', { n: modelValue.length || t('sh.all'), total: count }) }}</span>
        <button type="button" class="link-btn" @click="toggleAll(false)">{{ t('common.clear') }}</button>
      </div>
      <el-input v-model="query" :placeholder="t('sfd.searchPh')" clearable class="site-filter-search">
        <template #prefix><el-icon><Search /></el-icon></template>
      </el-input>
      <el-checkbox
        class="site-filter-all"
        :model-value="isAllSelected"
        :indeterminate="isIndeterminate"
        @change="toggleAll"
      >{{ t('sfd.selectAll', { n: count }) }}</el-checkbox>
      <el-checkbox-group
        :model-value="modelValue"
        class="site-filter-grid"
        @change="onGroupChange"
      >
        <el-checkbox v-for="s in filteredSites" :key="s.id" :value="s.id" class="site-filter-cell">{{ s.name }}</el-checkbox>
      </el-checkbox-group>
      <div v-if="!filteredSites.length" class="site-filter-empty text-muted">{{ t('sfd.noMatch') }}</div>
    </div>
    <template #footer>
      <button type="button" class="site-filter-manage" @click="goManage">{{ t('sfd.goManage') }} →</button>
      <el-button type="primary" @click="visible = false">{{ t('sfd.done') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped>
.scope-filter-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-1);
  background: transparent;
  border: none;
  cursor: pointer;
  padding: 0;
  color: var(--pt-primary);
  font-size: var(--fs-xs);
}
.scope-filter-btn:hover {
  text-decoration: underline;
}
.site-filter-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 8px;
  font-size: var(--fs-xs);
}
.site-filter-head .text-muted {
  flex: 1;
}
.site-filter .link-btn {
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--pt-primary);
  font-size: var(--fs-xs);
  padding: 0;
}
.site-filter .link-btn:hover {
  text-decoration: underline;
}
.site-filter-search {
  margin-bottom: 10px;
}
.site-filter-all {
  width: 100%;
  margin-right: 0;
  padding: 6px 0;
  border-top: 1px solid var(--glass-border);
  border-bottom: 1px solid var(--glass-border);
}
/* 紧凑多列网格：自适应列数 + 收紧行高/间距/字号，少滚动即可扫读大量站点 */
.site-filter-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 0 var(--sp-1);
  max-height: 440px;
  overflow-y: auto;
  padding-top: 2px;
}
.site-filter-cell {
  margin-right: 0;
  min-width: 0;
  height: 28px;
}
.site-filter-cell :deep(.el-checkbox__label) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: var(--fs-body);
}
.site-filter-empty {
  padding: 24px 0;
  text-align: center;
  font-size: var(--fs-xs);
}
/* footer 内管理链接：margin-right:auto 推到左侧，与右侧「完成」分立两端 */
.site-filter-manage {
  margin-right: auto;
  background: transparent;
  border: none;
  cursor: pointer;
  font: inherit;
  color: var(--pt-primary);
  font-size: var(--fs-body);
  padding: 0;
  text-decoration: none;
}
.site-filter-manage:hover {
  text-decoration: underline;
}
</style>

<style>
/* 非 scoped：el-dialog teleport 到 body，scoped 选择器不生效。玻璃化背景融入主题（深色自适应）。 */
.site-filter-dialog {
  --el-dialog-bg-color: transparent;
  background: var(--glass-bg-strong);
  backdrop-filter: blur(var(--blur));
  -webkit-backdrop-filter: blur(var(--blur));
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-lg);
  box-shadow: var(--glass-hover-shadow);
}
</style>
