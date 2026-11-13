<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
/**
 * 站点搜索结果筛选条。
 * pill 多选（站点/促销/分辨率/编码/来源/特效）+ 中字开关 + 高级面板（大小范围/做种下限/发布时间）+ 排序。
 * filter/sort 是父组件 composable 的 reactive 对象（传引用，直接改嵌套属性即可响应）；removeFilter/clearAll 透传。
 */
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import type { ActiveTag, ArrKey, FilterOptions, FilterState, SortField } from '@/composables/useSearchFilter'
import { SEARCH_DIMS, TIME_OPTS, SORT_FIELDS } from './searchFilterConstants'

// filter/sort 用 defineModel 双向绑定：筛选/排序修改经 update:filter / update:sort 回传，
// 消除对 prop 的直接突变（父级 v-model 接收；对象整体替换，嵌套字段由本组件内聚修改）。
const { t } = useI18n()

const filter = defineModel<FilterState>('filter', { required: true })
const sort = defineModel<{ field: SortField; order: 'asc' | 'desc' }>('sort', { required: true })
// 注：父链（Search → TorrentResultsTable）传入的是 useSearchFilter 的 reactive 句柄字段，
// 共享可变状态为既定架构；defineModel 在此的语义是「父授权的双向绑定」。

const props = withDefaults(
  defineProps<{
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

const sortLabel = computed(() => t(SORT_FIELDS.find((f) => f.key === sort.value.field)?.labelKey || 'sfc.seeders'))

function onSortField(key: SortField) {
  sort.value.field = key
}
function toggleOrder() {
  sort.value.order = sort.value.order === 'desc' ? 'asc' : 'desc'
}
function selectAll(key: ArrKey) {
  filter.value[key] = [...props.options[key]]
}
function clearDim(key: ArrKey) {
  filter.value[key] = []
}
// 维度图标统一走单色 SptIcon（常量文件里的 emoji 不再渲染）
function dimIcon(key: ArrKey): string {
  const map: Record<string, string> = {
    site: 'globe', promo: 'gift', res: 'grid', codec: 'film', src: 'package', hdr: 'star',
  }
  return map[key] || 'filter'
}
</script>

<template>
  <div class="sfb">
    <div class="sfb-row">
      <span class="sfb-count">
        <SptIcon name="search" :size="14" aria-hidden="true" /> <b>{{ resultCount }}</b
        ><span class="text-muted">/{{ totalCount }}</span>
      </span>

      <template v-if="sortable">
        <el-dropdown trigger="click" @command="onSortField">
          <button type="button" class="sfb-sort">{{ t('sfb.sortBy') }}: {{ sortLabel }} <SptIcon name="chevron-down" :size="12" aria-hidden="true" /></button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item
                v-for="f in SORT_FIELDS"
                :key="f.key"
                :command="f.key"
                :class="{ 'is-active': sort.field === f.key }"
                >{{ t(f.labelKey) }}</el-dropdown-item
              >
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <button
          type="button"
          class="sfb-order"
          :title="sort.order === 'desc' ? t('sfb.desc') : t('sfb.asc')"
          :aria-label="sort.order === 'desc' ? t('sfb.desc') : t('sfb.asc')"
          :aria-pressed="sort.order === 'desc'"
          @click="toggleOrder"
        >
          <SptIcon :name="sort.order === 'desc' ? 'arrow-down' : 'arrow-up'" :size="14" aria-hidden="true" />
        </button>
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
            <button type="button" class="sfb-pill" :class="{ on: filter[d.key].length }" :aria-pressed="filter[d.key].length > 0">
              <SptIcon :name="dimIcon(d.key)" :size="14" aria-hidden="true" /> {{ t(d.labelKey) }}
              <span v-if="filter[d.key].length" class="sfb-badge">{{ filter[d.key].length }}</span>
            </button>
          </template>
          <div class="sfb-pop">
            <div class="sfb-pop-head">
              <button type="button" @click="selectAll(d.key)">{{ t('sfb.selectAll') }}</button>
              <button type="button" @click="clearDim(d.key)">{{ t('sfb.clear') }}</button>
            </div>
            <div v-if="options[d.key].length" class="sfb-checks">
              <el-checkbox-group v-model="filter[d.key]">
                <el-checkbox v-for="opt in options[d.key]" :key="opt" :value="opt">{{ opt }}</el-checkbox>
              </el-checkbox-group>
            </div>
            <div v-else class="sfb-empty text-muted">{{ t('sfb.noOptions') }}</div>
          </div>
        </el-popover>

        <button
          type="button"
          class="sfb-pill"
          :class="{ on: filter.onlyCnSub }"
          :aria-pressed="filter.onlyCnSub"
          @click="filter.onlyCnSub = !filter.onlyCnSub"
        >
          <SptIcon name="tag" :size="14" aria-hidden="true" /> {{ t('sfb.cnSub') }}
        </button>

        <button
          type="button"
          class="sfb-pill sfb-free"
          :class="{ on: filter.onlyFree }"
          :title="t('sfb.onlyFree')"
          :aria-pressed="filter.onlyFree"
          @click="filter.onlyFree = !filter.onlyFree"
        >
          <SptIcon name="gift" :size="14" aria-hidden="true" /> {{ t('sfb.freeLabel') }}
        </button>

        <el-popover trigger="click" placement="bottom-end" :width="280" popper-class="sfb-popper">
          <template #reference>
            <button
              type="button"
              class="sfb-pill"
              :class="{
                on: hasActive && (filter.sizeMinGB || filter.sizeMaxGB || filter.seedersMin || filter.timePreset),
              }"
            >
              <SptIcon name="gear" :size="14" aria-hidden="true" /> {{ t('sfb.advanced') }}
            </button>
          </template>
          <div class="sfb-adv">
            <div class="adv-row">
              <label>{{ t('sfb.sizeGB') }}</label>
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
              <label>{{ t('sfb.seedersMin') }}</label>
              <el-input-number v-model="filter.seedersMin" :min="0" :step="1" size="small" controls-position="right" />
            </div>
            <div class="adv-row">
              <label>{{ t('sfb.time') }}</label>
              <div class="adv-time">
                <button
                  type="button"
                  class="time-chip"
                  :class="{ on: !filter.timePreset }"
                  :aria-pressed="!filter.timePreset"
                  @click="filter.timePreset = ''"
                >
                  {{ t('sfb.anyTime') }}
                </button>
                <button
                  v-for="topt in TIME_OPTS"
                  :key="topt.key"
                  type="button"
                  class="time-chip"
                  :class="{ on: filter.timePreset === topt.key }"
                  :aria-pressed="filter.timePreset === topt.key"
                  @click="filter.timePreset = topt.key"
                >
                  {{ t(topt.labelKey) }}
                </button>
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
        >{{ tag.label }}</el-tag
      >
      <button type="button" class="sfb-clear" @click="clearAll">{{ t('sfb.clearAll') }}</button>
    </div>
  </div>
</template>

<style scoped src="./SearchFilterBar.css"></style>
