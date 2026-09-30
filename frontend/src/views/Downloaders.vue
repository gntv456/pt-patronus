<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed, onMounted, ref } from 'vue'
import { downloaderApi, type Downloader } from '@/api/downloader'
import TorrentDrawer from '@/components/downloader/TorrentDrawer.vue'
import { useAuthStore } from '@/stores/auth'
import { ruleRequired, ruleURL } from '@/composables/useFormRules'
import EmptyState from '@/components/EmptyState.vue'
import {
  ArrowDown,
  Check,
  CircleCheck,
  CircleClose,
  Delete,
  Edit,
  Link,
  MoreFilled,
  Plus,
  VideoPlay,
} from '@element-plus/icons-vue'

const { t } = useI18n()
const auth = useAuthStore()

const list = ref<Downloader[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const form = ref({ name: '', type: 'qb', url: '', username: '', password: '', is_default: false })
const formRef = ref()
const formRules = {
  name: [ruleRequired(t('downloaders.form.name'))],
  url: [ruleRequired(t('downloaders.form.url')), ruleURL(t('downloaders.form.url'))],
}
const editingId = ref<number | null>(null)
const isEdit = computed(() => editingId.value !== null)
const dialogTitle = computed(() => (isEdit.value ? t('downloaders.edit') : t('downloaders.add')))
const connectionState = ref<Record<number, 'idle' | 'connected' | 'failed'>>({})
const connectedCount = computed(() => Object.values(connectionState.value).filter((s) => s === 'connected').length)
const defaultDownloader = computed(() => list.value.find((dl) => dl.is_default))
const typeCount = computed(() => new Set(list.value.map((dl) => dl.type)).size)

const DL_META: Record<string, { glyph: string; from: string; to: string; label: string }> = {
  qb: { glyph: 'qbittorrent', from: '#72B4F5', to: '#356EBF', label: 'qBittorrent' },
  tr: { glyph: 'transmission', from: '#E8443C', to: '#A82424', label: 'Transmission' },
  ut: { glyph: 'utorrent', from: '#5BB04C', to: '#3A7A2E', label: 'uTorrent' },
  dl: { glyph: 'deluge', from: '#4FB6E8', to: '#2A7AB3', label: 'Deluge' },
  ar: { glyph: 'aria2', from: '#F0A030', to: '#C07010', label: 'Aria2' },
  rt: { glyph: 'rutorrent', from: '#2E8B57', to: '#1A5C38', label: 'ruTorrent' },
  fl: { glyph: 'flood', from: '#5B8DEF', to: '#2F5FCC', label: 'Flood' },
}
const dlMeta = (type: string) =>
  DL_META[type] ?? {
    glyph: 'downloaders',
    from: '#536976',
    to: '#2c3e50',
    label: type || t('downloaders.genericLabel'),
  }
const typeOptions = Object.entries(DL_META).map(([value, meta]) => ({ value, ...meta }))

const drawerDl = ref<Downloader | null>(null)
const drawerVisible = ref(false)
const testing = ref<number | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = await downloaderApi.list()
    connectionState.value = Object.fromEntries(list.value.map((dl) => [dl.id, connectionState.value[dl.id] ?? 'idle']))
  } catch (e: unknown) {
    error.value = (e as Error).message || t('downloaders.loadFailed')
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  form.value = { name: '', type: 'qb', url: '', username: '', password: '', is_default: false }
  dialogVisible.value = true
}

function openEdit(dl: Downloader) {
  editingId.value = dl.id
  form.value = {
    name: dl.name,
    type: dl.type,
    url: dl.url,
    username: dl.username,
    password: '',
    is_default: dl.is_default,
  }
  dialogVisible.value = true
}

