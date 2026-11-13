<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// NotificationCenter — 全局通知中心铃铛（topbar 内）。
// 通知数据由 useNotices 共享单例管理（与 Notices 全页面 / AppSidebar 同源）；下拉列表分组折叠。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useNotices } from '@/composables/useNotices'
import { applyGroupRead, groupReadNoticeKey } from '@/utils/noticeGroupRead'
import { useWs } from '@/composables/useWs'
import { useNoticeMeta, noticeSourceRoute } from '@/composables/useNoticeMeta'
import type { Notice, NoticeSource } from '@/api/notice'
import SptIcon from '@/components/SptIcon.vue'
import { useFeedback } from '@/composables/useFeedback'
import { useI18n } from 'vue-i18n'

const { t } = useI18n()
const feedback = useFeedback()

const router = useRouter()
const {
  notices,
  loadFailed,
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

const { meta } = useNoticeMeta()
const badgeText = computed(() => {
  const n = urgentUnread.value || unread.value
  return n > 999 ? '999+' : String(n)
})

// 通知来源 → 路由名映射，点击通知体跳转到对应页面。
const sourceRoute = noticeSourceRoute
onMounted(() => {
  load()
  document.addEventListener('click', onDocClick)
})
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))

function onDocClick(e: MouseEvent) {
  if (open.value && boxRef.value && !boxRef.value.contains(e.target as Node)) open.value = false
}

// WS 实时推送：watch 源必须是 useWs 的 wsNotices（同步进共享单例后再触发铃铛合并），
// 不能 watch 单例里的 notices.value[0]?.id——那是合并后的结果，WS 新帧不改变其首条 id。
const { notices: wsNotices } = useWs()
watch(
  () => wsNotices.value[0]?.id,
  () => syncFromWs(),
)
async function readOne(n: Notice) {
  await readOneApi(n.id)
}
async function readAll() {
  if (!unread.value) return
  const ok = await feedback.confirm({
    title: t('nc.readAll'),
    detail: t('nc.confirmReadAll'),
  })
  if (!ok) return
  // 服务端无「恢复未读」接口：撤销在本次会话内回填本地状态，刷新后以服务端为准。
  const snapshot = new Map<number, boolean>(
    notices.value.map((n): [number, boolean] => [n.id, n.read]),
  )
  await readAllApi()
  feedback.undo(t('notices.allReadOk'), () => {
    notices.value.forEach((n) => {
      const prev = snapshot.get(n.id)
      if (prev !== undefined) n.read = prev
    })
  })
}
async function removeOne(n: Notice) {
  const ok = await feedback.confirm({
    title: t('common.delete'),
    detail: t('nc.confirmDelete', { title: n.title }),
    danger: true,
  })
  if (!ok) return
  await removeOneApi(n.id)
  // 服务端无「恢复单条」接口：撤销在本次会话内回填本地列表，刷新后以服务端为准。
  feedback.undo(t('common.removed'), () => {
    if (!notices.value.some((x) => x.id === n.id)) notices.value = [n, ...notices.value]
  })
}
async function clearAll() {
  const ok = await feedback.confirm({
    title: t('notices.clearTitle'),
    detail: t('notices.clearConfirm'),
    danger: true,
  })
  if (!ok) return
  const snapshot = notices.value.map((n) => ({ ...n }))
  await clearAllApi()
  feedback.undo(t('notices.cleared'), () => {
    notices.value = snapshot.map((n) => ({ ...n }))
  })
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

// 组头一键已读：不用先展开再逐条处理（高频场景：47 条站点离线批量确认）
async function readGroup(g: Group) {
  const pending = g.items.filter((n) => !n.read)
  if (!pending.length) return
  // readOne 内部吞掉失败（不 reject），所以 Promise.all 的结果必须逐条取回来判断：
  // 从前这里无条件把整组标成已读、未读清零，四条全失败也显示「本组已读完」（#294）。
  const results = await Promise.all(pending.map((n) => readOneApi(n.id, true)))
  const outcome = applyGroupRead(pending, results, g.unreadCount)
  g.unreadCount = outcome.unreadLeft
  const kind = groupReadNoticeKey(outcome)
  if (kind !== 'all') {
    ElMessage.warning(t(kind === 'none' ? 'nc.groupReadAllFailed' : 'nc.groupReadPartial', { n: outcome.failed }))
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
  e.style.transition = 'height var(--dur-base) var(--ease), opacity var(--dur-base) var(--ease)'
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
  e.style.transition = 'height var(--dur-base) var(--ease), opacity var(--dur-base) var(--ease)'
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
        urgentUnread ? t('nc.urgentN', { n: urgentUnread }) : unread ? t('nc.unreadN', { n: unread }) : t('notices.title')
      "
      :aria-label="
        urgentUnread ? t('nc.urgentN', { n: urgentUnread }) : unread ? t('nc.unreadN', { n: unread }) : t('notices.title')
      "
      :aria-expanded="open"
      @click.stop="open = !open"
    >
      <span class="nc-bell-ic">
        <SptIcon name="bell" :size="20" />
      </span>
      <span v-if="unread > 0" class="nc-badge">{{ badgeText }}</span>
    </button>

    <Transition name="nc-slide">
      <div v-if="open" class="nc-panel solid-card">
        <div class="nc-head">
          <span class="nc-title"
            >{{ t('notices.title') }}
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
          <div v-if="!notices.length" class="nc-empty">
            {{ loadFailed ? t('notices.loadFailed') : t('notices.emptyTitle') }}
          </div>
          <div v-for="g in grouped" :key="g.source" class="nc-group-wrap">
            <!-- 组头：折叠时显示彩色图标 + 类型名 + 未读 pill 徽标 + 最新时间 -->
            <div
              v-a11y-button
              class="nc-group"
              :class="{ 'nc-group--unread': g.unreadCount > 0 }"
              @click="toggleGroup(g.source)"
            >
              <span class="nc-group-ic" :style="{ color: meta(g.source).color, background: 'var(--color-surface-2)' }">
                <SptIcon :name="meta(g.source).icon" :size="18" />
              </span>
              <div class="nc-group-body">
                <div class="nc-row">
                  <span class="nc-group-name"
                    >{{ meta(g.source).label }}<span class="nc-count"> · {{ g.items.length }}</span></span
                  >
                  <span v-if="g.unreadCount > 0" class="nc-unread-pill">{{ g.unreadCount }}</span>
                </div>
                <div class="nc-text">
                  {{ g.unreadCount > 0 ? t('nc.nUnread', { n: g.unreadCount }) : '' }}{{ g.latest.title }}
                </div>
                <div class="nc-time nc-group-time">{{ relTime(g.latest.created_at) }}</div>
              </div>
              <span class="nc-chevron" :class="{ 'nc-chevron--open': isGroupExpanded(g.source) }">
                <SptIcon name="chevron-down" :size="14" />
              </span>
              <button
                v-if="g.unreadCount > 0"
                class="nc-group-read"
                :title="t('notices.markGroupRead')"
                :aria-label="t('notices.markGroupRead')"
                @click.stop="readGroup(g)"
              >
                <SptIcon name="check-circle" :size="14" />
              </button>
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
                    :style="{ color: meta(n.source).color, background: 'var(--color-surface-2)' }"
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
                  <button class="nc-del" :title="t('common.delete')" :aria-label="t('common.delete')" @click.stop="removeOne(n)">
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
