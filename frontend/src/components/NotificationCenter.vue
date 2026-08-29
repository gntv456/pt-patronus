<script setup lang="ts">
// NotificationCenter — 全局通知中心铃铛（topbar 内）。
// 通知数据由 useNotices 共享单例管理（与 Notices 全页面 / AppSidebar 同源）；下拉列表分组折叠。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useNotices } from '@/composables/useNotices'
import type { Notice, NoticeSource } from '@/api/notice'
import SptIcon from '@/components/SptIcon.vue'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

const router = useRouter()
const { notices, unread, urgentUnread, load, syncFromWs, readOne: readOneApi, readAll: readAllApi, removeOne: removeOneApi, clearAll: clearAllApi } = useNotices()
const open = ref(false)
const boxRef = ref<HTMLElement | null>(null)

const sourceMeta: Record<NoticeSource, { icon: string; color: string; label: string }> = {
  site_anniv: { icon: 'party-popper', color: '#f59e0b', label: t('notices.source.site_anniv') },
  new_mail: { icon: 'mail', color: '#3b82f6', label: t('notices.source.new_mail') },
  assess_pass: { icon: 'check-circle', color: '#22c55e', label: t('notices.source.assess_pass') },
  assess_fail: { icon: 'alert-circle', color: '#ef4444', label: t('notices.source.assess_fail') },
  announce: { icon: 'megaphone', color: '#3b82f6', label: t('notices.source.announce') },
  host_offline: { icon: 'alert-triangle', color: '#ef4444', label: t('notices.source.host_offline') },
  nas_offline: { icon: 'alert-triangle', color: '#ef4444', label: t('notices.source.nas_offline') },
  reseed_done: { icon: 'check-circle', color: '#22c55e', label: t('notices.source.reseed_done') },
  dl_done: { icon: 'download', color: '#22c55e', label: t('notices.source.dl_done') },
  site_offline: { icon: 'alert-triangle', color: '#ef4444', label: t('notices.source.site_offline') },
  backup_done: { icon: 'check-circle', color: '#22c55e', label: t('notices.source.backup_done') },
  backup_import: { icon: 'check-circle', color: '#22c55e', label: t('notices.source.backup_import') },
  freeleech: { icon: 'party-popper', color: '#f59e0b', label: 'Freeleech' },
}
function meta(s: NoticeSource) {
  return sourceMeta[s] ?? { ...sourceMeta.announce, label: t('notices.source.fallback') }
}
const badgeText = computed(() => {
  const n = urgentUnread.value || unread.value
  return n > 99 ? '99+' : String(n)
})

// 通知来源 → 路由名映射，点击通知体跳转到对应页面。
const sourceRoute: Record<NoticeSource, string> = {
  new_mail: 'messages',
  assess_pass: 'assessment',
  assess_fail: 'assessment',
  dl_done: 'downloaders',
  reseed_done: 'reseed',
  site_anniv: 'sites',
  site_offline: 'nas-server',
  host_offline: 'nas-server',
  nas_offline: 'nas-server',
  freeleech: 'freeleech',
  announce: 'announcements',
  backup_done: 'backup',
  backup_import: 'backup',
}
onMounted(() => {
  load()
  document.addEventListener('click', onDocClick)
})
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))

function onDocClick(e: MouseEvent) {
  if (open.value && boxRef.value && !boxRef.value.contains(e.target as Node)) open.value = false
}

