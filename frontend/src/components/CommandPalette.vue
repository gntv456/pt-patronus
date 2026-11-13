<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()

// 键位按平台显示，与顶栏保持一致。
const isMac = /Mac|iP(hone|ad|od)/.test(
  typeof navigator === 'undefined' ? '' : navigator.userAgent || navigator.platform,
)
const shortcutKey = isMac ? '⌘K' : 'Ctrl K'

export interface PaletteItem {
  name: string
  label: string
  icon?: string
  group?: string
  /** 额外别名（PT 黑话等），参与 fuzzy */
  aliases?: string[]
  desc?: string
  /** 自定义动作；无则 router.push({ name }) */
  action?: () => void
  /** true = 侧栏已有此入口；空查询时隐藏，仅输入查询时参与 fuzzy */
  sidebar?: boolean
}

const props = defineProps<{
  modelValue: boolean
  items: PaletteItem[]
  recent?: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [boolean]
}>()

const router = useRouter()
const query = ref('')
const active = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)
let lastFocused: HTMLElement | null = null

const open = computed({
  get: () => props.modelValue,
  set: (v: boolean) => emit('update:modelValue', v),
})

/** 简易 fuzzy：子序列匹配 + 前缀/包含加权；返回 score（越大越好），0=不匹配 */
function score(text: string, q: string): number {
  if (!q) return 1
  const t = text.toLowerCase()
  const s = q.toLowerCase().trim()
  if (!s) return 1
  if (t === s) return 1000
  if (t.startsWith(s)) return 800 - s.length
  if (t.includes(s)) return 500 - t.indexOf(s)
  // 子序列
  let ti = 0
  let gaps = 0
  let last = -1
  for (const ch of s) {
    const found = t.indexOf(ch, ti)
    if (found < 0) return 0
    if (last >= 0) gaps += found - last - 1
    last = found
    ti = found + 1
  }
  return Math.max(1, 200 - gaps * 3)
}

function itemScore(item: PaletteItem, q: string): number {
  if (!q.trim()) {
    // 空查询：最近访问靠前
    const ri = (props.recent || []).indexOf(item.name)
    if (ri >= 0) return 1000 - ri
    return 1
  }
  let best = score(item.label, q)
  best = Math.max(best, score(item.name, q))
  if (item.group) best = Math.max(best, score(item.group, q) * 0.4)
  if (item.desc) best = Math.max(best, score(item.desc, q) * 0.5)
  for (const a of item.aliases || []) {
    best = Math.max(best, score(a, q))
  }
  return best
}

const filtered = computed(() => {
  const q = query.value
  const empty = !q.trim()
  const scored = props.items
    .filter((it) => (empty ? !it.sidebar : true))
    .map((it) => ({ it, sc: itemScore(it, q) }))
    .filter((x) => x.sc > 0)
    .sort((a, b) => b.sc - a.sc)
  return scored.slice(0, 30).map((x) => x.it)
})

// listbox / combobox 的 aria-activedescendant 指向当前高亮选项。
const activeOptionId = computed(() => (filtered.value.length ? `cp-option-${active.value}` : undefined))

watch(filtered, () => {
  active.value = 0
})

watch(
  () => props.modelValue,
  async (v) => {
    if (v) {
      lastFocused = document.activeElement as HTMLElement | null
      query.value = ''
      active.value = 0
      await nextTick()
      inputRef.value?.focus()
    } else {
      // 关闭后把焦点归还触发者（deep-linking / focus-return）。
      lastFocused?.focus?.()
      lastFocused = null
    }
  },
)

function close() {
  open.value = false
}

function run(item: PaletteItem) {
  close()
  if (item.action) {
    item.action()
    return
  }
  // 命中媒体/种子类关键词时带 query 跳搜索；普通菜单项直达
  if (item.name === 'search' && query.value.trim()) {
    router.push({ name: 'search', query: { q: query.value.trim(), keyword: query.value.trim() } })
    return
  }
  router.push({ name: item.name })
}

function onKeydown(e: KeyboardEvent) {
  if (!open.value) return
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
    return
  }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    if (filtered.value.length) active.value = (active.value + 1) % filtered.value.length
    return
  }
  if (e.key === 'ArrowUp') {
    e.preventDefault()
    if (filtered.value.length) active.value = (active.value - 1 + filtered.value.length) % filtered.value.length
    return
  }
  if (e.key === 'Tab') {
    // 面板内仅输入框可聚焦：Tab 不逃出 dialog（简化焦点陷阱）。
    e.preventDefault()
    inputRef.value?.focus()
    return
  }
  if (e.key === 'Enter') {
    e.preventDefault()
    const it = filtered.value[active.value]
    if (it) run(it)
    else if (query.value.trim()) {
      // 无菜单命中 → 跳搜索工作台
      close()
      router.push({ name: 'search', query: { q: query.value.trim(), keyword: query.value.trim() } })
    }
  }
}

function onGlobalKey(e: KeyboardEvent) {
  // ⌘K / Ctrl+K 切换
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    open.value = !open.value
  }
}

