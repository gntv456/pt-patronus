<script setup lang="ts">
// NotificationCenter — 全局通知中心铃铛（topbar 内）。
// 初始拉取 noticeApi.list() + WS 实时推送 useWs.notices 合并；未读角标；下拉列表（已读/删除/全部已读）。
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useWs } from '@/composables/useWs'
import { noticeApi, type Notice, type NoticeSource } from '@/api/notice'
import SptIcon from '@/components/SptIcon.vue'

const { notices: wsNotices } = useWs()
const notices = ref<Notice[]>([])
const open = ref(false)
const boxRef = ref<HTMLElement | null>(null)

const sourceMeta: Record<NoticeSource, { icon: string; color: string }> = {
  site_anniv: { icon: 'party-popper', color: '#f59e0b' },
  new_mail: { icon: 'mail', color: '#3b82f6' },
  assess_pass: { icon: 'check-circle', color: '#22c55e' },
  assess_fail: { icon: 'alert-circle', color: '#ef4444' },
  announce: { icon: 'megaphone', color: '#3b82f6' },
  host_offline: { icon: 'alert-triangle', color: '#ef4444' },
  nas_offline: { icon: 'alert-triangle', color: '#ef4444' },
  reseed_done: { icon: 'check-circle', color: '#22c55e' },
  dl_done: { icon: 'download', color: '#22c55e' },
  site_offline: { icon: 'alert-triangle', color: '#ef4444' },
  backup_done: { icon: 'check-circle', color: '#22c55e' },
  backup_import: { icon: 'check-circle', color: '#22c55e' },
  freeleech: { icon: 'party-popper', color: '#f59e0b' },
}
function meta(s: NoticeSource) {
  return sourceMeta[s] ?? sourceMeta.announce
}

const unread = computed(() => notices.value.filter((n) => !n.read).length)

async function load() {
  try {
    notices.value = await noticeApi.list()
  } catch {
    /* 静默 */
  }
}
onMounted(() => {
  load()
  document.addEventListener('click', onDocClick)
})
onBeforeUnmount(() => document.removeEventListener('click', onDocClick))

function onDocClick(e: MouseEvent) {
  if (open.value && boxRef.value && !boxRef.value.contains(e.target as Node)) open.value = false
}

// WS 实时推送：新通知 prepend 到本地列表（去重 id）
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
async function readAll() {
  try {
    await noticeApi.readAll()
    notices.value.forEach((n) => (n.read = true))
  } catch {
    /* 静默 */
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
async function clearAll() {
  if (!window.confirm('确定清空全部通知？此操作不可恢复。')) return
  try {
    await noticeApi.clear()
    notices.value = []
  } catch {
    /* 静默 */
  }
}

function relTime(iso: string): string {
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const diff = (Date.now() - d.getTime()) / 1000
  if (diff < 60) return '刚刚'
  if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`
  if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`
  return `${Math.floor(diff / 86400)} 天前`
}
</script>

<template>
  <div ref="boxRef" class="nc">
    <button
      class="nc-bell"
      :class="{ 'nc-bell--has': unread > 0 }"
      title="通知中心"
      @click.stop="open = !open"
    >
      <SptIcon name="bell" :size="20" />
      <span v-if="unread > 0" class="nc-badge">{{ unread > 99 ? '99+' : unread }}</span>
    </button>

    <Transition name="nc-slide">
      <div v-if="open" class="nc-panel glass">
        <div class="nc-head">
          <span class="nc-title">通知中心</span>
          <div class="nc-actions">
            <button v-if="unread > 0" class="nc-action" @click="readAll">全部已读</button>
            <button v-if="notices.length" class="nc-action nc-action--danger" @click="clearAll">清空</button>
          </div>
        </div>
        <div class="nc-list">
          <div v-if="!notices.length" class="nc-empty">暂无通知</div>
          <div
            v-for="n in notices"
            :key="n.id"
            class="nc-item"
            :class="{ 'nc-item--unread': !n.read }"
          >
            <span class="nc-ic" :style="{ color: meta(n.source).color, background: meta(n.source).color + '1f' }">
              <SptIcon :name="meta(n.source).icon" :size="16" />
            </span>
            <div class="nc-body" @click="!n.read && readOne(n)">
              <div class="nc-row">
                <span class="nc-name">{{ n.title }}</span>
                <span class="nc-time">{{ relTime(n.created_at) }}</span>
              </div>
              <div v-if="n.body" class="nc-text">{{ n.body }}</div>
            </div>
            <button class="nc-del" title="删除" @click.stop="removeOne(n)">
              <SptIcon name="trash" :size="14" />
            </button>
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
  transition: background 0.2s;
}
.nc-bell:hover {
  background: rgba(108, 92, 231, 0.12);
}
.nc-bell--has {
  color: var(--pt-warn, #f39c12);
}
.nc-badge {
  position: absolute;
  top: 0;
  right: 0;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 999px;
  background: #e74c3c;
  color: #fff;
  font-size: 10px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  line-height: 1;
  box-shadow: 0 0 0 2px var(--glass-bg-strong, rgba(255, 255, 255, 0.72));
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
}
.nc-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 14px;
  border-bottom: 1px solid var(--glass-border);
}
.nc-title {
  font-weight: 700;
  font-size: 14px;
}
.nc-action {
  background: none;
  border: none;
  cursor: pointer;
  color: var(--pt-primary);
  font-size: 12px;
}
.nc-action:hover {
  text-decoration: underline;
}
.nc-actions {
  display: inline-flex;
  align-items: center;
  gap: 12px;
}
.nc-action--danger {
  color: var(--pt-danger, #e74c3c);
}
.nc-list {
  overflow-y: auto;
  flex: 1;
}
.nc-empty {
  padding: 36px;
  text-align: center;
  color: var(--text-muted);
  font-size: 13px;
}
.nc-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px 14px;
  border-bottom: 1px solid var(--glass-border);
  transition: background 0.15s;
}
.nc-item:hover {
  background: rgba(108, 92, 231, 0.05);
}
.nc-item--unread {
  background: rgba(108, 92, 231, 0.06);
}
.nc-ic {
  flex: none;
  width: 30px;
  height: 30px;
  border-radius: 8px;
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
  gap: 8px;
}
.nc-name {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.nc-time {
  font-size: 11px;
  color: var(--text-muted);
  flex: none;
}
.nc-text {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 2px;
  line-height: 1.45;
}
.nc-del {
  flex: none;
  background: none;
  border: none;
  cursor: pointer;
  color: var(--text-muted);
  padding: 2px;
  opacity: 0.5;
  transition: opacity 0.15s, color 0.15s;
}
.nc-del:hover {
  opacity: 1;
  color: var(--pt-danger);
}
.nc-slide-enter-active,
.nc-slide-leave-active {
  transition: all 0.2s ease;
}
.nc-slide-enter-from,
.nc-slide-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}
</style>