// WS 实时推送：新通知由共享单例合并
watch(() => notices.value[0]?.id, () => syncFromWs())
async function readOne(n: Notice) { await readOneApi(n.id) }
async function readAll() { await readAllApi() }
async function removeOne(n: Notice) { await removeOneApi(n.id) }
async function clearAll() {
  try {
    await ElMessageBox.confirm(t('nc.clearConfirm'), t('nc.clearTitle'), {
      type: 'warning',
      confirmButtonText: t('nc.clearGo'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return // 用户取消
  }
  await clearAllApi()
}

// 点击通知体：标已读 + 按 source 跳转对应页面
async function onBodyClick(n: Notice) {
  if (!n.read) readOne(n)
  const routeName = sourceRoute[n.source]
  if (routeName) {
    open.value = false
    router.push({ name: routeName })
  }
}

// 分组折叠：按 source 聚合，每组显示新图标 + 类型名 + 数量徽标 + 最新时间。点击组头展开/收起。
const collapsed = ref(true) // 默认全部折叠（只显示汇总）
const expandedGroups = ref<Set<string>>(new Set()) // 展开的组 key（以 source 为 key）

interface Group {
  source: NoticeSource
  items: Notice[]
  latest: Notice
  unreadCount: number
}
const grouped = computed<Group[]>(() => {
  const map = new Map<NoticeSource, Notice[]>()
  for (const n of notices.value) {
    if (!map.has(n.source)) map.set(n.source, [])
    map.get(n.source)!.push(n)
  }
  const out: Group[] = []
  for (const [source, items] of map) {
    // 同组内按时间倒序，latest 为最新一条
    items.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    out.push({
      source,
      items,
      latest: items[0],
      unreadCount: items.filter((n) => !n.read).length,
    })
  }
  // 组之间也按 latest 时间倒序
  out.sort((a, b) => new Date(b.latest.created_at).getTime() - new Date(a.latest.created_at).getTime())
  return out
})

function isGroupExpanded(source: NoticeSource): boolean {
  return expandedGroups.value.has(source)
}
function toggleGroup(source: NoticeSource) {
  const s = new Set(expandedGroups.value)
  if (s.has(source)) s.delete(source)
  else s.add(source)
  expandedGroups.value = s
}

function relTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return t('notices.relTime.justNow')
  if (diff < 3600) return t('notices.relTime.minutesAgo', { n: Math.floor(diff / 60) })
  if (diff < 86400) return t('notices.relTime.hoursAgo', { n: Math.floor(diff / 3600) })
  return t('notices.relTime.daysAgo', { n: Math.floor(diff / 86400) })
}

// 折叠展开高度过渡（Vue <Transition> JS hooks）
function onGroupEnter(el: Element) {
  const e = el as HTMLElement
  e.style.height = "0"
  e.style.overflow = "hidden"
  void e.offsetHeight
  e.style.transition = "height var(--dur-base) var(--ease-out-smooth), opacity var(--dur-base) var(--ease-out-smooth)"
  e.style.height = e.scrollHeight ? e.scrollHeight + "px" : "auto"
  e.style.opacity = "1"
}
function onGroupAfterEnter(el: Element) {
  const e = el as HTMLElement
  e.style.height = ""
  e.style.overflow = ""
  e.style.transition = ""
}
function onGroupLeave(el: Element) {
  const e = el as HTMLElement
  e.style.height = e.scrollHeight + "px"
  e.style.overflow = "hidden"
  void e.offsetHeight
  e.style.transition = "height var(--dur-base) var(--ease-out-smooth), opacity var(--dur-base) var(--ease-out-smooth)"
  e.style.height = "0"
  e.style.opacity = "0"
}
</script>

<template>
  <div ref="boxRef" class="nc">
    <button
      class="nc-bell"
      :class="{ 'nc-bell--has': unread > 0, 'nc-bell--urgent': urgentUnread > 0 }"
      :title="urgentUnread ? t('nc.urgentN', { n: urgentUnread }) : unread ? t('nc.unreadN', { n: unread }) : t('nc.title')"
      @click.stop="open = !open"
    >
      <span class="nc-bell-ic" :class="{ 'nc-bell-ic--ring': unread > 0 }">
        <SptIcon name="bell" :size="20" />
      </span>
      <span v-if="unread > 0" class="nc-badge">{{ badgeText }}</span>
    </button>

    <Transition name="nc-slide">
      <div v-if="open" class="nc-panel glass">
        <div class="nc-head">
          <span class="nc-title">{{ t('nc.title') }} <small v-if="unread">{{ t('nc.unread') }} {{ unread }}<template v-if="urgentUnread"> · {{ t('nc.urgent') }} {{ urgentUnread }}</template></small></span>
          <div class="nc-actions">
            <router-link :to="{ name: 'notices' }" class="nc-action" @click="open = false">{{ t('nc.viewAll') }}</router-link>
            <button v-if="unread > 0" class="nc-action" @click="readAll">{{ t('nc.readAll') }}</button>
            <button v-if="notices.length" class="nc-action nc-action--danger" @click="clearAll">{{ t('nc.clearGo') }}</button>
          </div>
        </div>
        <div class="nc-list">
          <div v-if="!notices.length" class="nc-empty">{{ t('nc.empty') }}</div>
          <div v-for="g in grouped" :key="g.source" class="nc-group-wrap">
            <!-- 组头：折叠时显示彩色图标 + 类型名 + 未读 pill 徽标 + 最新时间 -->
            <div
              class="nc-group"
              :class="{ 'nc-group--unread': g.unreadCount > 0 }"
              @click="toggleGroup(g.source)"
            >
              <span class="nc-group-ic" :style="{ color: '#fff', background: meta(g.source).color }">
                <SptIcon :name="meta(g.source).icon" :size="18" />
              </span>
              <div class="nc-group-body">
                <div class="nc-row">
                  <span class="nc-group-name">{{ meta(g.source).label }}<span class="nc-count"> · {{ g.items.length }}</span></span>
                  <span v-if="g.unreadCount > 0" class="nc-unread-pill" :style="{ background: meta(g.source).color }">{{ g.unreadCount }}</span>
                </div>
                <div class="nc-text">{{ g.unreadCount > 0 ? t('nc.nUnread', { n: g.unreadCount }) : '' }}{{ g.latest.title }}</div>
                <div class="nc-time nc-group-time">{{ relTime(g.latest.created_at) }}</div>
              </div>
              <span class="nc-chevron" :class="{ 'nc-chevron--open': isGroupExpanded(g.source) }">
                <SptIcon name="chevron-down" :size="14" />
              </span>
            </div>
            <!-- 展开后：逐条列出（带折叠动画） -->
            <Transition
              v-if="isGroupExpanded(g.source)"
              :css="false"
              @enter="onGroupEnter"
              @after-enter="onGroupAfterEnter"
              @leave="onGroupLeave"
            >
              <div class="nc-group-items">
                <div
                  v-for="n in g.items"
                  :key="n.id"
                  class="nc-item"
                  :class="{ 'nc-item--unread': !n.read }"
                >
                  <span class="nc-ic nc-ic--sub" :style="{ color: meta(n.source).color, background: meta(n.source).color + '1f' }">
                    <SptIcon :name="meta(n.source).icon" :size="14" />
                  </span>
                  <div class="nc-body" @click="onBodyClick(n)">
                    <div class="nc-row">
                      <span class="nc-name">{{ n.title }}</span>
                      <span class="nc-time">{{ relTime(n.created_at) }}</span>
                    </div>
                    <div v-if="n.body" class="nc-text">{{ n.body }}</div>
                  </div>
                  <button class="nc-del" :title="t('common.delete')" @click.stop="removeOne(n)">
                    <SptIcon name="trash" :size="14" />
                  </button>
                </div>
              </div>
            </Transition>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.nc {
  position: relative;
  display: inline-flex;
}
.nc-bell {
  position: relative;
  background: transparent;
  border: none;
  cursor: pointer;
  color: var(--text-strong);
  padding: 6px 8px;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  transition: background var(--dur-base) var(--ease-out-smooth);
}
.nc-bell:hover {
  background: var(--pt-primary-bg);
}
.nc-bell--has {
  color: var(--text-strong);
}
.nc-bell--urgent {
  color: var(--pt-danger);
}
.nc-badge {
  position: absolute;
  top: 0;
  right: 0;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: var(--radius-pill);
  background: var(--text-muted);
  color: #fff;
  font-size: var(--fs-2xs);
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  box-shadow: 0 0 0 2px var(--glass-bg-strong, rgba(255, 255, 255, 0.72));
}
.nc-bell--urgent .nc-badge {
  background: #e74c3c;
}
.nc-panel {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 340px;
  max-width: 90vw;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  z-index: 300;
  /* 磨砂玻璃（与命令面板同体系）。下拉面板无全屏遮罩打底，背景要比命令面板更实，
     否则背后复杂图文透出会压低文字对比、看不清字。 */
  border-radius: var(--radius-md);
  background: var(--glass-bg-strong);
  backdrop-filter: blur(calc(var(--blur) + 8px)) saturate(180%);
  -webkit-backdrop-filter: blur(calc(var(--blur) + 8px)) saturate(180%);
  box-shadow: var(--glass-hover-shadow), 0 18px 40px rgba(15, 23, 42, 0.18);
}
.nc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--divider);
  background: var(--surface-muted);
}
.nc-title {
  font-weight: 700;
  font-size: var(--fs-md);
}
.nc-title small {
  margin-left: 6px;
  color: var(--text-muted);
  font-weight: 500;
}
.nc-action {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--pt-primary);
  font-size: var(--fs-xs);
}
.nc-action:hover {
  text-decoration: underline;
}
.nc-actions {
  display: inline-flex;
  align-items: center;
  gap: var(--sp-3);
}
.nc-action--danger {
  color: var(--pt-danger);
}
.nc-list {
  overflow-y: auto;
  flex: 1;
}
.nc-empty {
  padding: 36px;
  text-align: center;
  color: var(--text-muted);
  font-size: var(--fs-body);
}
.nc-item {
  display: flex;
  align-items: flex-start;
  gap: var(--sp-2);
  padding: 10px 14px;
  border-bottom: 1px solid var(--divider);
  transition: background var(--dur-fast) var(--ease-out-smooth);
}
.nc-item:hover {
  background: var(--surface-hover);
}
.nc-item--unread {
  background: var(--pt-primary-bg);
}
.nc-ic {
  flex: none;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-xs);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.nc-body {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}
