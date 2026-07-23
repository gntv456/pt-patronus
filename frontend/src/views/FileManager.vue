<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { fileApi, type FileStorage, type FileEntry } from '@/api/filemanager'
import { cloudApi, type CloudDrive, type CloudFile } from '@/api/cloud'
import { agentApi, type Agent } from '@/api/monitor'
import {

  Folder, Document, Top, Plus, Refresh, Upload, FolderAdd,
  Download, Edit, Rank as MoveIcon, CopyDocument, Delete, Link,
} from '@element-plus/icons-vue'
import EmptyState from '@/components/EmptyState.vue'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })


// 存储项统一：本地/WebDAV/SMB 等（可读写）｜云盘（115/夸克/123，只读 + STRM）。
type StorageItem = { kind: 'fs'; fs: FileStorage } | { kind: 'cloud'; drive: CloudDrive }

const list = ref<StorageItem[]>([])
const loading = ref(false)
const agents = ref<Agent[]>([])
const current = ref<StorageItem | null>(null)

type ConnState = 'checking' | 'ok' | 'fail'
const statusMap = ref<Record<string, { state: ConnState; error?: string }>>({})
const testing = ref<string | null>(null)

// 浏览
const path = ref('/')
const entries = ref<FileEntry[]>([])
const bloading = ref(false)
const viewMode = ref<'list' | 'grid'>(localStorage.getItem('fm.view') === 'grid' ? 'grid' : 'list')
const selected = ref<FileEntry[]>([])
const uploadProgress = ref(-1)

// 存储添加/编辑
const storageDialog = ref(false)
const editingId = ref<number | null>(null)
const form = ref({ name: '', type: 'webdav', url: '', username: '', password: '', base_path: '/', agent_id: 0 })

// 新建文件夹 / 重命名
const mkdirVisible = ref(false)
const mkdirName = ref('')
const renameVisible = ref(false)
const renameTarget = ref<FileEntry | null>(null)
const renameNewName = ref('')

// 移动/复制目标选择
const moveCopyVisible = ref(false)
const moveCopyMode = ref<'move' | 'copy'>('move')
const moveCopyPath = ref('/')
const moveCopyEntries = ref<FileEntry[]>([])
const moveCopyLoading = ref(false)
const moveCopySources = ref<FileEntry[]>([])

// STRM 即时生成
const strmVisible = ref(false)
const strmDir = ref<FileEntry | null>(null)
const strmForm = ref({ output_dir: '', mount_root: '', template: '', recursive: true, keep_ext: true })
const strmLoading = ref(false)

