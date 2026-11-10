<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { messageApi, type MessageResult, type Message, type Mailbox } from '@/api/message'
import { friendlySiteError } from '@/utils/siteError'
import EmptyState from '@/components/EmptyState.vue'
import SiteFilterDialog from '@/components/SiteFilterDialog.vue'

const { t } = useI18n()
const mailbox = ref<Mailbox>('inbox')
const results = ref<MessageResult[]>([])
const loading = ref(false)
const detail = ref<Message | null>(null)
const detailContent = ref('')
const detailLoading = ref(false)
const replyText = ref('')
const replyVisible = ref(false)

// 站点筛选：空=全部，非空=仅选中站点（多选，纯前端筛选）。UI 用 SiteFilterDialog 组件。
const selectedSiteIds = ref<number[]>([])
const sites = computed(() => {
  const m = new Map<number, string>()
  for (const r of results.value) if (!m.has(r.site_id)) m.set(r.site_id, r.site_name)
  return [...m].map(([id, name]) => ({ id, name }))
})
const filtered = computed(() =>
  selectedSiteIds.value.length ? results.value.filter((r) => selectedSiteIds.value.includes(r.site_id)) : results.value,
)

const mailboxes: { key: Mailbox; label: string }[] = [
  { key: 'inbox', label: t('messages.box.inbox') },
  { key: 'system', label: t('messages.box.system') },
  { key: 'sent', label: t('messages.box.sent') },
  { key: 'cheater', label: t('messages.box.cheater') },
  { key: 'reports', label: t('messages.box.reports') },
  { key: 'staff', label: t('messages.box.staff') },
]

// 站点错误友好化：统一用 @/utils/siteError（Messages / Announcements 共用）。
// 原始文本仍保留在 title 属性供高级用户悬停查看。
function friendlyError(err?: string): string {
  return friendlySiteError(err)
}

let abortCtrl: AbortController | null = null
async function load(refresh = false) {
  abortCtrl?.abort()
  const ctrl = new AbortController()
  abortCtrl = ctrl
  loading.value = true
  results.value = []
  try {
    await messageApi.streamAll(
      mailbox.value,
      refresh,
      (r) => {
        // 流式逐站追加：同 site_id 覆盖（刷新），否则追加。Vue 响应式即时渲染"加载一个显示一个"。
        const idx = results.value.findIndex((x) => x.site_id === r.site_id)
        if (idx >= 0) results.value[idx] = r
        else results.value.push(r)
      },
      ctrl.signal,
    )
  } catch (e: unknown) {
    if ((e as Error).name !== 'AbortError') ElMessage.error((e as Error).message || t('messages.loadFailed'))
  } finally {
    if (abortCtrl === ctrl) {
      loading.value = false
      abortCtrl = null
    }
  }
}

function switchMailbox(mb: Mailbox) {
  mailbox.value = mb
  load()
}

async function openDetail(msg: Message) {
  detail.value = msg
  detailContent.value = msg.content || ''
  detailLoading.value = true
  try {
    if (msg.url) detailContent.value = await messageApi.content(msg.site_id, msg.url)
    // 打开即标记已读
    void markRead(msg)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('messages.loadContentFailed'))
  } finally {
    detailLoading.value = false
  }
}

// markRead 打开即标已读，但成败要回到界面：站点常常回 HTTP 200 却 failed:1（或明确不支持远程
// 标记，如馒头/FluxTorrent 走 5xx），旧写法 `.then(() => load())` 把这两种都吞掉——红点原地不
// 动、不报错，用户只能反复点开同一条；而它挂在 try 里也拦不住自己的 rejection（游离 Promise）。
async function markRead(msg: Message) {
  const id = msg.admin_actions?.item_id
  if (msg.is_read || !id) return
  try {
    const r = await messageApi.markRead(msg.site_id, [id])
    if (r.failed > 0 || r.success < 1) {
      ElMessage.warning(r.errors?.[0] || t('messages.markReadFailed'))
      return
    }
    // 就地清红点：为一颗点重扫全站会把正在读的这条所在的流 abort 掉
    msg.is_read = true
  } catch (e: unknown) {
    ElMessage.warning((e as Error).message || t('messages.markReadFailed'))
  }
}

function startReply() {
  replyText.value = ''
  replyVisible.value = true
}

async function sendReply() {
  if (!detail.value) return
  try {
    const r = await messageApi.reply(detail.value.site_id, detail.value.url, replyText.value)
    if (r.success) {
      ElMessage.success(r.message)
      replyVisible.value = false
    } else {
      ElMessage.warning(r.message)
    }
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('messages.replyFailed'))
  }
}

async function removeMsg() {
  if (!detail.value) return
  try {
    await ElMessageBox.confirm(t('messages.confirmDelete'), t('messages.tip'), { type: 'warning' })
  } catch {
    return
  }
  const msg = detail.value
  try {
    await messageApi.remove(msg.site_id, msg.admin_actions?.delete_url || msg.url)
    ElMessage.success(t('common.deleted'))
    detail.value = null
    load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('common.deleteFailed'))
  }
}