.nc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-2);
}
.nc-name {
  font-size: var(--fs-body);
  font-weight: 600;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nc-time {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
  flex: none;
}
.nc-text {
  font-size: var(--fs-xs);
  color: var(--text-strong);
  opacity: 0.82;
  margin-top: 2px;
  line-height: 1.45;
  word-break: break-word;
}
.nc-del {
  flex: none;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-muted);
  padding: 2px;
  opacity: 0.5;
  transition: opacity var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast);
}
.nc-del:hover {
  opacity: 1;
  color: var(--pt-danger);
}
.nc-slide-enter-active,
.nc-slide-leave-active {
  transition: all var(--dur-base) var(--ease-out-smooth);
}
.nc-slide-enter-from,
.nc-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

/* 分组折叠样式 */
.nc-group-wrap { border-bottom: 1px solid var(--divider); }
.nc-group {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: 12px 14px;
  cursor: pointer;
  transition: background var(--dur-fast) var(--ease-out-smooth);
}
.nc-group:hover { background: var(--surface-hover); }
.nc-group--unread { background: var(--pt-primary-bg); }
.nc-group-body { flex: 1; min-width: 0; }
.nc-group-ic {
  flex: none;
  width: 36px;
  height: 36px;
  border-radius: var(--radius-sm);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.12);
}
.nc-group-name { font-size: var(--fs-md); font-weight: 700; color: var(--text-strong); }
.nc-group-time { margin-top: 2px; font-size: var(--fs-2xs); }
.nc-unread-pill {
  flex: none;
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  color: #fff;
  font-size: var(--fs-2xs);
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  box-shadow: 0 1px 4px rgba(15, 23, 42, 0.2);
}
.nc-count { color: var(--text-muted); font-weight: 600; font-size: var(--fs-xs); }
.nc-chevron {
  flex: none;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
  transition: transform var(--dur-base) var(--ease-out-smooth);
  margin-top: 6px;
}
.nc-chevron--open { transform: rotate(180deg); }
.nc-group-items {
  background: var(--surface-muted, rgba(0,0,0,0.02));
}
.nc-group-items .nc-item { padding-left: 50px; }
.nc-ic--sub { width: 26px; height: 26px; opacity: 0.85; }

/* 铃铛图标摇晃动画（有未读时） */
.nc-bell-ic {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  transform-origin: top center;
}
.nc-bell-ic--ring {
  animation: nc-bell-ring 2s ease-in-out infinite;
}
.nc-bell--urgent .nc-bell-ic--ring {
  animation-duration: 1.2s;
}
@keyframes nc-bell-ring {
  0%, 65%, 100% { transform: rotate(0deg); }
  70% { transform: rotate(14deg); }
  76% { transform: rotate(-14deg); }
  82% { transform: rotate(10deg); }
  88% { transform: rotate(-10deg); }
  94% { transform: rotate(4deg); }
}
@media (prefers-reduced-motion: reduce) {
  .nc-bell-ic--ring { animation: none; }
}
</style>