// 写操作防重复提交锁：执行中禁用/Loading 按钮，防止慢网连点重复创建/重命名/移动/复制。
const submitting = ref(false)
// 文件名非法字符（与 OS 保留字符对齐）：含其一即拒绝提交。
const ILLEGAL_NAME_RE = /[\/\\:*?"<>|]/

// ── 派生 ──
const curFs = computed(() => (current.value?.kind === 'fs' ? current.value.fs : null))
const curCloud = computed(() => (current.value?.kind === 'cloud' ? current.value.drive : null))
const curReadOnly = computed(() => current.value?.kind === 'cloud')
const curName = computed(() => (current.value ? (current.value.kind === 'fs' ? current.value.fs.name : current.value.drive.name) : ''))
const curId = computed(() => (current.value ? (current.value.kind === 'fs' ? current.value.fs.id : current.value.drive.id) : 0))

function itemKey(it: StorageItem) {
  return it.kind + ':' + (it.kind === 'fs' ? it.fs.id : it.drive.id)
}

// ── 加载 ──
async function loadStorages() {
  loading.value = true
  try {
    const [fs, drives] = await Promise.all([fileApi.list(), cloudApi.listDrives().catch(() => [])])
    list.value = [
      ...fs.map((f) => ({ kind: 'fs' as const, fs: f })),
      ...drives.map((d) => ({ kind: 'cloud' as const, drive: d })),
    ]
    probeAll()
    if (!current.value && list.value.length) selectStorage(list.value[0])
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function loadAgents() {
  try {
    agents.value = await agentApi.list()
  } catch {
    agents.value = []
  }
}

function agentName(id: number) {
  const a = agents.value.find((x) => x.id === id)
  return a ? a.name : ''
}

async function probeAll() {
  // 并行探测（串行在 5+ 存储时明显卡顿）；先标 checking 再合并结果，保留其余项既有状态。
  const pending = { ...statusMap.value }
  for (const it of list.value) pending[itemKey(it)] = { state: 'checking' }
  statusMap.value = pending
  const results = await Promise.all(
    list.value.map(async (it): Promise<[string, { state: ConnState; error?: string }]> => {
      const k = itemKey(it)
      const id = it.kind === 'fs' ? it.fs.id : it.drive.id
      try {
        const ok = it.kind === 'fs' ? (await fileApi.test(id)).ok : (await cloudApi.test(id)).ok
        return [k, ok ? { state: 'ok' } : { state: 'fail', error: '连接失败' }]
      } catch (e: unknown) {
        return [k, { state: 'fail', error: (e as Error).message }]
      }
    }),
  )
  const merged = { ...statusMap.value }
  for (const [k, v] of results) merged[k] = v
  statusMap.value = merged
}

async function testOne(it: StorageItem) {
  const k = itemKey(it)
  testing.value = k
  statusMap.value = { ...statusMap.value, [k]: { state: 'checking' } }
  try {
    const id = it.kind === 'fs' ? it.fs.id : it.drive.id
    const ok = it.kind === 'fs' ? (await fileApi.test(id)).ok : (await cloudApi.test(id)).ok
    statusMap.value = { ...statusMap.value, [k]: ok ? { state: 'ok' } : { state: 'fail', error: '连接失败' } }
  } catch (e: unknown) {
    statusMap.value = { ...statusMap.value, [k]: { state: 'fail', error: (e as Error).message } }
  } finally {
    testing.value = null
  }
}

function statusOf(it: StorageItem): ConnState {
  return statusMap.value[itemKey(it)]?.state ?? 'checking'
}

// 存储项 ⋮ 菜单：测试连接 / 编辑 / 删除。云盘（cloud）的编辑/删除在「云盘」页，此处禁用。
function onStorageMenu(cmd: string, it: StorageItem) {
  if (cmd === 'test') {
    testOne(it)
  } else if (cmd === 'edit' && it.kind === 'fs') {
    openEdit(it.fs)
  } else if (cmd === 'delete' && it.kind === 'fs') {
    delStorage(it.fs)
  }
}

// ── 选择 + 浏览 ──
function selectStorage(it: StorageItem) {
  current.value = it
  path.value = it.kind === 'fs' ? (it.fs.type === 'local' ? '/' : it.fs.base_path || '/') : it.drive.root_path || '/'
  selected.value = []
  refresh()
}

function toEntry(f: CloudFile): FileEntry {
  return { name: f.name, path: f.path, is_dir: f.is_dir, size: f.size, mod_time: f.mod_time }
}

async function refresh() {
  if (!current.value) return
  bloading.value = true
  selected.value = []
  try {
    if (current.value.kind === 'fs') entries.value = await fileApi.files(current.value.fs.id, path.value)
    else entries.value = (await cloudApi.browse(current.value.drive.id, path.value)).map(toEntry)
  } catch (e: unknown) {
    entries.value = []
    ElMessage.warning((e as { response?: { data?: { error?: string } } })?.response?.data?.error || '读取失败')
  } finally {
    bloading.value = false
  }
}

const crumbs = computed(() => {
  if (!current.value) return []
  const root = current.value.kind === 'fs'
    ? (current.value.fs.type === 'local' ? '/' : current.value.fs.base_path || '/')
    : current.value.drive.root_path || '/'
  const rootParts = root.replace(/\/$/, '').split('/').filter(Boolean)
  const curParts = path.value.replace(/\/$/, '').split('/').filter(Boolean)
  const relParts = curParts.slice(rootParts.length)
  const out = [{ name: curName.value, path: root.endsWith('/') ? root : root + '/' }]
  let acc = root.replace(/\/$/, '')
  for (const p of relParts) {
    acc += '/' + p
    out.push({ name: p, path: acc + '/' })
  }
  return out
})

function onSelectionChange(s: FileEntry[]) {
  selected.value = s
}

function enter(e: FileEntry) {
  if (e.is_dir) {
    path.value = e.path
    refresh()
  }
}
function jump(p: string) {
  path.value = p
  refresh()
}
function up() {
  const parts = path.value.replace(/\/$/, '').split('/').filter(Boolean)
  parts.pop()
  path.value = '/' + parts.join('/')
  if (path.value !== '/') path.value += '/'
  refresh()
}
function dlUrl(e: FileEntry) {
  return curFs.value ? fileApi.downloadUrl(curFs.value.id, e.path) : ''
}

// ── 写操作（仅 fs；只读后端模板隐藏入口）──
async function doMkdir() {
  if (!curFs.value || submitting.value) return
  const name = mkdirName.value.trim()
  if (!name) return
  if (ILLEGAL_NAME_RE.test(name)) {
    ElMessage.warning('名称不能包含 / \\ : * ? " < > | 等非法字符')
    return
  }
  submitting.value = true
  try {
    await fileApi.mkdir(curFs.value.id, path.value.replace(/\/$/, '') + '/' + name)
    ElMessage.success('已新建')
    mkdirVisible.value = false
    mkdirName.value = ''
    await refresh()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '新建失败')
  } finally {
    submitting.value = false
  }
}

async function handleUpload(opt: { file: File }) {
  if (!curFs.value) return
  const dup = entries.value.find((e) => !e.is_dir && e.name === opt.file.name)
  if (dup) {
    try {
      await ElMessageBox.confirm(`「${opt.file.name}」已存在，上传将覆盖，是否继续？`, '覆盖确认', { type: 'warning' })
    } catch {
      return
    }
  }
  uploadProgress.value = 0
  try {
    await fileApi.upload(curFs.value.id, path.value, opt.file, (p) => (uploadProgress.value = p))
    ElMessage.success('上传成功')
    await refresh()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '上传失败')
  } finally {
    uploadProgress.value = -1
  }
}

function openRename(e: FileEntry) {
  renameTarget.value = e
  renameNewName.value = e.name
  renameVisible.value = true
}
async function doRename() {
  if (!curFs.value || !renameTarget.value || submitting.value) return
  const name = renameNewName.value.trim()
  if (!name) return
  if (ILLEGAL_NAME_RE.test(name)) {
    ElMessage.warning('名称不能包含 / \\ : * ? " < > | 等非法字符')
    return
  }
  submitting.value = true
  try {
    await fileApi.rename(curFs.value.id, renameTarget.value.path, name)
    ElMessage.success('已重命名')
    renameVisible.value = false
    await refresh()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '重命名失败')
  } finally {
    submitting.value = false
  }
}

