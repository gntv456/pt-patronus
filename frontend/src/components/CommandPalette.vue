<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

export interface PaletteItem {
  name: string
  label: string
  icon?: string
  from?: string
  to?: string
  group?: string
  /** 额外别名（PT 黑话等），参与 fuzzy */
  aliases?: string[]
  desc?: string
  /** 自定义动作；无则 router.push({ name }) */
  action?: () => void
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
  const scored = props.items
    .map((it) => ({ it, sc: itemScore(it, q) }))
    .filter((x) => x.sc > 0)
    .sort((a, b) => b.sc - a.sc)
  return scored.slice(0, 12).map((x) => x.it)
})

watch(filtered, () => {
  active.value = 0
})

watch(
  () => props.modelValue,
  async (v) => {
    if (v) {
      query.value = ''
      active.value = 0
      await nextTick()
      inputRef.value?.focus()
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
    router.push({ name: 'search', query: { q: query.value.trim() } })
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
  if (e.key === 'Enter') {
    e.preventDefault()
    const it = filtered.value[active.value]
    if (it) run(it)
    else if (query.value.trim()) {
      // 无菜单命中 → 跳搜索工作台
      close()
      router.push({ name: 'search', query: { q: query.value.trim() } })
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
      <div class="cp-panel glass" role="dialog" aria-modal="true" aria-label="命令面板" @click.stop>
        <div class="cp-input-row">
          <span class="cp-search-icon">🔍</span>
          <input
            ref="inputRef"
            v-model="query"
            class="cp-input"
            type="search"
            placeholder="跳转到… 菜单 / 设置 / PT 黑话（刷流·保级·H&R）"
            autocomplete="off"
            spellcheck="false"
          />
          <kbd class="cp-kbd">Esc</kbd>
        </div>
        <ul class="cp-list" role="listbox">
          <li
            v-for="(item, i) in filtered"
            :key="item.name + (item.label)"
            class="cp-item"
            :class="{ active: i === active }"
            role="option"
            :aria-selected="i === active"
            @mouseenter="active = i"
            @click="run(item)"
          >
            <GlassIcon
              v-if="item.icon"
              :name="item.icon"
              :from="item.from || '#6c5ce7'"
              :to="item.to || '#4285f4'"
              :size="28"
              class="cp-icon"
            />
            <span v-else class="cp-icon-fallback">↗</span>
            <div class="cp-meta">
              <div class="cp-label">{{ item.label }}</div>
              <div v-if="item.group || item.desc" class="cp-sub">
                <span v-if="item.group">{{ item.group }}</span>
                <span v-if="item.group && item.desc"> · </span>
                <span v-if="item.desc">{{ item.desc }}</span>
              </div>
            </div>
            <span class="cp-hint">↵</span>
          </li>
          <li v-if="!filtered.length" class="cp-empty">
            无匹配菜单 · Enter 搜索「{{ query }}」
          </li>
        </ul>
        <div class="cp-footer">
          <span><kbd>↑↓</kbd> 选择</span>
          <span><kbd>↵</kbd> 打开</span>
          <span><kbd>Esc</kbd> 关闭</span>
          <span class="cp-foot-right"><kbd>⌘K</kbd> 切换</span>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.cp-backdrop {
  position: fixed;
  inset: 0;
  z-index: 2000;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(6px);
  display: flex;
  justify-content: center;
  padding-top: 15vh;
}
.cp-panel {
  width: min(640px, 90vw);
  max-height: min(70vh, 560px);
  display: flex;
  flex-direction: column;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.28);
}
.cp-input-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 14px 16px;
  border-bottom: 1px solid rgba(108, 92, 231, 0.12);
}
.cp-search-icon {
  font-size: 16px;
  opacity: 0.7;
}
.cp-input {
  flex: 1;
  border: none;
  outline: none;
  background: transparent;
  font-size: 16px;
  color: var(--text-strong);
}
.cp-input::placeholder {
  color: var(--text-muted);
  opacity: 0.7;
}
.cp-kbd {
  font-family: inherit;
  font-size: 11px;
  padding: 2px 6px;
  border-radius: 6px;
  background: rgba(108, 92, 231, 0.1);
  color: var(--text-muted);
  border: 1px solid rgba(108, 92, 231, 0.15);
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
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  cursor: pointer;
  color: var(--text-strong);
}
.cp-item.active {
  background: linear-gradient(135deg, rgba(108, 92, 231, 0.22), rgba(66, 133, 244, 0.14));
}
.cp-icon {
  flex-shrink: 0;
}
.cp-icon-fallback {
  width: 28px;
  height: 28px;
  display: grid;
  place-items: center;
  opacity: 0.5;
}
.cp-meta {
  flex: 1;
  min-width: 0;
}
.cp-label {
  font-weight: 600;
  font-size: 14px;
}
.cp-sub {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.cp-hint {
  opacity: 0.35;
  font-size: 12px;
}
.cp-empty {
  padding: 24px 12px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.cp-footer {
  display: flex;
  gap: 14px;
  padding: 8px 14px;
  font-size: 11px;
  color: var(--text-muted);
  border-top: 1px solid rgba(108, 92, 231, 0.1);
}
.cp-footer kbd {
  font-family: inherit;
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 4px;
  background: rgba(108, 92, 231, 0.08);
  border: 1px solid rgba(108, 92, 231, 0.12);
}
.cp-foot-right {
  margin-left: auto;
}
</style>