// 提交防双击：请求在途时忽略重复提交。
const submitting = ref(false)
async function submit() {
  if (submitting.value) return
  form.value.name = form.value.name.trim()
  form.value.url = form.value.url.trim()
  // 字段级校验：错误显示在对应输入框下方（替代此前的提交后一条 toast）
  if (formRef.value) {
    try {
      await formRef.value.validate()
    } catch {
      return
    }
  }
  submitting.value = true
  try {
    if (isEdit.value) {
      await downloaderApi.update(editingId.value!, form.value)
      ElMessage.success(t('downloaders.updated'))
    } else {
      await downloaderApi.create(form.value)
      ElMessage.success(t('downloaders.added'))
    }
    dialogVisible.value = false
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('downloaders.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// 概览条上点击选择默认下载器：复用 update（is_default=true），后端事务内自动清掉其他默认。
async function setDefault(id: number) {
  const dl = list.value.find((d) => d.id === id)
  if (!dl || dl.is_default) return
  try {
    await downloaderApi.update(id, {
      name: dl.name,
      type: dl.type,
      url: dl.url,
      username: dl.username,
      is_default: true,
    })
    ElMessage.success(t('downloaders.setDefaultOk', { name: dl.name }))
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('downloaders.setDefaultFailed'))
  }
}

async function removeDownloader(dl: Downloader) {
  try {
    await ElMessageBox.confirm(t('downloaders.confirmDelete', { name: dl.name }), t('downloaders.deleteTitle'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
  try {
    await downloaderApi.remove(dl.id)
    ElMessage.success(t('downloaders.deleteOk'))
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('downloaders.deleteFailed'))
  }
}

async function test(dl: Downloader) {
  testing.value = dl.id
  try {
    const result = await downloaderApi.test(dl.id)
    connectionState.value = { ...connectionState.value, [dl.id]: result.ok ? 'connected' : 'failed' }
    result.ok
      ? ElMessage.success(t('downloaders.connOk', { name: dl.name }))
      : ElMessage.warning(result.error || t('downloaders.connFailed'))
  } catch (e: unknown) {
    connectionState.value = { ...connectionState.value, [dl.id]: 'failed' }
    ElMessage.error((e as Error).message || t('downloaders.requestFailed'))
  } finally {
    testing.value = null
  }
}

function connectionLabel(dl: Downloader): string {
  const state = connectionState.value[dl.id] ?? 'idle'
  const map: Record<string, string> = {
    connected: t('downloaders.connState.connected'),
    failed: t('downloaders.connState.failed'),
    idle: t('downloaders.connState.idle'),
  }
  return map[state] ?? map.idle
}

function onCardCommand(command: string, dl: Downloader) {
  if (command === 'edit') openEdit(dl)
  else if (command === 'delete') removeDownloader(dl)
}

function openTorrents(dl: Downloader) {
  drawerDl.value = dl
  drawerVisible.value = true
}

onMounted(load)
</script>

<template>
  <div class="page">
    <header class="head">
      <div class="head-copy">
        <h1 class="title">{{ t('downloaders.title') }}</h1>
        <p class="subtitle">{{ t('downloaders.subtitle') }}</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('downloaders.add') }}</el-button>
    </header>

    <section class="overview-strip" :aria-label="t('downloaders.overview.label')">
      <div class="overview-item">
        <span class="overview-label">{{ t('downloaders.overview.configured') }}</span>
        <strong>{{ list.length }}</strong>
        <span class="overview-caption">{{ t('downloaders.overview.configuredUnit') }}</span>
      </div>
      <div class="overview-item status-overview">
        <span class="overview-label">{{ t('downloaders.overview.tested') }}</span>
        <strong>{{ connectedCount }}</strong>
        <span class="overview-caption">{{ t('downloaders.overview.testedUnit') }}</span>
      </div>
      <div class="overview-item">
        <span class="overview-label">{{ t('downloaders.overview.types') }}</span>
        <strong>{{ typeCount }}</strong>
        <span class="overview-caption">{{ t('downloaders.overview.typesUnit') }}</span>
      </div>
      <div class="overview-note">
        <span class="overview-label">{{ t('downloaders.overview.defaultDl') }}</span>
        <el-dropdown trigger="click" :disabled="!list.length" class="default-dropdown" @command="setDefault">
          <el-button
            size="small"
            :title="list.length ? t('downloaders.overview.pickDefault') : t('downloaders.overview.addFirst')"
          >
            <span class="default-name">{{ defaultDownloader?.name || t('downloaders.overview.unset') }}</span>
            <el-icon class="el-icon--right"><ArrowDown /></el-icon>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-for="dl in list" :key="dl.id" :command="dl.id">
                <el-icon v-if="dl.is_default" class="default-check"><Check /></el-icon>
                {{ dl.name }}
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <span class="overview-caption">{{ t('downloaders.overview.defaultHint') }}</span>
      </div>
    </section>

    <div v-loading="loading" class="grid">
      <div v-if="error" class="empty error-state">
        <CircleClose class="state-icon" />
        <div class="err-text">{{ error }}</div>
        <el-button size="small" @click="load">{{ t('downloaders.retry') }}</el-button>
      </div>
      <template v-else>
        <article v-for="dl in list" :key="dl.id" class="glass glass-hover card">
          <div class="card-head">
            <GlassIcon
              :name="dlMeta(dl.type).glyph"
              :from="dlMeta(dl.type).from"
              :to="dlMeta(dl.type).to"
              :size="42"
              class="dl-badge"
            />
            <div class="identity">
              <div class="identity-title">
                <span class="dname">{{ dl.name }}</span>
                <span v-if="dl.is_default" class="default-chip">{{ t('downloaders.card.defaultChip') }}</span>
              </div>
              <span class="dtype-tag text-muted">{{ dlMeta(dl.type).label }}</span>
            </div>
            <el-dropdown trigger="click" @command="(command: string) => onCardCommand(command, dl)">
              <el-button
                text
                circle
                :icon="MoreFilled"
                :title="t('downloaders.card.more')"
                :aria-label="t('downloaders.card.more')"
              />
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit"
                    ><el-icon><Edit /></el-icon>{{ t('downloaders.card.editConfig') }}</el-dropdown-item
                  >
                  <el-dropdown-item v-if="auth.isAdmin" command="delete" divided>
                    <el-icon><Delete /></el-icon>{{ t('downloaders.card.delete') }}
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>

          <div class="connection-row">
            <span class="connection-status" :class="connectionState[dl.id] || 'idle'">
              <el-icon>
                <CircleCheck v-if="connectionState[dl.id] === 'connected'" />
                <CircleClose v-else-if="connectionState[dl.id] === 'failed'" />
                <Link v-else />
              </el-icon>
              {{ connectionLabel(dl) }}
            </span>
            <span class="url text-muted" :title="dl.url">{{ dl.url }}</span>
          </div>

          <div class="card-actions">
            <el-button type="primary" :icon="VideoPlay" @click="openTorrents(dl)">{{
              t('downloaders.card.openTasks')
            }}</el-button>
            <el-button :loading="testing === dl.id" :icon="Link" @click="test(dl)">{{
              t('downloaders.card.testConn')
            }}</el-button>
          </div>
        </article>

        <div v-if="!list.length && !loading" class="empty">
          <EmptyState
            icon="downloaders"
            :title="t('downloaders.card.emptyTitle')"
            :description="t('downloaders.card.emptyDesc')"
          >
            <el-button type="primary" :icon="Plus" @click="openCreate">{{ t('downloaders.add') }}</el-button>
          </EmptyState>
        </div>
      </template>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="680px" class="downloader-dialog">
      <div class="dialog-intro">
        <span class="dialog-kicker">{{ t('downloaders.form.kicker') }}</span>
        <p>{{ t('downloaders.form.hint') }}</p>
      </div>
      <el-form ref="formRef" :model="form" :rules="formRules" label-position="top" class="config-form">
        <div class="form-grid">
          <el-form-item :label="t('downloaders.form.name')" prop="name" required>
            <el-input v-model="form.name" :placeholder="t('downloaders.form.namePlaceholder')" />
          </el-form-item>
          <el-form-item :label="t('downloaders.form.isDefault')">
            <el-switch
              v-model="form.is_default"
              inline-prompt
              :active-text="t('downloaders.form.on')"
              :inactive-text="t('downloaders.form.off')"
            />
          </el-form-item>
        </div>
        <el-form-item :label="t('downloaders.form.type')" required>
          <el-radio-group v-model="form.type" class="type-grid">
            <el-radio v-for="option in typeOptions" :key="option.value" :value="option.value" class="type-option">
              <GlassIcon :name="option.glyph" :from="option.from" :to="option.to" :size="28" />
              <span>{{ option.label }}</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item :label="t('downloaders.form.url')" prop="url" required>
          <el-input v-model="form.url" placeholder="http://127.0.0.1:8080" autocomplete="url" />
          <span class="field-hint">{{ t('downloaders.form.urlHint') }}</span>
        </el-form-item>
        <div class="form-grid">
          <el-form-item :label="t('downloaders.form.username')">
            <el-input v-model="form.username" autocomplete="username" />
          </el-form-item>
          <el-form-item :label="t('downloaders.form.password')">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              autocomplete="new-password"
              :placeholder="isEdit ? t('downloaders.form.passwordEdit') : t('downloaders.form.passwordNew')"
            />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{
          isEdit ? t('downloaders.form.save') : t('downloaders.add')
        }}</el-button>
      </template>
    </el-dialog>

    <TorrentDrawer v-if="drawerDl" :downloader="drawerDl" v-model:visible="drawerVisible" />
  </div>
</template>

<style scoped src="./Downloaders.css"></style>