async function delOne(e: FileEntry) {
  if (!curFs.value) return
  try {
    await ElMessageBox.confirm(`确认删除「${e.name}」？${e.is_dir ? '（目录及其内容）' : ''}`, '删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    await fileApi.removeFile(curFs.value.id, e.path)
    ElMessage.success('已删除')
    await refresh()
  } catch (e2: unknown) {
    ElMessage.error((e2 as Error).message || '删除失败')
  }
}

async function delBatch() {
  if (!curFs.value || !selected.value.length) return
  try {
    await ElMessageBox.confirm(`确认删除选中的 ${selected.value.length} 项？`, '批量删除', { type: 'warning' })
  } catch {
    return
  }
  let ok = 0
  for (const e of selected.value) {
    try {
      await fileApi.removeFile(curFs.value.id, e.path)
      ok++
    } catch (err) {
      ElMessage.error(`「${e.name}」删除失败：${(err as Error).message}`)
    }
  }
  // 全部失败时不弹 success（避免「已删除 0 项」误导），仅汇总错误。
  if (ok > 0) {
    ElMessage.success(`已删除 ${ok} 项`)
  } else {
    ElMessage.error('选中项全部删除失败')
  }
  await refresh()
}

function openMoveCopy(mode: 'move' | 'copy', sources: FileEntry[]) {
  moveCopyMode.value = mode
  moveCopySources.value = sources
  moveCopyPath.value = path.value
  moveCopyVisible.value = true
  refreshMoveCopy()
}
async function refreshMoveCopy() {
  if (!curFs.value) return
  moveCopyLoading.value = true
  try {
    const all = await fileApi.files(curFs.value.id, moveCopyPath.value)
    moveCopyEntries.value = all.filter((e) => e.is_dir)
  } catch {
    moveCopyEntries.value = []
  } finally {
    moveCopyLoading.value = false
  }
}
function moveCopyEnter(e: FileEntry) {
  moveCopyPath.value = e.path
  refreshMoveCopy()
}
function moveCopyUp() {
  const parts = moveCopyPath.value.replace(/\/$/, '').split('/').filter(Boolean)
  parts.pop()
  moveCopyPath.value = '/' + parts.join('/')
  if (moveCopyPath.value !== '/') moveCopyPath.value += '/'
  refreshMoveCopy()
}
async function doMoveCopy() {
  if (!curFs.value || !moveCopySources.value.length || submitting.value) return
  // 目标目录同名覆盖确认：先取目标已有文件名，源 basename 命中则弹确认（后端静默覆盖会丢数据）。
  // 列目录失败（如权限）则跳过检查交给后端，不阻断操作。
  try {
    const targetEntries = await fileApi.files(curFs.value.id, moveCopyPath.value)
    const existing = new Set(targetEntries.map((e) => e.name))
    const conflicts = moveCopySources.value.filter((s) => existing.has(s.name))
    if (conflicts.length) {
      try {
        await ElMessageBox.confirm(
          `目标存在同名 ${conflicts.length} 项（如「${conflicts[0].name}」），${moveCopyMode.value === 'move' ? '移动' : '复制'}将覆盖，是否继续？`,
          '覆盖确认',
          { type: 'warning' },
        )
      } catch {
        return // 用户取消覆盖
      }
    }
  } catch {
    // 列目标目录失败，跳过覆盖检查
  }
  const fn = moveCopyMode.value === 'move' ? fileApi.move : fileApi.copy
  const verb = moveCopyMode.value === 'move' ? '移动' : '复制'
  submitting.value = true
  let ok = 0
  try {
    for (const e of moveCopySources.value) {
      try {
        await fn(curFs.value.id, e.path, moveCopyPath.value)
        ok++
      } catch (err) {
        ElMessage.error(`「${e.name}」${verb}失败：${(err as Error).message}`)
      }
    }
    // 全部失败时不弹 success（避免「已移动 0 项」误导）；至少一项成功才关对话框并刷新。
    if (ok > 0) {
      ElMessage.success(`已${verb} ${ok} 项`)
      moveCopyVisible.value = false
      await refresh()
    } else {
      ElMessage.error(`选中项全部${verb}失败`)
    }
  } finally {
    submitting.value = false
  }
}

// ── STRM 即时生成（仅云盘目录）──
function openStrm(e: FileEntry) {
  strmDir.value = e
  strmForm.value = { output_dir: '', mount_root: '', template: '', recursive: true, keep_ext: true }
  strmVisible.value = true
}
async function doStrm() {
  if (!curCloud.value || !strmDir.value || !strmForm.value.output_dir) return ElMessage.warning('请填写输出目录')
  strmLoading.value = true
  try {
    const r = await cloudApi.generate({
      drive_id: curCloud.value.id,
      cloud_path: strmDir.value.path,
      output_dir: strmForm.value.output_dir,
      mount_root: strmForm.value.mount_root,
      template: strmForm.value.template || undefined,
      recursive: strmForm.value.recursive,
      keep_ext: strmForm.value.keep_ext,
    })
    ElMessage.success(`STRM 已生成：新增 ${r.generated}，跳过 ${r.skipped}${r.errors.length ? '，失败 ' + r.errors.length : ''}`)
    strmVisible.value = false
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || 'STRM 生成失败')
  } finally {
    strmLoading.value = false
  }
}

