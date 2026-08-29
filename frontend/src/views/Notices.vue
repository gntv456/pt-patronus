<script setup lang="ts">
// 通知中心（全页面）：按来源类型分组并折叠，点击展开查看每条详情。
// 与 topbar 铃铛下拉（NotificationCenter.vue）共用 sourceMeta / 路由映射，体验一致。
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useWs } from '@/composables/useWs'
import { noticeApi, type Notice, type NoticeSource } from '@/api/notice'
import SptIcon from '@/components/SptIcon.vue'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const router = useRouter()
const { notices: wsNotices } = useWs()
const notices = ref<Notice[]>([])
const loading = ref(false)

// 来源类型元信息（图标 / 配色 / 中文名）
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

// 点击通知体后跳转的目标路由（按来源分类）
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

// 筛选：全部 / 仅未读
const filter = ref<'all' | 'unread'>('all')
const onlyUnread = computed(() => filter.value === 'unread')

const unreadCount = computed(() => notices.value.filter((n) => !n.read).length)
const urgentSources = new Set<NoticeSource>(['assess_fail', 'host_offline', 'nas_offline', 'site_offline'])
const urgentUnread = computed(() =>
  notices.value.filter((n) => !n.read && urgentSources.has(n.source)).length,
)

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
    notices.value = await noticeApi.list()
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

