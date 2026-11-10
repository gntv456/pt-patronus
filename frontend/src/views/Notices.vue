<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 通知中心（全页面）：按来源类型分组并折叠，点击展开查看每条详情。
// 与 topbar 铃铛下拉（NotificationCenter.vue）共用 sourceMeta / 路由映射，体验一致。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useWs } from '@/composables/useWs'
import { noticeFailReason } from '@/composables/useNotices'
import { useNoticeMeta, noticeSourceRoute } from '@/composables/useNoticeMeta'
import { noticeApi, type Notice, type NoticeSource } from '@/api/notice'
import SptIcon from '@/components/SptIcon.vue'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const router = useRouter()
const { notices: wsNotices } = useWs()
const notices = ref<Notice[]>([])
const loading = ref(false)
// 取数窗口：后端一次只给最近 limit 条，被截断时必须说出来，
// 否则「最近 200 条」被读成整个收件箱，超出部分的未读永远看不见。
const inboxWindow = ref<{ shown: number; total?: number; truncated: boolean }>({
  shown: 0,
  total: undefined,
  truncated: false,
})

// 来源类型元信息（图标 / 配色 / 中文名）
const { meta } = useNoticeMeta()

// 点击通知体后跳转的目标路由（按来源分类）
const sourceRoute = noticeSourceRoute

// 筛选：全部 / 仅未读
const filter = ref<'all' | 'unread'>('all')
const onlyUnread = computed(() => filter.value === 'unread')

const unreadCount = computed(() => notices.value.filter((n) => !n.read).length)
const urgentSources = new Set<NoticeSource>(['assess_fail', 'host_offline', 'nas_offline', 'camera_offline', 'site_offline'])
const urgentUnread = computed(() => notices.value.filter((n) => !n.read && urgentSources.has(n.source)).length)

// 分组折叠：按 source 聚合
const expandedGroups = ref<Set<string>>(new Set())

interface Group {
  source: NoticeSource
  items: Notice[]
  latest: Notice
  unreadCount: number
}
const grouped = computed<Group[]>(() => {
  const pool = onlyUnread.value ? notices.value.filter((n) => !n.read) : notices.value
  const map = new Map<NoticeSource, Notice[]>()
  for (const n of pool) {
    if (!map.has(n.source)) map.set(n.source, [])
    map.get(n.source)!.push(n)
  }
  const out: Group[] = []
  for (const [source, items] of map) {
    items.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    out.push({ source, items, latest: items[0], unreadCount: items.filter((n) => !n.read).length })
  }
  out.sort((a, b) => new Date(b.latest.created_at).getTime() - new Date(a.latest.created_at).getTime())
  return out
})

function isGroupExpanded(source: string) {
  return expandedGroups.value.has(source)
}
function toggleGroup(source: string) {
  const s = new Set(expandedGroups.value)
  if (s.has(source)) s.delete(source)
  else s.add(source)
  expandedGroups.value = s
}

async function load() {
  loading.value = true
  try {
    const page = await noticeApi.listPage()
    notices.value = page.notices ?? []
    inboxWindow.value = {
      shown: notices.value.length,
      total: page.total,
      truncated: page.truncated,
    }
  } catch {
    ElMessage.warning(t('notices.loadFailed'))
  } finally {
    loading.value = false
  }
}

// WS 实时推送：新通知 prepend 到本地列表
watch(
  () => wsNotices.value[0]?.id,
  () => {
    const n = wsNotices.value[0]
    if (n && !notices.value.some((x) => x.id === n.id)) notices.value.unshift(n)
  },
)

// 后端会把原因写进信封（404 说「通知不存在，可能已被删除，请刷新通知列表」），
// 从前这四个处置出口一律 `catch { /* 静默 */ }`：用户点了没反应，只会反复点。
function failReason(e: unknown): string {
  return noticeFailReason(e, t('notices.actionFailed'))
}

// 逐条结算：一条失败不能带走整组（旧写法是 Promise.all，一条 404 就让剩下的原地不动，
// 且因为异常被 catch 吞掉，连「哪条没删掉」都不说）。口径同下载器批量表（#43/#44）。
async function settleAll(targets: Notice[], run: (n: Notice) => Promise<unknown>, onOk: (n: Notice) => void) {
  if (!targets.length) return
  const rs = await Promise.allSettled(targets.map((n) => run(n)))
  let failed = 0
  let reason = ''
  rs.forEach((r, i) => {
    if (r.status === 'fulfilled') {
      onOk(targets[i])
    } else {
      failed++
      if (!reason) reason = failReason(r.reason)
    }
  })
  if (failed > 0) ElMessage.warning(t('notices.partialFailed', { n: failed, reason }))
}