// ── 存储后端 CRUD ──
function openCreate() {
  editingId.value = null
  form.value = { name: '', type: 'webdav', url: '', username: '', password: '', base_path: '/', agent_id: 0 }
  storageDialog.value = true
}
function openEdit(fs: FileStorage) {
  editingId.value = fs.id
  form.value = {
    name: fs.name, type: fs.type, url: fs.url, username: fs.username,
    password: '', base_path: fs.base_path || '/', agent_id: fs.agent_id || 0,
  }
  storageDialog.value = true
}
async function submit() {
  if (submitting.value) return
  if (!form.value.name) return ElMessage.warning('请填写名称')
  if (form.value.type === 'agent' && !form.value.agent_id) return ElMessage.warning('请选择 Agent')
  if (form.value.type === 'rclone' && !form.value.username) return ElMessage.warning('请填写 remote 名')
  if (['webdav', 'alist', 'smb'].includes(form.value.type) && !form.value.url) return ElMessage.warning('请填写地址')
  submitting.value = true
  try {
    if (editingId.value) {
      await fileApi.update(editingId.value, form.value)
      ElMessage.success('已更新')
    } else {
      await fileApi.create(form.value)
      ElMessage.success('已添加')
    }
    storageDialog.value = false
    await loadStorages()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '失败')
  } finally {
    submitting.value = false
  }
}
async function delStorage(fs: FileStorage) {
  try {
    await ElMessageBox.confirm(`确认删除存储「${fs.name}」？仅移除配置，不影响远端文件。`, '删除存储', { type: 'warning' })
  } catch {
    return
  }
  try {
    await fileApi.remove(fs.id)
    ElMessage.success('已删除')
    if (curFs.value?.id === fs.id) current.value = null
    await loadStorages()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '删除失败')
  }
}

// 删除正在编辑的存储（对话框内按钮；find 后需 narrow kind 才能取 .fs）。
function delEditingStorage() {
  const found = list.value.find((x) => x.kind === 'fs' && x.fs.id === editingId.value)
  if (found && found.kind === 'fs') {
    delStorage(found.fs)
    storageDialog.value = false
  }
}

function toggleView() {
  viewMode.value = viewMode.value === 'list' ? 'grid' : 'list'
  localStorage.setItem('fm.view', viewMode.value)
  selected.value = []
}
function toggleSelect(e: FileEntry) {
  const i = selected.value.findIndex((s) => s.path === e.path)
  if (i >= 0) selected.value.splice(i, 1)
  else selected.value.push(e)
}
function isSelected(e: FileEntry) {
  return selected.value.some((s) => s.path === e.path)
}