onMounted(() => {
  window.addEventListener('keydown', onGlobalKey)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  window.removeEventListener('keydown', onGlobalKey)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div v-if="open" class="cp-backdrop" @click.self="close">
      <div class="cp-panel solid-card" role="dialog" aria-modal="true" :aria-label="t('cpa.aria')" @click.stop>
        <div class="cp-input-row">
          <SptIcon name="search" :size="18" class="cp-search-icon" aria-hidden="true" />
          <input
            ref="inputRef"
            v-model="query"
            class="cp-input"
            type="search"
            role="combobox"
            aria-controls="cp-list"
            :aria-expanded="true"
            :aria-activedescendant="activeOptionId"
            :placeholder="t('cpa.placeholder')"
            autocomplete="off"
            spellcheck="false"
          />
          <kbd class="cp-kbd">Esc</kbd>
        </div>
        <ul id="cp-list" class="cp-list" role="listbox" :aria-activedescendant="activeOptionId">
          <li
            v-for="(item, i) in filtered"
            :id="'cp-option-' + i"
            :key="item.name + item.label"
            class="cp-item"
            :class="{ active: i === active }"
            role="option"
            :aria-selected="i === active"
            @mouseenter="active = i"
            @click="run(item)"
          >
            <GlassIcon v-if="item.icon" :name="item.icon" :size="28" class="cp-icon" aria-hidden="true" />
            <span v-else class="cp-icon-fallback"><SptIcon name="arrow-right" :size="16" aria-hidden="true" /></span>
            <div class="cp-meta">
              <div class="cp-label">{{ item.label }}</div>
              <div v-if="item.group || item.desc" class="cp-sub">
                <span v-if="item.group">{{ item.group }}</span>
                <span v-if="item.group && item.desc"> · </span>
                <span v-if="item.desc">{{ item.desc }}</span>
              </div>
            </div>
            <span class="cp-hint"><SptIcon name="arrow-right" :size="14" aria-hidden="true" /></span>
          </li>
          <li v-if="!filtered.length" class="cp-empty">
            {{ t('cpa.noMatchPre') }}{{ query }}{{ t('cpa.noMatchPost') }}
          </li>
        </ul>
        <div class="cp-footer">
          <span><kbd>↑↓</kbd> {{ t('cpa.select') }}</span>
          <span><kbd><SptIcon name="arrow-right" :size="12" aria-hidden="true" /></kbd> {{ t('cpa.open') }}</span>
          <span><kbd>Esc</kbd> {{ t('common.close') }}</span>
          <span class="cp-foot-right"><kbd>{{ shortcutKey }}</kbd> {{ t('cpa.toggle') }}</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.cp-backdrop {
  position: fixed;
  inset: 0;
  z-index: var(--z-overlay);
  background: color-mix(in srgb, var(--color-text) 42%, transparent);
  display: flex;
  justify-content: center;
  padding-top: 15vh;
}
.cp-panel {
  width: min(640px, 90vw);
  max-height: min(70vh, 560px);
  display: flex;
  flex-direction: column;
  border-radius: var(--radius-panel);
  overflow: hidden;
  box-shadow: var(--shadow-popover);
}
.cp-input-row {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 14px 16px;
  border-bottom: 1px solid var(--color-border);
}
.cp-search-icon {
  color: var(--color-text-secondary);
}
.cp-input {
  flex: 1;
  border: none;
  background: transparent;
  font-size: var(--fs-lg);
  color: var(--color-text);
}
.cp-input::placeholder {
  color: var(--color-text-muted);
}
.cp-kbd {
  font-family: inherit;
  font-size: var(--fs-xs);
  padding: 2px 6px;
  border-radius: var(--radius-control);
  background: var(--color-surface-2);
  color: var(--color-text-secondary);
  border: 1px solid var(--color-border);
}
.cp-list {
  list-style: none;
  margin: 0;
  padding: 8px;
  overflow-y: auto;
  flex: 1;
  min-height: 120px;
}
.cp-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 10px 12px;
  border-radius: var(--radius-control);
  cursor: pointer;
  color: var(--color-text);
}
.cp-item.active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}
.cp-item.active::before {
  content: '';
  position: absolute;
  left: 0;
  top: 6px;
  bottom: 6px;
  width: 2px;
  border-radius: 0 2px 2px 0;
  background: var(--color-primary);
}
.cp-icon {
  flex-shrink: 0;
}
.cp-icon-fallback {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  color: var(--color-text-muted);
}
.cp-meta {
  flex: 1;
  min-width: 0;
}
.cp-label {
  font-weight: 600;
  font-size: var(--fs-md);
}
.cp-sub {
  font-size: var(--fs-xs);
  color: var(--color-text-secondary);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-hint {
  color: var(--color-text-muted);
  font-size: var(--fs-xs);
}
.cp-empty {
  padding: 24px 12px;
  text-align: center;
  color: var(--color-text-secondary);
  font-size: var(--fs-body);
}
.cp-footer {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 8px 14px;
  font-size: var(--fs-xs);
  color: var(--color-text-secondary);
  border-top: 1px solid var(--color-border);
}
.cp-footer kbd {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-family: inherit;
  font-size: var(--fs-xs);
  padding: 1px 4px;
  border-radius: var(--radius-control);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border);
  color: var(--color-text-secondary);
}
.cp-foot-right {
  margin-left: auto;
}
</style>