async function readOne(n: Notice) {
  try {
    await noticeApi.read(n.id)
    n.read = true
  } catch (e: unknown) {
    ElMessage.warning(failReason(e))
  }
}
async function readGroup(g: Group) {
  await settleAll(
    g.items.filter((n) => !n.read),
    (n) => noticeApi.read(n.id),
    (n) => {
      n.read = true
    },
  )
}
async function readAll() {
  try {
    await noticeApi.readAll()
    notices.value.forEach((n) => (n.read = true))
    ElMessage.success(t('notices.allReadOk'))
  } catch {
    ElMessage.warning(t('notices.actionFailed'))
  }
}
async function removeOne(n: Notice) {
  try {
    await noticeApi.delete(n.id)
    notices.value = notices.value.filter((x) => x.id !== n.id)
  } catch (e: unknown) {
    ElMessage.error(failReason(e))
  }
}
async function removeGroup(g: Group) {
  const gone: number[] = []
  await settleAll(
    g.items,
    (n) => noticeApi.delete(n.id),
    (n) => {
      gone.push(n.id)
    },
  )
  if (gone.length) {
    const ids = new Set(gone)
    notices.value = notices.value.filter((n) => !ids.has(n.id))
  }
}
async function clearAll() {
  try {
    await ElMessageBox.confirm(t('notices.clearConfirm'), t('notices.clearTitle'), {
      type: 'warning',
      confirmButtonText: t('common.clear'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await noticeApi.clear()
    notices.value = []
    ElMessage.success(t('notices.cleared'))
  } catch {
    ElMessage.warning(t('notices.clearFailed'))
  }
}

// 点击通知体：标已读 + 跳转对应页面
function onBodyClick(n: Notice) {
  if (!n.read) readOne(n)
  const routeName = sourceRoute[n.source]
  if (routeName) router.push({ name: routeName })
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

onMounted(load)
</script>

<template>
  <div class="page" v-loading="loading && !notices.length">
    <div class="head">
      <h2 class="title">
        {{ t('notices.title') }}
        <small v-if="unreadCount"
          >{{ t('notices.unread', { n: unreadCount })
          }}<template v-if="urgentUnread"> · {{ t('notices.urgent', { n: urgentUnread }) }}</template></small
        >
      </h2>
      <div class="head-tools">
        <div class="filter glass">
          <button class="filter-btn" :class="{ on: filter === 'all' }" @click="filter = 'all'">
            {{ t('notices.filter.all') }}
          </button>
          <button class="filter-btn" :class="{ on: filter === 'unread' }" @click="filter = 'unread'">
            {{ t('notices.filter.unread') }}<span v-if="unreadCount" class="filter-badge">{{ unreadCount }}</span>
          </button>
        </div>
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button v-if="unreadCount" @click="readAll">{{ t('notices.markAllRead') }}</el-button>
        <el-button v-if="notices.length" type="danger" plain @click="clearAll">{{ t('common.clear') }}</el-button>
      </div>
    </div>

    <div v-if="inboxWindow.truncated" class="inbox-note">
      {{
        inboxWindow.total == null
          ? t('notices.truncatedUnknown', { n: inboxWindow.shown })
          : t('notices.truncated', { n: inboxWindow.shown, total: inboxWindow.total })
      }}
    </div>

    <div class="groups">
      <div v-for="g in grouped" :key="g.source" class="group glass">
        <div
          v-a11y-button
          class="group-head"
          :class="{ 'group-head--unread': g.unreadCount > 0 }"
          @click="toggleGroup(g.source)"
        >
          <span class="g-ic" :style="{ color: meta(g.source).color, background: meta(g.source).color + '1f' }">
            <SptIcon :name="meta(g.source).icon" :size="18" />
          </span>
          <div class="g-info">
            <div class="g-title">
              {{ meta(g.source).label }}
              <span class="g-count">{{ t('notices.count', { n: g.items.length }) }}</span>
              <span v-if="g.unreadCount" class="g-unread">{{ t('notices.unreadCount', { n: g.unreadCount }) }}</span>
            </div>
            <div class="g-sub">{{ t('notices.latest') }}{{ g.latest.title }} · {{ relTime(g.latest.created_at) }}</div>
          </div>
          <span class="g-chevron" :class="{ 'g-chevron--open': isGroupExpanded(g.source) }">
            <SptIcon name="chevron-down" :size="18" />
          </span>
        </div>
        <div v-if="isGroupExpanded(g.source)" class="group-items">
          <div v-for="n in g.items" :key="n.id" class="item" :class="{ 'item--unread': !n.read }">
            <span
              class="g-ic g-ic--sub"
              :style="{ color: meta(n.source).color, background: meta(n.source).color + '1f' }"
            >
              <SptIcon :name="meta(n.source).icon" :size="15" />
            </span>
            <div v-a11y-button class="item-body" @click="onBodyClick(n)">
              <div class="item-row">
                <span class="item-name">{{ n.title }}</span>
                <span class="item-time">{{ relTime(n.created_at) }}</span>
              </div>
              <div v-if="n.body" class="item-text">{{ n.body }}</div>
            </div>
            <div class="item-actions">
              <button v-if="!n.read" class="item-act" :title="t('notices.markRead')" @click.stop="readOne(n)">
                <SptIcon name="check-circle" :size="15" />
              </button>
              <button class="item-act item-act--del" :title="t('common.delete')" @click.stop="removeOne(n)">
                <SptIcon name="trash" :size="15" />
              </button>
            </div>
          </div>
          <div class="group-footer">
            <button v-if="g.unreadCount" class="g-foot-btn" @click="readGroup(g)">
              {{ t('notices.markGroupRead') }}
            </button>
            <button class="g-foot-btn g-foot-btn--danger" @click="removeGroup(g)">
              {{ t('notices.deleteGroup') }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <EmptyState
      v-if="!grouped.length && !loading"
      icon="🔔"
      :title="onlyUnread ? t('notices.emptyUnreadTitle') : t('notices.emptyTitle')"
      :description="t('notices.emptyDesc')"
    />
  </div>
</template>

<style scoped src="./Notices.css"></style>