function fmtBytes(n: number) {
  if (!n) return '-'
  const u = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(1)} ${u[i]}`
}
function itemIcon(it: StorageItem) {
  if (it.kind === 'cloud') return '☁'
  return { local: '📁', agent: '🖥', webdav: '🌐', alist: '🗂', smb: '💻', rclone: '🔌' }[it.fs.type] || '💾'
}
function itemLabel(it: StorageItem) {
  return it.kind === 'fs' ? it.fs.type : it.drive.type
}

onMounted(async () => {
  await Promise.all([loadStorages(), loadAgents()])
})
watch(() => storageDialog.value, (v) => { if (v) loadAgents() })
</script>

<template>
  <div class="fm-page" :class="{ embedded: props.embedded }">
    <!-- 左栏 -->
    <aside class="fm-sidebar">
      <div class="sb-head">
        <span>存储</span>
        <el-button size="small" :icon="Refresh" circle :loading="loading" @click="loadStorages" />
      </div>
      <div v-loading="loading" class="sb-list">
        <div
          v-for="it in list"
          :key="itemKey(it)"
          class="sb-item"
          :class="{ active: current && itemKey(current) === itemKey(it) }"
          @click="selectStorage(it)"
        >
          <span class="sb-icon">{{ itemIcon(it) }}</span>
          <div class="sb-main">
            <div class="sb-name">{{ it.kind === 'fs' ? it.fs.name : it.drive.name }}</div>
            <div class="sb-type">{{ itemLabel(it) }}<span v-if="it.kind === 'cloud'" class="ro">·只读</span></div>
          </div>
          <span class="dot" :class="statusOf(it)" :title="statusOf(it) === 'fail' ? '连接失败' : statusOf(it) === 'ok' ? '连接正常' : '检测中…'" @click.stop="testOne(it)"></span>
          <el-dropdown trigger="click" placement="bottom-end" @command="(c) => onStorageMenu(c as string, it)">
            <span class="sb-more" @click.stop>⋯</span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="test">测试连接</el-dropdown-item>
                <template v-if="it.kind === 'fs'">
                  <el-dropdown-item command="edit">编辑</el-dropdown-item>
                  <el-dropdown-item command="delete" divided>删除存储</el-dropdown-item>
                </template>
                <el-dropdown-item v-else disabled>云盘请在「云盘」页管理</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
        <div v-if="!list.length && !loading" class="sb-empty">
          <EmptyState icon="📁" title="还没有存储" description="接入本机、WebDAV、SMB、Agent 或云盘后即可浏览文件。" />
        </div>
      </div>
      <div class="sb-actions">
        <el-button :icon="Plus" @click="openCreate">添加存储</el-button>
      </div>
    </aside>

    <!-- 右栏 -->
    <section class="fm-main">
      <template v-if="current">
        <div class="fm-toolbar">
          <el-button size="small" :icon="Top" :disabled="path === '/'" @click="up">上级</el-button>
          <div class="crumbs">
            <template v-for="(c, i) in crumbs" :key="i">
              <span class="crumb" @click="jump(c.path)">{{ c.name }}</span>
              <span v-if="i < crumbs.length - 1" class="crumb-sep">/</span>
            </template>
          </div>
          <div class="tb-actions">
            <el-button v-if="!curReadOnly" size="small" :icon="FolderAdd" @click="mkdirVisible = true">新建文件夹</el-button>
            <el-upload v-if="!curReadOnly" :show-file-list="false" :http-request="handleUpload">
              <el-button size="small" type="primary" :icon="Upload" :disabled="uploadProgress >= 0">上传</el-button>
            </el-upload>
            <el-button size="small" @click="toggleView" :title="viewMode === 'list' ? '网格' : '列表'">{{ viewMode === 'list' ? '▤' : '☰' }}</el-button>
            <el-button size="small" :icon="Refresh" circle @click="refresh" />
          </div>
        </div>
        <el-progress v-if="uploadProgress >= 0" :percentage="uploadProgress" :stroke-width="6" style="margin: 0 12px 8px" />

        <transition name="fade">
          <div v-if="selected.length && !curReadOnly" class="batch-bar glass">
            <span>已选 {{ selected.length }} 项</span>
            <el-button size="small" :icon="MoveIcon" @click="openMoveCopy('move', selected)">移动</el-button>
            <el-button size="small" :icon="CopyDocument" @click="openMoveCopy('copy', selected)">复制</el-button>
            <el-button size="small" type="danger" plain :icon="Delete" @click="delBatch">删除</el-button>
            <el-button size="small" text @click="selected = []">取消</el-button>
          </div>
        </transition>

        <div v-loading="bloading" class="fm-files">
          <el-table v-if="viewMode === 'list'" :data="entries" empty-text="该目录为空" @selection-change="onSelectionChange" style="width: 100%">
            <el-table-column v-if="!curReadOnly" type="selection" width="40" />
            <el-table-column label="名称" min-width="220">
              <template #default="{ row }">
                <a v-if="row.is_dir" class="link" @click="enter(row as FileEntry)"><el-icon><Folder /></el-icon>&nbsp;{{ row.name }}</a>
                <span v-else><el-icon><Document /></el-icon>&nbsp;{{ row.name }}</span>
              </template>
            </el-table-column>
            <el-table-column label="大小" width="100"><template #default="{ row }">{{ row.is_dir ? '-' : fmtBytes(row.size) }}</template></el-table-column>
            <el-table-column :label="curReadOnly ? '操作' : ''" :width="curReadOnly ? 130 : 280">
              <template #default="{ row }">
                <el-button v-if="row.is_dir && curReadOnly" size="small" link :icon="Link" @click="openStrm(row as FileEntry)">生成 STRM</el-button>
                <template v-else-if="!curReadOnly">
                  <el-button v-if="!row.is_dir" size="small" link :icon="Download" tag="a" :href="dlUrl(row as FileEntry)" target="_blank">下载</el-button>
                  <el-button size="small" link :icon="Edit" @click="openRename(row as FileEntry)">重命名</el-button>
                  <el-button size="small" link :icon="MoveIcon" @click="openMoveCopy('move', [row as FileEntry])">移动</el-button>
                  <el-button size="small" link :icon="CopyDocument" @click="openMoveCopy('copy', [row as FileEntry])">复制</el-button>
                  <el-button size="small" link type="danger" :icon="Delete" @click="delOne(row as FileEntry)">删除</el-button>
                </template>
              </template>
            </el-table-column>
          </el-table>

          <div v-else class="grid-view">
            <div v-for="e in entries" :key="e.path" class="grid-card glass glass-hover" :class="{ sel: !curReadOnly && !e.is_dir && isSelected(e) }" @click="e.is_dir ? enter(e) : (!curReadOnly && toggleSelect(e))">
              <div class="grid-icon">{{ e.is_dir ? '📁' : '🎬' }}</div>
              <div class="grid-name" :title="e.name">{{ e.name }}</div>
              <div class="grid-meta">{{ e.is_dir ? '文件夹' : fmtBytes(e.size) }}</div>
              <el-button v-if="e.is_dir && curReadOnly" size="small" link class="grid-strm" :icon="Link" @click.stop="openStrm(e)">STRM</el-button>
            </div>
            <div v-if="!entries.length" class="grid-empty">该目录为空</div>
          </div>
        </div>
      </template>

      <div v-else class="fm-empty">
        <EmptyState icon="📂" :title="list.length ? '请选择左侧存储' : '还没有接入存储'" :description="list.length ? '点击左侧任一存储开始浏览。' : '点击左下角「添加存储」接入。'" />
      </div>
    </section>

    <!-- 存储添加/编辑 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="storageDialog" :title="editingId ? '编辑存储' : '添加存储'" width="460">
      <el-form label-width="80">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="local">本机</el-radio>
            <el-radio value="webdav">WebDAV</el-radio>
            <el-radio value="alist">Alist</el-radio>
            <el-radio value="smb">SMB</el-radio>
            <el-radio value="rclone">Rclone</el-radio>
            <el-radio value="agent">Agent</el-radio>
          </el-radio-group>
          <div v-if="form.type === 'local'" class="hint">本机：直接读写服务所在机器的本地目录。根路径填绝对路径（如 <code>D:\Media</code> 或 <code>/mnt/nas</code>）。⚠️ 仅配置可信目录。</div>
          <div v-else-if="form.type === 'webdav'" class="hint">WebDAV：地址填完整 URL（如 http://host:5005）。</div>
          <div v-else-if="form.type === 'alist'" class="hint">Alist：地址填服务地址（如 http://host:5244），账密默认可留空。</div>
          <div v-else-if="form.type === 'rclone'" class="hint">Rclone：二进制路径填 url，remote 名填 username，密码框粘 rclone.conf 全文。</div>
        </el-form-item>
        <template v-if="form.type === 'agent'">
          <el-form-item label="Agent">
            <el-select v-model="form.agent_id" placeholder="选择 Agent" style="width: 100%">
              <el-option v-for="a in agents" :key="a.id" :label="`${a.name}${a.online ? '' : '（离线）'}`" :value="a.id" />
            </el-select>
          </el-form-item>
          <div class="hint">Agent：选择已部署 agent 的设备，根路径填 agent 机器上的绝对路径。</div>
        </template>
        <template v-else-if="form.type === 'rclone'">
          <el-form-item label="二进制"><el-input v-model="form.url" placeholder="rclone（留空用 PATH）" /></el-form-item>
          <el-form-item label="remote"><el-input v-model="form.username" placeholder="如 myremote" /></el-form-item>
          <el-form-item label="conf"><el-input v-model="form.password" type="textarea" :rows="5" :placeholder="editingId ? '留空不修改' : '粘贴 rclone.conf'" /></el-form-item>
        </template>
        <template v-else-if="form.type !== 'local'">
          <el-form-item label="地址"><el-input v-model="form.url" /></el-form-item>
          <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
          <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password :placeholder="editingId ? '留空不修改' : ''" /></el-form-item>
          <div v-if="form.type === 'smb'" class="hint">SMB：地址填 smb://主机[:端口]，根路径填 /共享名/子目录。</div>
        </template>
        <el-form-item label="根路径"><el-input v-model="form.base_path" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button v-if="editingId" type="danger" plain @click="delEditingStorage">删除该存储</el-button>
        <el-button @click="storageDialog = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="submit">{{ editingId ? '保存' : '添加' }}</el-button>
      </template>
    </el-dialog>

    <!-- 新建文件夹 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="mkdirVisible" title="新建文件夹" width="360">
      <el-input v-model="mkdirName" placeholder="文件夹名称" @keyup.enter="doMkdir" />
      <template #footer>
        <el-button @click="mkdirVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="doMkdir">新建</el-button>
      </template>
    </el-dialog>

    <!-- 重命名 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="renameVisible" title="重命名" width="360">
      <el-input v-model="renameNewName" @keyup.enter="doRename" />
      <template #footer>
        <el-button @click="renameVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="doRename">确认</el-button>
      </template>
    </el-dialog>

    <!-- 移动/复制 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="moveCopyVisible" :title="moveCopyMode === 'move' ? '移动到…' : '复制到…'" width="560">
      <div class="mc-bar">
        <el-button size="small" :icon="Top" :disabled="moveCopyPath === '/'" @click="moveCopyUp">上级</el-button>
        <span class="mc-path">{{ moveCopyPath }}</span>
      </div>
      <div v-loading="moveCopyLoading" class="mc-list">
        <div v-for="d in moveCopyEntries" :key="d.path" class="mc-item" @click="moveCopyEnter(d)">
          <el-icon><Folder /></el-icon>&nbsp;{{ d.name }}
        </div>
        <div v-if="!moveCopyEntries.length && !moveCopyLoading" class="mc-empty">（无子文件夹）</div>
      </div>
      <template #footer>
        <el-button @click="moveCopyVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="doMoveCopy">{{ moveCopyMode === 'move' ? '移动到此处' : '复制到此处' }}</el-button>
      </template>
    </el-dialog>

    <!-- STRM 即时生成 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="strmVisible" title="生成 STRM" width="480">
      <div class="hint">为云盘目录「{{ strmDir?.name }}」下的媒体文件生成本地 .strm（媒体服务器读取后直链播放云盘资源，无需落地）。</div>
      <el-form label-width="88" style="margin-top: 8px">
        <el-form-item label="输出目录"><el-input v-model="strmForm.output_dir" placeholder="本地 .strm 输出根目录，如 /Media/STRM" /></el-form-item>
        <el-form-item label="挂载根"><el-input v-model="strmForm.mount_root" placeholder="如 /CloudDrive2/115（模板 {mount}）" /></el-form-item>
        <el-form-item label="模板"><el-input v-model="strmForm.template" placeholder="留空用默认 {mount}{cloudpath}" /></el-form-item>
        <el-form-item label="递归"><el-switch v-model="strmForm.recursive" /></el-form-item>
        <el-form-item label="保留扩展名"><el-switch v-model="strmForm.keep_ext" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="strmVisible = false">取消</el-button>
        <el-button type="primary" :loading="strmLoading" @click="doStrm">生成</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.fm-page { display: flex; gap: 14px; height: calc(100vh - 120px); min-height: 480px; }
.fm-sidebar { flex: 0 0 220px; display: flex; flex-direction: column; gap: 10px; }
.sb-head { display: flex; align-items: center; justify-content: space-between; font-weight: 700; font-size: 14px; padding: 0 4px; }
.sb-list { flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; padding: 6px; border: 1px solid var(--glass-border); border-radius: 10px; background: color-mix(in srgb, var(--glass-bg) 86%, transparent); }
.sb-item { display: flex; align-items: center; gap: 10px; padding: 9px 10px; border-radius: 8px; cursor: pointer; transition: background 0.15s; }
.sb-item:hover { background: color-mix(in srgb, var(--pt-primary, #4361ee) 10%, transparent); }
.sb-item.active { background: linear-gradient(135deg, color-mix(in srgb, var(--pt-primary, #6366f1) 22%, transparent), color-mix(in srgb, var(--pt-primary2, #4361ee) 16%, transparent)); }
.sb-icon { font-size: 20px; }
.sb-main { flex: 1; min-width: 0; }
.sb-name { font-weight: 600; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.sb-type { font-size: 11px; color: var(--text-muted, #909399); text-transform: uppercase; }
.sb-type .ro { color: #f59e0b; text-transform: none; margin-left: 3px; }
.sb-empty { padding: 24px 8px; }
.sb-actions { display: flex; flex-direction: column; gap: 6px; }
.sb-actions .el-button { width: 100%; }
.dot { width: 9px; height: 9px; border-radius: 50%; flex-shrink: 0; cursor: pointer; }
.dot.checking { background: var(--text-muted, #909399); opacity: 0.5; }
.dot.ok { background: #2ecc71; box-shadow: 0 0 4px rgba(46, 204, 113, 0.6); }
.dot.fail { background: #e74c3c; }
.sb-more { cursor: pointer; padding: 0 2px; font-size: 18px; line-height: 1; color: var(--text-muted, #909399); flex-shrink: 0; }
.sb-more:hover { color: var(--pt-primary, #4361ee); }

.fm-main { flex: 1; min-width: 0; display: flex; flex-direction: column; border: 1px solid var(--glass-border); border-radius: 10px; background: color-mix(in srgb, var(--glass-bg) 86%, transparent); overflow: hidden; }
.fm-toolbar { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-bottom: 1px solid var(--glass-border); flex-wrap: wrap; }
.crumbs { flex: 1; min-width: 0; display: flex; align-items: center; gap: 4px; overflow-x: auto; padding: 2px 0; }
.crumbs::-webkit-scrollbar { height: 4px; }
.crumb { color: var(--pt-primary, #4361ee); cursor: pointer; white-space: nowrap; font-size: 13px; }
.crumb:hover { text-decoration: underline; }
.crumb-sep { color: var(--text-muted, #909399); }
.tb-actions { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }
.batch-bar { display: flex; align-items: center; gap: 10px; padding: 8px 14px; margin: 10px 12px 0; border-radius: 8px; font-size: 13px; }
.fm-files { flex: 1; overflow-y: auto; padding: 8px 12px 12px; }
.link { color: var(--pt-primary, #4361ee); cursor: pointer; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }

.grid-view { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 12px; padding: 6px 0; }
.grid-card { position: relative; padding: 14px 10px; border-radius: 10px; cursor: pointer; text-align: center; transition: transform 0.12s, box-shadow 0.12s; }
.grid-card:hover { transform: translateY(-2px); }
.grid-card.sel { border-color: var(--pt-primary, #4361ee); box-shadow: 0 0 0 2px color-mix(in srgb, var(--pt-primary, #4361ee) 40%, transparent); }
.grid-icon { font-size: 34px; }
.grid-name { margin-top: 6px; font-size: 12px; font-weight: 600; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.grid-meta { font-size: 11px; color: var(--text-muted, #909399); }
.grid-strm { margin-top: 6px; }
.grid-empty { grid-column: 1 / -1; text-align: center; padding: 40px; color: var(--text-muted, #909399); }

.fm-empty { flex: 1; display: grid; place-items: center; }
.hint { margin: -4px 0 8px; padding: 8px 12px; font-size: 12px; color: var(--text-muted, #909399); background: var(--pt-bg-soft, rgba(255, 255, 255, 0.04)); border-radius: 6px; line-height: 1.6; }

.mc-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.mc-path { flex: 1; font-family: monospace; font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.mc-list { max-height: 320px; overflow-y: auto; border: 1px solid var(--glass-border); border-radius: 8px; padding: 6px; }
.mc-item { display: flex; align-items: center; padding: 7px 10px; border-radius: 6px; cursor: pointer; font-size: 13px; }
.mc-item:hover { background: color-mix(in srgb, var(--pt-primary, #4361ee) 10%, transparent); }
.mc-empty { text-align: center; color: var(--text-muted, #909399); padding: 20px; }

.fade-enter-active, .fade-leave-active { transition: opacity 0.18s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

@media (max-width: 760px) {
  .fm-page { flex-direction: column; height: auto; }
  .fm-sidebar { flex: none; }
  .sb-list { max-height: 200px; }
}
.fm-page.embedded { height: calc(100vh - 240px); min-height: 420px; }
</style>
