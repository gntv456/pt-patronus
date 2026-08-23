<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { downloaderApi, type Downloader } from '@/api/downloader'
import TorrentDrawer from '@/components/downloader/TorrentDrawer.vue'
import { useAuthStore } from '@/stores/auth'
import EmptyState from '@/components/EmptyState.vue'
import { ArrowDown, Check, CircleCheck, CircleClose, Delete, Edit, Link, MoreFilled, Plus, VideoPlay } from '@element-plus/icons-vue'

const auth = useAuthStore()

const list = ref<Downloader[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const form = ref({ name: '', type: 'qb', url: '', username: '', password: '', is_default: false })
const editingId = ref<number | null>(null)
const isEdit = computed(() => editingId.value !== null)
const dialogTitle = computed(() => (isEdit.value ? '编辑下载器' : '添加下载器'))
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
  DL_META[type] ?? { glyph: 'downloaders', from: '#536976', to: '#2c3e50', label: type || '下载器' }
const typeOptions = Object.entries(DL_META).map(([value, meta]) => ({ value, ...meta }))

const drawerDl = ref<Downloader | null>(null)
const drawerVisible = ref(false)
const testing = ref<number | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = await downloaderApi.list()
    connectionState.value = Object.fromEntries(
      list.value.map((dl) => [dl.id, connectionState.value[dl.id] ?? 'idle']),
    )
  } catch (e: unknown) {
    error.value = (e as Error).message || '加载失败'
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

async function submit() {
  form.value.name = form.value.name.trim()
  form.value.url = form.value.url.trim()
  if (!form.value.name || !form.value.url) {
    ElMessage.warning('名称与地址必填')
    return
  }
  if (!/^https?:\/\/.+/i.test(form.value.url)) {
    ElMessage.warning('地址需以 http:// 或 https:// 开头')
    return
  }
  try {
    if (isEdit.value) {
      await downloaderApi.update(editingId.value!, form.value)
      ElMessage.success('下载器配置已更新')
    } else {
      await downloaderApi.create(form.value)
      ElMessage.success('下载器已添加')
    }
    dialogVisible.value = false
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '保存失败')
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
    ElMessage.success(`已将「${dl.name}」设为默认下载器`)
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '设置失败')
  }
}

