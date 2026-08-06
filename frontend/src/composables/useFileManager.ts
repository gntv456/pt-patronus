import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fileApi, type FileStorage, type FileEntry } from '@/api/filemanager'
import { cloudApi, type CloudDrive, type CloudFile } from '@/api/cloud'
import { agentApi, type Agent } from '@/api/monitor'

// 存储项统一：本地/WebDAV/SMB 等（可读写）｜云盘（115/夸克/123，只读 + STRM）。
export type StorageItem = { kind: 'fs'; fs: FileStorage } | { kind: 'cloud'; drive: CloudDrive }
type ConnState = 'checking' | 'ok' | 'fail'

// 文件名非法字符（与 OS 保留字符对齐）：含其一即拒绝提交。
const ILLEGAL_NAME_RE = /[\/\\:*?"<>|]/

export function useFileManager() {
  const list = ref<StorageItem[]>([])
  const loading = ref(false)
  const agents = ref<Agent[]>([])
  const current = ref<StorageItem | null>(null)

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
      const [fsList, drives] = await Promise.all([fileApi.list(), cloudApi.listDrives().catch(() => [])])
      list.value = [
        ...fsList.map((f) => ({ kind: 'fs' as const, fs: f })),
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

  return {
    list, loading, agents, current, statusMap, testing,
    path, entries, bloading, viewMode, selected, uploadProgress,
    storageDialog, editingId, form,
    mkdirVisible, mkdirName, renameVisible, renameTarget, renameNewName,
    moveCopyVisible, moveCopyMode, moveCopyPath, moveCopyEntries, moveCopyLoading, moveCopySources,
    strmVisible, strmDir, strmForm, strmLoading, submitting,
    curFs, curCloud, curReadOnly, curName, curId, crumbs,
    itemKey, loadStorages, loadAgents, agentName, probeAll, testOne, statusOf,
    selectStorage, refresh, onSelectionChange, enter, jump, up, dlUrl,
    ILLEGAL_NAME_RE,
  }
}
