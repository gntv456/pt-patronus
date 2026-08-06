<script setup lang="ts">
/**
 * 站点搜索结果筛选条（对标 MoviePilot TorrentFilterBar / PT-depiler 高级筛选）。
 * pill 多选（站点/促销/分辨率/编码/来源/特效）+ 中字开关 + 高级面板（大小范围/做种下限/发布时间）+ 排序。
 * filter/sort 是父组件 composable 的 reactive 对象（传引用，直接改嵌套属性即可响应）；removeFilter/clearAll 透传。
 */
import { computed } from 'vue'
import type { ActiveTag, ArrKey, FilterOptions, FilterState, SortField } from '@/composables/useSearchFilter'
import { SEARCH_DIMS, TIME_OPTS, SORT_FIELDS } from './searchFilterConstants'

const props = withDefaults(
  defineProps<{
    filter: FilterState
    sort: { field: SortField; order: 'asc' | 'desc' }
    options: FilterOptions
    activeTags: ActiveTag[]
    hasActive: boolean
    resultCount: number
    totalCount: number
    sortable?: boolean
    removeFilter: (t: ActiveTag) => void
    clearAll: () => void
  }>(),
  { sortable: true },
)

const sortLabel = computed(
  () => SORT_FIELDS.find((f) => f.key === props.sort.field)?.label || '做种数',
)

function onSortField(key: SortField) {
  props.sort.field = key
}
function toggleOrder() {
  props.sort.order = props.sort.order === 'desc' ? 'asc' : 'desc'
}
function selectAll(key: ArrKey) {
  props.filter[key] = [...props.options[key]]
}
function clearDim(key: ArrKey) {
  props.filter[key] = []
}
</script>


<template>
  <div class="sfb">
    <div class="sfb-row">
      <span class="sfb-count">
        🔍 <b>{{ resultCount }}</b><span class="text-muted">/{{ totalCount }}</span>
      </span>

      <template v-if="sortable">
        <el-dropdown trigger="click" @command="onSortField">
          <button type="button" class="sfb-sort">排序: {{ sortLabel }} ▾</button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item
                v-for="f in SORT_FIELDS"
                :key="f.key"
                :command="f.key"
                :class="{ 'is-active': sort.field === f.key }"
              >{{ f.label }}</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <button
          type="button"
          class="sfb-order"
          :title="sort.order === 'desc' ? '降序' : '升序'"
          @click="toggleOrder"
        >{{ sort.order === 'desc' ? '↓' : '↑' }}</button>
      </template>

      <div class="sfb-pills">
        <el-popover
          v-for="d in SEARCH_DIMS"
          :key="d.key"
          trigger="click"
          placement="bottom-start"
          :width="200"
          :popper-class="'sfb-popper'"
        >
          <template #reference>
            <button type="button" class="sfb-pill" :class="{ on: filter[d.key].length }">
              {{ d.icon }} {{ d.label }}
              <span v-if="filter[d.key].length" class="sfb-badge">{{ filter[d.key].length }}</span>
            </button>
          </template>
          <div class="sfb-pop">
            <div class="sfb-pop-head">
              <button type="button" @click="selectAll(d.key)">全选</button>
              <button type="button" @click="clearDim(d.key)">清除</button>
            </div>
            <div v-if="options[d.key].length" class="sfb-checks">
              <el-checkbox-group v-model="filter[d.key]">
                <el-checkbox
                  v-for="opt in options[d.key]"
                  :key="opt"
                  :value="opt"
                >{{ opt }}</el-checkbox>
              </el-checkbox-group>
            </div>
            <div v-else class="sfb-empty text-muted">无可用选项</div>
          </div>
        </el-popover>

        <button
          type="button"
          class="sfb-pill"
          :class="{ on: filter.onlyCnSub }"
          @click="filter.onlyCnSub = !filter.onlyCnSub"
        >🔤 中字</button>

        <el-popover trigger="click" placement="bottom-end" :width="280" popper-class="sfb-popper">
          <template #reference>
            <button type="button" class="sfb-pill" :class="{ on: hasActive && (filter.sizeMinGB || filter.sizeMaxGB || filter.seedersMin || filter.timePreset) }">
              ⚙ 高级
            </button>
          </template>
          <div class="sfb-adv">
            <div class="adv-row">
              <label>大小 (GB)</label>
              <div class="adv-range">
                <el-input-number
                  v-model="filter.sizeMinGB"
                  :min="0"
                  :max="options.maxSizeGB"
                  :step="1"
                  size="small"
                  controls-position="right"
                />
                <span class="adv-sep">~</span>
                <el-input-number
                  v-model="filter.sizeMaxGB"
                  :min="0"
                  :max="options.maxSizeGB"
                  :step="1"
                  size="small"
                  controls-position="right"
                />
              </div>
            </div>
            <div class="adv-row">
              <label>做种 ≥</label>
              <el-input-number
                v-model="filter.seedersMin"
                :min="0"
                :step="1"
                size="small"
                controls-position="right"
              />
            </div>
            <div class="adv-row">
              <label>发布时间</label>
              <div class="adv-time">
                <button
                  type="button"
                  class="time-chip"
                  :class="{ on: !filter.timePreset }"
                  @click="filter.timePreset = ''"
                >不限</button>
                <button
                  v-for="t in TIME_OPTS"
                  :key="t.key"
                  type="button"
                  class="time-chip"
                  :class="{ on: filter.timePreset === t.key }"
                  @click="filter.timePreset = t.key"
                >{{ t.label }}</button>
              </div>
            </div>
          </div>
        </el-popover>
      </div>
    </div>

    <div v-if="hasActive" class="sfb-tags">
      <el-tag
        v-for="tag in activeTags"
        :key="tag.key + ':' + tag.value"
        closable
        size="small"
        @close="removeFilter(tag)"
      >{{ tag.label }}</el-tag>
      <button type="button" class="sfb-clear" @click="clearAll">清除全部</button>
    </div>
  </div>