async function removeDownloader(dl: Downloader) {
  try {
    await ElMessageBox.confirm(`确认删除下载器「${dl.name}」？`, '删除下载器', {
      type: 'warning',
      confirmButtonText: '删除',
      cancelButtonText: '取消',
    })
  } catch {
    return
  }
  try {
    await downloaderApi.remove(dl.id)
    ElMessage.success('下载器已删除')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}

async function test(dl: Downloader) {
  testing.value = dl.id
  try {
    const result = await downloaderApi.test(dl.id)
    connectionState.value = { ...connectionState.value, [dl.id]: result.ok ? 'connected' : 'failed' }
    result.ok ? ElMessage.success(`${dl.name} 连接正常`) : ElMessage.warning(result.error || '连接失败')
  } catch (e: unknown) {
    connectionState.value = { ...connectionState.value, [dl.id]: 'failed' }
    ElMessage.error((e as Error).message || '请求失败')
  } finally {
    testing.value = null
  }
}

function connectionLabel(dl: Downloader): string {
  const state = connectionState.value[dl.id] ?? 'idle'
  return state === 'connected' ? '连接正常' : state === 'failed' ? '连接异常' : '尚未测试'
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
        <h1 class="title">下载器</h1>
        <p class="subtitle">统一管理连接、任务和实时传输状态</p>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">添加下载器</el-button>
    </header>

    <section class="overview-strip" aria-label="下载器概览">
      <div class="overview-item">
        <span class="overview-label">已配置</span>
        <strong>{{ list.length }}</strong>
        <span class="overview-caption">个下载器</span>
      </div>
      <div class="overview-item status-overview">
        <span class="overview-label">已测试连接</span>
        <strong>{{ connectedCount }}</strong>
        <span class="overview-caption">个正常</span>
      </div>
      <div class="overview-item">
        <span class="overview-label">类型覆盖</span>
        <strong>{{ typeCount }}</strong>
        <span class="overview-caption">种客户端</span>
      </div>
      <div class="overview-note">
        <span class="overview-label">默认下载器</span>
        <el-dropdown trigger="click" :disabled="!list.length" class="default-dropdown" @command="setDefault">
          <el-button size="small" :title="list.length ? '点击选择默认下载器' : '请先添加下载器'">
            <span class="default-name">{{ defaultDownloader?.name || '未设置' }}</span>
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
        <span class="overview-caption">搜索、订阅和工作流会优先使用</span>
      </div>
    </section>

    <div v-loading="loading" class="grid">
      <div v-if="error" class="empty error-state">
        <CircleClose class="state-icon" />
        <div class="err-text">{{ error }}</div>
        <el-button size="small" @click="load">重试</el-button>
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
                <span v-if="dl.is_default" class="default-chip">默认</span>
              </div>
              <span class="dtype-tag text-muted">{{ dlMeta(dl.type).label }}</span>
            </div>
            <el-dropdown trigger="click" @command="(command: string) => onCardCommand(command, dl)">
              <el-button text circle :icon="MoreFilled" title="更多设置" aria-label="更多设置" />
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit"><el-icon><Edit /></el-icon>编辑配置</el-dropdown-item>
                  <el-dropdown-item v-if="auth.isAdmin" command="delete" divided>
                    <el-icon><Delete /></el-icon>删除下载器
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
            <el-button type="primary" :icon="VideoPlay" @click="openTorrents(dl)">打开任务</el-button>
            <el-button :loading="testing === dl.id" :icon="Link" @click="test(dl)">测试连接</el-button>
          </div>
        </article>

        <div v-if="!list.length && !loading" class="empty">
          <EmptyState icon="downloaders" title="还没有下载器" description="添加下载器后即可统一查看实时任务和传输状态。">
            <el-button type="primary" :icon="Plus" @click="openCreate">添加下载器</el-button>
          </EmptyState>
        </div>
      </template>
    </div>

    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="680px" class="downloader-dialog">
      <div class="dialog-intro">
        <span class="dialog-kicker">连接配置</span>
        <p>保存后可立即测试连接；编辑时密码留空会沿用现有凭据。</p>
      </div>
      <el-form :model="form" label-position="top" class="config-form">
        <div class="form-grid">
          <el-form-item label="显示名称" required>
            <el-input v-model="form.name" placeholder="例如：家庭 qB" />
          </el-form-item>
          <el-form-item label="设为默认">
            <el-switch v-model="form.is_default" inline-prompt active-text="是" inactive-text="否" />
          </el-form-item>
        </div>
        <el-form-item label="客户端类型" required>
          <el-radio-group v-model="form.type" class="type-grid">
            <el-radio v-for="option in typeOptions" :key="option.value" :value="option.value" class="type-option">
              <GlassIcon :name="option.glyph" :from="option.from" :to="option.to" :size="28" />
              <span>{{ option.label }}</span>
            </el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="服务地址" required>
          <el-input v-model="form.url" placeholder="http://127.0.0.1:8080" autocomplete="url" />
          <span class="field-hint">填写下载器 Web API 地址，不要带具体接口路径。</span>
        </el-form-item>
        <div class="form-grid">
          <el-form-item label="用户名">
            <el-input v-model="form.username" autocomplete="username" />
          </el-form-item>
          <el-form-item label="密码">
            <el-input
              v-model="form.password"
              type="password"
              show-password
              autocomplete="new-password"
              :placeholder="isEdit ? '留空则不修改' : '请输入密码'"
            />
          </el-form-item>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submit">{{ isEdit ? '保存配置' : '添加下载器' }}</el-button>
      </template>
    </el-dialog>

    <TorrentDrawer v-if="drawerDl" :downloader="drawerDl" v-model:visible="drawerVisible" />
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: var(--sp-4); }
.head { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--sp-4); }
.head-copy { min-width: 0; }
.title { margin: 0; font-size: var(--fs-title); font-weight: 700; }
.subtitle { margin: 5px 0 0; color: var(--text-muted); font-size: var(--fs-body); }
.overview-strip { display: grid; grid-template-columns: repeat(3, minmax(140px, .7fr)) minmax(220px, 1.4fr); gap: var(--sp-1); overflow: hidden; border: 1px solid var(--glass-border, rgba(127,127,127,.18)); border-radius: var(--radius-xs); background: var(--glass-border, rgba(127,127,127,.18)); }
.overview-item, .overview-note { min-height: 74px; padding: 14px 16px; background: var(--surface, rgba(255,255,255,.04)); }
.overview-item { display: grid; grid-template-columns: auto auto; column-gap: var(--sp-2); align-content: center; }
.overview-label { grid-column: 1 / -1; color: var(--text-muted); font-size: var(--fs-2xs); }
.overview-item strong { font-size: var(--fs-title); line-height: 1.1; }
.overview-caption { align-self: end; padding-bottom: 2px; color: var(--text-muted); font-size: var(--fs-2xs); }
.status-overview strong { color: var(--pt-success); }
.overview-note { display: flex; flex-direction: column; justify-content: center; min-width: 0; gap: var(--sp-1); }
.default-dropdown { align-self: flex-start; }
.default-name { display: inline-block; max-width: 160px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-lg); font-weight: 650; vertical-align: middle; }
.default-check { margin-right: 4px; color: var(--pt-success); }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(310px, 1fr)); gap: var(--sp-3); }
.card { padding: 18px; min-width: 0; border-radius: var(--radius-xs); }
.card-head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-3); }
.identity { display: flex; flex: 1; flex-direction: column; min-width: 0; }
.identity-title { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; }
.dname { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-lg); font-weight: 650; }
.default-chip { display: inline-block; padding: 2px 7px; border-radius: var(--radius-2xs); background: var(--pt-warn-bg); color: var(--pt-warn); font-size: var(--fs-2xs); font-weight: 700; }
.dtype-tag { font-size: var(--fs-xs); }
.connection-row { display: flex; align-items: center; gap: var(--sp-2); min-width: 0; margin: 16px 0 18px; }
.connection-status { display: inline-flex; align-items: center; gap: var(--sp-1); flex: 0 0 auto; font-size: var(--fs-xs); font-weight: 600; }
.connection-status.connected { color: var(--pt-success); }
.connection-status.failed { color: var(--pt-danger); }
.connection-status.idle { color: var(--text-muted); }
.url { overflow: hidden; min-width: 0; text-overflow: ellipsis; white-space: nowrap; font-size: var(--fs-xs); }
.card-actions { display: grid; grid-template-columns: 1.15fr 1fr; gap: var(--sp-2); }
.card-actions :deep(.el-button) { width: 100%; margin: 0; }
.empty { grid-column: 1 / -1; padding: 42px; text-align: center; }
.error-state { display: flex; flex-direction: column; align-items: center; gap: var(--sp-2); color: var(--pt-danger); }
.state-icon { width: 28px; }
.err-text { font-size: var(--fs-md); }
.dialog-intro { padding: 12px 14px; margin-bottom: 18px; border-left: 3px solid var(--pt-primary-2); background: var(--surface-muted, rgba(127,127,127,.08)); }
.dialog-kicker { font-size: var(--fs-xs); font-weight: 700; }
.dialog-intro p { margin: 4px 0 0; color: var(--text-muted); font-size: var(--fs-xs); }
.config-form :deep(.el-form-item) { margin-bottom: 16px; }
.form-grid { display: grid; grid-template-columns: 1fr 1fr; gap: var(--sp-4); }
.type-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: var(--sp-2); width: 100%; }
.type-option { display: flex; align-items: center; min-height: 58px; padding: 8px 10px; margin: 0 !important; border: 1px solid var(--glass-border, rgba(127,127,127,.2)); border-radius: var(--radius-xs); background: var(--surface-muted, rgba(127,127,127,.05)); }
.type-option.is-checked { border-color: var(--pt-primary-2); background: color-mix(in srgb, var(--pt-primary-2) 10%, transparent); }
.type-option :deep(.el-radio__input) { margin-right: 5px; }
.type-option :deep(.el-radio__label) { display: inline-flex; align-items: center; gap: var(--sp-2); overflow: hidden; padding-left: 0; font-size: var(--fs-xs); }
.field-hint { display: block; margin-top: 5px; color: var(--text-muted); font-size: var(--fs-2xs); }

@media (max-width: 720px) {
  .head { align-items: flex-start; }
  .overview-strip { grid-template-columns: repeat(2, 1fr); }
  .overview-note { grid-column: 1 / -1; }
  .type-grid { grid-template-columns: repeat(2, 1fr); }
  .form-grid { grid-template-columns: 1fr; gap: 0; }
}

/* 下载器卡 staggered 入场 + hover 反馈 */
.grid > * { animation: pt-fade-in-up var(--dur-slow) var(--ease-out-smooth) both; }
.grid > *:nth-child(1) { animation-delay: 0ms; }
.grid > *:nth-child(2) { animation-delay: 50ms; }
.grid > *:nth-child(3) { animation-delay: 100ms; }
.grid > *:nth-child(4) { animation-delay: 150ms; }
.grid > *:nth-child(5) { animation-delay: 200ms; }
.grid > *:nth-child(n+6) { animation-delay: 250ms; }
.card { transition: transform var(--dur-base) var(--ease-out-spring), box-shadow var(--dur-base) var(--ease-out-smooth); }
.card:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); }
</style>
