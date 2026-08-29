<script lang="ts">
// 部件目录导出（script setup 不允许值导出，放普通 script 块）。
export interface WidgetDef {
  key: string
  labelKey: string
  descKey: string
  defaultSize: 'compact' | 'wide' | 'focus'
}
export const WIDGET_CATALOG: WidgetDef[] = [
  { key: 'quick_tools', labelKey: 'dwg.wQuickTools', descKey: 'dwg.wQuickToolsDesc', defaultSize: 'wide' },
]
</script>

<script setup lang="ts">
// 首页定制化部件区（对标 MSaber 自定义部件）：从目录添加部件、拖拽排序（HTML5 DnD，
// 移动端用默认顺序兜底）、尺寸预设（紧凑/横向/焦点）、显示/隐藏。
// 布局存用户偏好域 dashboard（useUserPrefs → localStorage + 服务端跨浏览器同步）。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useUserPrefs } from '@/composables/useUserPrefs'
import DashboardQuickTools from './DashboardQuickTools.vue'

const SIZES = ['compact', 'wide', 'focus'] as const

const { prefs, update } = useUserPrefs('dashboard')

interface WidgetSetting { key: string; size: (typeof SIZES)[number]; hidden?: boolean }

const widgets = computed<WidgetSetting[]>(() => {
  const raw = (prefs.value as any)?.widgets
  if (Array.isArray(raw) && raw.length) return raw
  return WIDGET_CATALOG.map((w) => ({ key: w.key, size: w.defaultSize }))
})

const visible = computed(() => widgets.value.filter((w) => !w.hidden))
const catalog = computed(() =>
  WIDGET_CATALOG.map((c) => ({ ...c, on: !widgets.value.find((w) => w.key === c.key)?.hidden })),
)

const customizeOpen = defineModel<boolean>({ default: false })

function persist(next: WidgetSetting[]) {
  update({ widgets: next })
}

function toggleWidget(key: string) {
  const next = widgets.value.map((w) => (w.key === key ? { ...w, hidden: !w.hidden } : w))
  persist(next)
}

function cycleSize(key: string) {
  persist(
    widgets.value.map((w) => {
      if (w.key !== key) return w
      const idx = SIZES.indexOf(w.size)
      return { ...w, size: SIZES[(idx + 1) % SIZES.length] }
    }),
  )
}

// ---- 拖拽排序（HTML5 DnD）----
let dragKey = ''
function onDragStart(key: string) {
  dragKey = key
}
function onDrop(targetKey: string) {
  if (!dragKey || dragKey === targetKey) return
  const list = [...visible.value.map((w) => w.key)]
  const from = list.indexOf(dragKey)
  const to = list.indexOf(targetKey)
  if (from < 0 || to < 0) return
  list.splice(to, 0, list.splice(from, 1)[0])
  // 隐藏的保持原位追加
  const hidden = widgets.value.filter((w) => w.hidden)
  persist([...list.map((k) => widgets.value.find((w) => w.key === k)!), ...hidden])
  dragKey = ''
}
function moveWidget(key: string, dir: -1 | 1) {
  const list = [...visible.value.map((w) => w.key)]
  const i = list.indexOf(key)
  const j = i + dir
  if (i < 0 || j < 0 || j >= list.length) return
  ;[list[i], list[j]] = [list[j], list[i]]
  const hidden = widgets.value.filter((w) => w.hidden)
  persist([...list.map((k) => widgets.value.find((w) => w.key === k)!), ...hidden])
}

function sizeClass(s: string) {
  return `w-${s}`
}

const { t } = useI18n()
const sizeLabel = computed<Record<string, string>>(() => ({ compact: t('dwg.sizeCompact'), wide: t('dwg.sizeWide'), focus: t('dwg.sizeFocus') }))
</script>

<template>
  <div class="widgets-zone">
    <!-- 部件区标题栏：常驻「部件定制」入口（对标 MSaber 右下角首页定制），空态另有引导 -->
    <div class="widgets-bar">
      <span class="widgets-bar-title">{{ t('dwg.barTitle') }}</span>
      <el-button size="small" @click="customizeOpen = true">{{ t('dwg.customize') }}</el-button>
    </div>
    <div v-if="!visible.length" class="widgets-empty glass">
      {{ t('dwg.emptyPre') }}
      <el-button link type="primary" @click="customizeOpen = true">{{ t('dwg.addWidget') }}</el-button>
    </div>
    <div class="widgets-grid">
      <div
        v-for="w in visible"
        :key="w.key"
        class="widget-cell"
        :class="sizeClass(w.size)"
        draggable="true"
        @dragstart="onDragStart(w.key)"
        @dragover.prevent
        @drop.prevent="onDrop(w.key)"
      >
        <DashboardQuickTools v-if="w.key === 'quick_tools'" />
      </div>
    </div>

    <!-- 部件定制弹窗 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="customizeOpen" :title="t('dwg.customizeTitle')" width="480px">
      <div class="cat-list">
        <div v-for="c in catalog" :key="c.key" class="cat-row">
          <div class="cat-info">
            <span class="cat-name">{{ t(c.labelKey) }}</span>
            <span class="cat-desc">{{ t(c.descKey) }}</span>
          </div>
          <div class="cat-ops">
            <el-button
              v-for="w in widgets.filter((x) => x.key === c.key)"
              :key="w.key"
              size="small"
              @click="cycleSize(c.key)"
            >{{ sizeLabel[w.size] }}</el-button>
            <el-button size="small" :type="c.on ? 'danger' : 'primary'" plain @click="toggleWidget(c.key)">
              {{ c.on ? t('common.remove') : t('common.add') }}
            </el-button>
          </div>
        </div>
      </div>
      <div class="cat-hint">
        {{ t('dwg.dragHint') }}
      </div>
    </el-dialog>
  </div>
</template>

<style scoped>
.widgets-zone { display: flex; flex-direction: column; gap: var(--sp-3); }
.widgets-bar { display: flex; align-items: center; justify-content: space-between; }
.widgets-bar-title { font-size: var(--fs-sm); font-weight: 600; color: var(--text-muted); }
.widgets-empty { padding: var(--sp-6); text-align: center; color: var(--text-muted); }
.widgets-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: var(--sp-3); }
.widget-cell { min-width: 0; }
.w-compact { grid-column: span 1; }
.w-wide { grid-column: span 2; }
.w-focus { grid-column: span 2; grid-row: span 2; }
.cat-list { display: flex; flex-direction: column; gap: var(--sp-3); }
.cat-row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
.cat-info { display: flex; flex-direction: column; gap: 2px; }
.cat-name { font-weight: 600; }
.cat-desc { font-size: var(--fs-xs); color: var(--text-muted); }
.cat-ops { display: flex; gap: var(--sp-1); flex-shrink: 0; }
.cat-hint { margin-top: var(--sp-3); font-size: var(--fs-xs); color: var(--text-muted); }
@media (max-width: 900px) {
  .widgets-grid { grid-template-columns: 1fr; }
  .w-compact, .w-wide, .w-focus { grid-column: span 1; grid-row: span 1; }
}
</style>