async function readOne(n: Notice) {
  try {
    await noticeApi.read(n.id)
    n.read = true
  } catch {
    /* 静默 */
  }
}
async function readGroup(g: Group) {
  try {
    await Promise.all(g.items.filter((n) => !n.read).map((n) => noticeApi.read(n.id)))
    g.items.forEach((n) => (n.read = true))
    g.unreadCount = 0
  } catch {
    /* 静默 */
  }
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
  } catch {
    /* 静默 */
  }
}
async function removeGroup(g: Group) {
  try {
    await Promise.all(g.items.map((n) => noticeApi.delete(n.id)))
    const ids = new Set(g.items.map((n) => n.id))
    notices.value = notices.value.filter((n) => !ids.has(n.id))
  } catch {
    /* 静默 */
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
        <small v-if="unreadCount">{{ t('notices.unread', { n: unreadCount }) }}<template v-if="urgentUnread"> · {{ t('notices.urgent', { n: urgentUnread }) }}</template></small>
      </h2>
      <div class="head-tools">
        <div class="filter glass">
          <button class="filter-btn" :class="{ on: filter === 'all' }" @click="filter = 'all'">{{ t('notices.filter.all') }}</button>
          <button class="filter-btn" :class="{ on: filter === 'unread' }" @click="filter = 'unread'">
            {{ t('notices.filter.unread') }}<span v-if="unreadCount" class="filter-badge">{{ unreadCount }}</span>
          </button>
        </div>
        <el-button :loading="loading" @click="load">{{ t('common.refresh') }}</el-button>
        <el-button v-if="unreadCount" @click="readAll">{{ t('notices.markAllRead') }}</el-button>
        <el-button v-if="notices.length" type="danger" plain @click="clearAll">{{ t('common.clear') }}</el-button>
      </div>
    </div>

    <div class="groups">
      <div v-for="g in grouped" :key="g.source" class="group glass">
        <div
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
          <div
            v-for="n in g.items"
            :key="n.id"
            class="item"
            :class="{ 'item--unread': !n.read }"
          >
            <span class="g-ic g-ic--sub" :style="{ color: meta(n.source).color, background: meta(n.source).color + '1f' }">
              <SptIcon :name="meta(n.source).icon" :size="15" />
            </span>
            <div class="item-body" @click="onBodyClick(n)">
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
            <button v-if="g.unreadCount" class="g-foot-btn" @click="readGroup(g)">{{ t('notices.markGroupRead') }}</button>
            <button class="g-foot-btn g-foot-btn--danger" @click="removeGroup(g)">{{ t('notices.deleteGroup') }}</button>
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

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--sp-3); }
.title { margin: 4px 0; font-size: var(--fs-title); font-weight: 700; }
.title small { margin-left: 8px; color: var(--text-muted); font-weight: 500; font-size: var(--fs-md); }
.head-tools { display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.filter { display: inline-flex; padding: 4px; gap: var(--sp-1); border-radius: var(--radius-sm); }
.filter-btn { background: transparent; border: none; cursor: pointer; padding: 6px 14px; border-radius: var(--radius-xs); font-size: var(--fs-body); color: var(--text-strong); display: inline-flex; align-items: center; gap: var(--sp-1); transition: all var(--dur-base) var(--ease-out-smooth); }
.filter-btn:hover:not(.on) { background: var(--surface-muted); }
.filter-btn.on { background: var(--pt-primary-bg); color: var(--pt-primary,#6c5ce7); font-weight: 600; }
.filter-badge { min-width: 16px; height: 16px; padding: 0 4px; border-radius: var(--radius-pill); background: var(--pt-primary,#6c5ce7); color: #fff; font-size: var(--fs-2xs); font-weight: 700; display: inline-flex; align-items: center; justify-content: center; line-height: 1; }
.groups { display: flex; flex-direction: column; gap: var(--sp-3); }
.group { overflow: hidden; }
.group-head { display: flex; align-items: center; gap: var(--sp-3); padding: 16px 18px; cursor: pointer; transition: background var(--dur-fast) var(--ease-out-smooth); }
.group-head:hover { background: var(--surface-hover); }
.group-head--unread { background: var(--pt-primary-bg); }
.g-ic { flex: none; width: 38px; height: 38px; border-radius: var(--radius-sm); display: inline-flex; align-items: center; justify-content: center; }
.g-info { flex: 1; min-width: 0; }
.g-title { font-size: var(--fs-lg); font-weight: 700; color: var(--text-strong); display: flex; align-items: center; gap: var(--sp-2); flex-wrap: wrap; }
.g-count { color: var(--text-muted); font-weight: 500; font-size: var(--fs-xs); }
.g-unread { color: var(--pt-primary,#6c5ce7); font-weight: 600; font-size: var(--fs-xs); }
.g-sub { font-size: var(--fs-xs); color: var(--text-muted); margin-top: 3px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.g-chevron { flex: none; color: var(--text-muted); transition: transform var(--dur-base) var(--ease-out-smooth); }
.g-chevron--open { transform: rotate(180deg); }
.group-items { border-top: 1px solid var(--divider); }
.item { display: flex; align-items: flex-start; gap: var(--sp-3); padding: 12px 18px 12px 50px; border-bottom: 1px solid var(--divider); transition: background var(--dur-fast) var(--ease-out-smooth); }
.item:last-of-type { border-bottom: none; }
.item:hover { background: var(--surface-hover); }
.item--unread { background: var(--pt-primary-bg); }
.g-ic--sub { width: 28px; height: 28px; opacity: 0.85; }
.item-body { flex: 1; min-width: 0; cursor: pointer; }
.item-row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-2); }
.item-name { font-size: var(--fs-md); font-weight: 600; color: var(--text-strong); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.item-time { font-size: var(--fs-xs); color: var(--text-muted); flex: none; }
.item-text { font-size: var(--fs-body); color: var(--text-strong); opacity: 0.75; margin-top: 3px; line-height: 1.5; word-break: break-word; }
.item-actions { flex: none; display: flex; gap: var(--sp-1); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out-smooth); }
.item:hover .item-actions { opacity: 1; }
.item-act { background: none; border: none; cursor: pointer; color: var(--text-muted); padding: 4px; border-radius: var(--radius-xs); display: inline-flex; transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth) }
.item-act:hover { color: var(--pt-primary,#6c5ce7); background: var(--pt-primary-bg); }
.item-act--del:hover { color: var(--pt-danger); background: var(--pt-danger-bg); }
.group-footer { display: flex; gap: var(--sp-2); padding: 8px 18px 12px 50px; }
.g-foot-btn { background: none; border: none; cursor: pointer; color: var(--pt-primary,#6c5ce7); font-size: var(--fs-xs); padding: 4px 8px; border-radius: var(--radius-xs); transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth) }
.g-foot-btn:hover { background: var(--pt-primary-bg); }
.g-foot-btn--danger { color: var(--pt-danger); }
.g-foot-btn--danger:hover { background: var(--pt-danger-bg); }
</style>