onMounted(() => load())
onUnmounted(() => abortCtrl?.abort())
</script>

<template>
  <div class="page" v-loading="loading && !filtered.length">
    <div class="head">
      <h2 class="title">{{ t('messages.title') }}</h2>
      <div class="head-tools">
        <SiteFilterDialog v-model="selectedSiteIds" :sites="sites" :title="t('messages.siteFilterTitle')" />
        <el-button :loading="loading" @click="load(true)">{{ t('common.refresh') }}</el-button>
      </div>
    </div>

    <div class="tabs glass">
      <button
        v-for="mb in mailboxes"
        :key="mb.key"
        class="tab"
        :class="{ on: mailbox === mb.key }"
        @click="switchMailbox(mb.key)"
      >
        {{ mb.label }}
      </button>
    </div>

    <div class="groups">
      <div v-for="r in filtered" :key="r.site_id" class="group glass">
        <div class="group-head">
          <span class="site-name">{{ r.site_name }}</span>
          <span v-if="r.error" class="err-tag" :title="r.error">{{ friendlyError(r.error) }}</span>
          <span v-else-if="r.unsupported" class="text-muted" :title="t('messages.notSupportedTip')">{{
            t('messages.notSupported')
          }}</span>
          <span v-else-if="!r.messages.length" class="text-muted">{{ t('messages.noMessages') }}</span>
        </div>
        <div class="msg-list">
          <div
            v-a11y-button
            v-for="m in r.messages"
            :key="m.id"
            class="msg"
            :class="{ unread: !m.is_read }"
            @click="openDetail(m)"
          >
            <span class="dot" v-if="!m.is_read"></span>
            <div class="msg-main">
              <div class="msg-title">
                {{ m.title }}<span v-if="m.subtitle" class="sub"> - {{ m.subtitle }}</span>
              </div>
              <div class="msg-meta">{{ m.sender }} · {{ m.date }}</div>
            </div>
          </div>
        </div>
      </div>
    </div>
    <EmptyState
      v-if="!filtered.length && !loading"
      icon="✉️"
      :title="selectedSiteIds.length === 0 ? t('messages.emptyTitle') : t('messages.emptySiteTitle')"
      :description="selectedSiteIds.length === 0 ? t('messages.emptyDesc') : t('messages.emptySiteDesc')"
    />

    <el-drawer
      :model-value="!!detail"
      @update:model-value="
        (v: boolean) => {
          if (!v) detail = null
        }
      "
      :title="detail?.title || ''"
      size="45%"
    >
      <div v-if="detail" v-loading="detailLoading">
        <div class="detail-meta">
          <span>{{ detail.sender }}</span> · <span>{{ detail.date }}</span> ·
          <span class="text-muted">{{ detail.site_name }}</span>
        </div>
        <div class="detail-body">{{ detailContent || t('messages.noContent') }}</div>
        <div class="detail-actions">
          <el-button @click="startReply">{{ t('messages.reply') }}</el-button>
          <el-button type="danger" @click="removeMsg">{{ t('common.delete') }}</el-button>
        </div>
      </div>
    </el-drawer>

    <el-dialog v-model="replyVisible" :title="t('messages.replyTitle')" width="500">
      <el-input v-model="replyText" type="textarea" :rows="6" :placeholder="t('messages.replyPlaceholder')" />
      <template #footer>
        <el-button @click="replyVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="sendReply">{{ t('messages.send') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.head-tools {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.tabs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
  padding: 8px;
}
.tab {
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-xs);
  padding: 6px 16px;
  cursor: pointer;
  font-size: var(--fs-md);
  color: var(--text-strong);
  transition: all var(--dur-base) var(--ease-out-smooth);
}
.tab:hover {
  background: var(--surface-muted);
}
.tab.on {
  background: var(--pt-primary-bg);
  color: var(--pt-primary);
  font-weight: 600;
}
.groups {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.group {
  padding: 14px;
}
.group-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 8px;
}
.site-name {
  font-weight: 600;
}
.err-tag {
  font-size: var(--fs-xs);
  color: var(--pt-danger);
}
.msg-list {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.msg {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 8px 10px;
  border-radius: var(--radius-xs);
  cursor: pointer;
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.msg:hover {
  background: var(--pt-primary-bg);
}
.msg.unread {
  background: var(--pt-primary-bg);
  font-weight: 600;
}
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--pt-primary);
  flex: none;
}
.msg-main {
  flex: 1;
  min-width: 0;
}
.msg-title {
  font-size: var(--fs-md);
}
.msg-title .sub {
  font-weight: 400;
  color: var(--text-muted);
  font-size: var(--fs-xs);
}
.msg-meta {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  margin-top: 2px;
}
.detail-meta {
  font-size: var(--fs-body);
  color: var(--text-muted);
  margin-bottom: 12px;
}
.detail-body {
  font-size: var(--fs-md);
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
  min-height: 120px;
}
.detail-actions {
  margin-top: 20px;
  display: flex;
  gap: var(--sp-2);
}
.empty {
  padding: 40px;
  text-align: center;
}
</style>
