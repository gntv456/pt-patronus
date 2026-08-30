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
const {
  notices,
  unread,
  urgentUnread,
  load,
  syncFromWs,
  readOne: readOneApi,
  readAll: readAllApi,
  removeOne: removeOneApi,
  clearAll: clearAllApi,
} = useNotices()
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
watch(
  () => notices.value[0]?.id,
  () => syncFromWs(),
)
async function readOne(n: Notice) {
  await readOneApi(n.id)
}
async function readAll() {
  await readAllApi()
}
async function removeOne(n: Notice) {
  await removeOneApi(n.id)
}
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
  e.style.height = '0'
  e.style.overflow = 'hidden'
  void e.offsetHeight
  e.style.transition = 'height var(--dur-base) var(--ease-out-smooth), opacity var(--dur-base) var(--ease-out-smooth)'
  e.style.height = e.scrollHeight ? e.scrollHeight + 'px' : 'auto'
  e.style.opacity = '1'
}
function onGroupAfterEnter(el: Element) {
  const e = el as HTMLElement
  e.style.height = ''
  e.style.overflow = ''
  e.style.transition = ''
}
function onGroupLeave(el: Element) {
  const e = el as HTMLElement
  e.style.height = e.scrollHeight + 'px'
  e.style.overflow = 'hidden'
  void e.offsetHeight
  e.style.transition = 'height var(--dur-base) var(--ease-out-smooth), opacity var(--dur-base) var(--ease-out-smooth)'
  e.style.height = '0'
  e.style.opacity = '0'
}
</script>

<template>
  <div ref="boxRef" class="nc">
    <button
      class="nc-bell"
      :class="{ 'nc-bell--has': unread > 0, 'nc-bell--urgent': urgentUnread > 0 }"
      :title="
        urgentUnread ? t('nc.urgentN', { n: urgentUnread }) : unread ? t('nc.unreadN', { n: unread }) : t('nc.title')
      "
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
          <span class="nc-title"
            >{{ t('nc.title') }}
            <small v-if="unread"
              >{{ t('nc.unread') }} {{ unread
              }}<template v-if="urgentUnread"> · {{ t('nc.urgent') }} {{ urgentUnread }}</template></small
            ></span
          >
          <div class="nc-actions">
            <router-link :to="{ name: 'notices' }" class="nc-action" @click="open = false">{{
              t('nc.viewAll')
            }}</router-link>
            <button v-if="unread > 0" class="nc-action" @click="readAll">{{ t('nc.readAll') }}</button>
            <button v-if="notices.length" class="nc-action nc-action--danger" @click="clearAll">
              {{ t('nc.clearGo') }}
            </button>
          </div>
        </div>
        <div class="nc-list">
          <div v-if="!notices.length" class="nc-empty">{{ t('nc.empty') }}</div>
          <div v-for="g in grouped" :key="g.source" class="nc-group-wrap">
            <!-- 组头：折叠时显示彩色图标 + 类型名 + 未读 pill 徽标 + 最新时间 -->
            <div
              v-a11y-button
              class="nc-group"
              :class="{ 'nc-group--unread': g.unreadCount > 0 }"
              @click="toggleGroup(g.source)"
            >
              <span class="nc-group-ic" :style="{ color: '#fff', background: meta(g.source).color }">
                <SptIcon :name="meta(g.source).icon" :size="18" />
              </span>
              <div class="nc-group-body">
                <div class="nc-row">
                  <span class="nc-group-name"
                    >{{ meta(g.source).label }}<span class="nc-count"> · {{ g.items.length }}</span></span
                  >
                  <span v-if="g.unreadCount > 0" class="nc-unread-pill" :style="{ background: meta(g.source).color }">{{
                    g.unreadCount
                  }}</span>
                </div>
                <div class="nc-text">
                  {{ g.unreadCount > 0 ? t('nc.nUnread', { n: g.unreadCount }) : '' }}{{ g.latest.title }}
                </div>
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
                <div v-for="n in g.items" :key="n.id" class="nc-item" :class="{ 'nc-item--unread': !n.read }">
                  <span
                    class="nc-ic nc-ic--sub"
                    :style="{ color: meta(n.source).color, background: meta(n.source).color + '1f' }"
                  >
                    <SptIcon :name="meta(n.source).icon" :size="14" />
                  </span>
                  <div v-a11y-button class="nc-body" @click="onBodyClick(n)">
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

<style scoped src="./NotificationCenter.css"></style>