</template>

<style scoped>
.sfb {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px 14px;
  border-radius: 12px;
  background: var(--surface-muted, rgba(108, 92, 231, 0.05));
  border: 1px solid var(--surface-border, rgba(15, 23, 42, 0.08));
}
.sfb-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.sfb-count {
  font-size: 13px;
  white-space: nowrap;
}
.sfb-count b {
  color: var(--pt-primary, #6c5ce7);
}
.sfb-sort,
.sfb-order {
  background: transparent;
  border: 1px solid var(--surface-border, rgba(15, 23, 42, 0.12));
  border-radius: 999px;
  padding: 3px 12px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-strong);
  white-space: nowrap;
}
.sfb-sort:hover,
.sfb-order:hover {
  border-color: var(--pt-primary, #6c5ce7);
  color: var(--pt-primary, #6c5ce7);
}
.sfb-order {
  padding: 3px 10px;
  font-weight: 700;
}
.sfb-pills {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.sfb-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  background: transparent;
  border: 1px solid var(--surface-border, rgba(15, 23, 42, 0.12));
  border-radius: 999px;
  padding: 3px 12px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-strong);
  white-space: nowrap;
  transition: all 0.15s;
}
.sfb-pill:hover {
  border-color: var(--pt-primary, #6c5ce7);
}
.sfb-pill.on {
  background: rgba(108, 92, 231, 0.14);
  border-color: var(--pt-primary, #6c5ce7);
  color: var(--pt-primary, #6c5ce7);
  font-weight: 600;
}
.sfb-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: var(--pt-primary, #6c5ce7);
  color: #fff;
  font-size: 10px;
  font-weight: 700;
}
.sfb-tags {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.sfb-clear {
  background: transparent;
  border: 0;
  color: var(--pt-primary, #6c5ce7);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.sfb-clear:hover {
  text-decoration: underline;
}
.sfb-pop {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 320px;
}
.sfb-pop-head {
  display: flex;
  gap: 12px;
  border-bottom: 1px solid var(--divider, rgba(0, 0, 0, 0.06));
  padding-bottom: 6px;
}
.sfb-pop-head button {
  background: transparent;
  border: 0;
  color: var(--pt-primary, #6c5ce7);
  font-size: 12px;
  cursor: pointer;
  padding: 0;
}
.sfb-pop-head button:hover {
  text-decoration: underline;
}
.sfb-checks {
  overflow-y: auto;
  max-height: 260px;
}
.sfb-checks :deep(.el-checkbox) {
  display: flex;
  margin-right: 0;
  height: 26px;
}
.sfb-empty {
  font-size: 12px;
  padding: 8px 0;
  text-align: center;
}
.sfb-adv {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.adv-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.adv-row label {
  font-size: 12px;
  color: var(--text-muted);
}
.adv-range {
  display: flex;
  align-items: center;
  gap: 6px;
}
.adv-range .el-input-number {
  flex: 1;
  min-width: 0;
}
.adv-sep {
  color: var(--text-muted);
}
.adv-time {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.time-chip {
  background: transparent;
  border: 1px solid var(--surface-border, rgba(15, 23, 42, 0.12));
  border-radius: 999px;
  padding: 3px 12px;
  font-size: 12px;
  cursor: pointer;
  color: var(--text-strong);
}
.time-chip.on {
  background: rgba(108, 92, 231, 0.14);
  border-color: var(--pt-primary, #6c5ce7);
  color: var(--pt-primary, #6c5ce7);
  font-weight: 600;
}
:deep(.el-dropdown-menu__item.is-active) {
  color: var(--pt-primary, #6c5ce7);
  font-weight: 600;
}
@media (max-width: 720px) {
  .sfb-row {
    gap: 6px;
  }
}
</style>
