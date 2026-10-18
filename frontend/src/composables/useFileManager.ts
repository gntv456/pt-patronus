import { ref, computed, onMounted, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { fileApi, type FileStorage, type FileEntry } from '@/api/filemanager'
import { cloudApi, type CloudDrive, type CloudFile } from '@/api/cloud'
import { agentApi, type Agent } from '@/api/monitor'
import { useAuthStore } from '@/stores/auth'

// 存储项统一：本地/WebDAV/SMB 等（可读写）｜云盘（115/夸克/123，只读 + STRM）。
export type StorageItem = { kind: 'fs'; fs: FileStorage } | { kind: 'cloud'; drive: CloudDrive }
type ConnState = 'checking' | 'ok' | 'fail'

// 文件名非法字符（与 OS 保留字符对齐）：含其一即拒绝提交。
const ILLEGAL_NAME_RE = /[\/\\:*?"<>|]/

export function useFileManager() {
  const auth = useAuthStore()
  const list = ref<StorageItem[]>([])
  const loading = ref(false)
  const agents = ref<Agent[]>([])
  // Agent 列表读失败的原因：空下拉既可能是「真没配」也可能是「没读到」，界面必须分得清。
  const agentsError = ref('')
  const current = ref<StorageItem | null>(null)
  // 云盘列表读失败置位：区分「没配网盘」与「网盘列表没读到」。
  const drivesFailed = ref(false)
  // 存储后端（/storages 整组是 admin-only）同样要分得开：读失败 / 无权限 / 真的没配。
  const fsFailed = ref(false)
  const fsForbidden = ref(false)

  const statusMap = ref<Record<string, { state: ConnState; error?: string }>>({})
  const testing = ref<string | null>(null)

  // 浏览
  const path = ref('/')
  const entries = ref<FileEntry[]>([])
  const bloading = ref(false)
  const viewMode = ref<'list' | 'grid'>(localStorage.getItem('fm.view') === 'grid' ? 'grid' : 'list')
  const selected = ref<FileEntry[]>([])
  const uploadProgress = ref(-1)
  // 批量删除进度：{ done, total, active } | null（null=不在删除中）
  const batchDelProgress = ref<{ done: number; total: number } | null>(null)

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
  // 在线编辑
  const editVisible = ref(false)
  const editTarget = ref<FileEntry | null>(null)
  const editContent = ref('')
  const editLoading = ref(false)
  // 压缩
  const archiveVisible = ref(false)
  const archiveTarget = ref<FileEntry | null>(null)
  const archiveName = ref('')
  const archiveFmt = ref<'zip' | 'targz'>('targz')
  const archiveLoading = ref(false)
  // 权限
  const chmodVisible = ref(false)
  const chmodTarget = ref<FileEntry | null>(null)
  const chmodMode = ref('755')
  const chmodRecursive = ref(false)
  const chmodLoading = ref(false)
  // 目录大小（行内显示）
  const dirSizeMap = ref<Record<string, string>>({})
  const dirSizeLoading = ref<Record<string, boolean>>({})
  // 文件预览
  const previewVisible = ref(false)
  const previewTarget = ref<FileEntry | null>(null)
  const previewContent = ref('')
  const previewLoading = ref(false)
  const submitting = ref(false)

  // ── 派生 ──
  const curFs = computed(() => (current.value?.kind === 'fs' ? current.value.fs : null))
  const curCloud = computed(() => (current.value?.kind === 'cloud' ? current.value.drive : null))
  const curReadOnly = computed(() => current.value?.kind === 'cloud')
  const curName = computed(() =>
    current.value ? (current.value.kind === 'fs' ? current.value.fs.name : current.value.drive.name) : '',
  )
  const curId = computed(() =>
    current.value ? (current.value.kind === 'fs' ? current.value.fs.id : current.value.drive.id) : 0,
  )

  function itemKey(it: StorageItem) {
    return it.kind + ':' + (it.kind === 'fs' ? it.fs.id : it.drive.id)
  }

  // ── 加载 ──
  async function loadStorages() {
    loading.value = true
    drivesFailed.value = false
    fsFailed.value = false
    fsForbidden.value = false
    try {
      let drives: CloudDrive[] = []
      let fsList: FileStorage[] = []
      let dFail = false
      let fFail = false
      // 两侧各自留痕、互不阻断：旧写法只给云盘那侧挂了 catch，
      // 存储后端读失败（含非管理员必然的 403）会把整个 Promise.all 抛掉，
      // 连带本可正常浏览的云盘也进不了侧栏——只剩一条错误提示。
      const fsP = (async () => {
        if (!auth.isAdmin) {
          fsForbidden.value = true
          return
        }
        try {
          fsList = await fileApi.list()
        } catch (e: unknown) {
          if ((e as { response?: { status?: number } }).response?.status === 403) fsForbidden.value = true
          else fFail = true
        }
      })()
      const cloudP = (async () => {
        try {
          drives = await cloudApi.listDrives()
        } catch {
          dFail = true
        }
      })()
      await Promise.all([fsP, cloudP])
      fsFailed.value = fFail
      drivesFailed.value = dFail
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
      agentsError.value = ''
    } catch (e: unknown) {
      // 从前这里只写 agents.value = []：读失败和「这台主机真没配 Agent」在界面上是同一个空下拉，
      // 用户会以为自己没建过（或去重复建一个）。原因带出来，重开对话框会自动再取一次。
      agents.value = []
      // http 拦截器已把后端信封的 message 改写进 err.message（无则退回异常名）。
      agentsError.value = (e as Error)?.message || '未知错误'
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
      const be = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      const msg = be || (e as Error).message || ''
      ElMessage.warning(`读取「${path.value}」失败${msg ? '：' + msg : ''}`)
    } finally {
      bloading.value = false
    }
  }

  const crumbs = computed(() => {
    if (!current.value) return []
    const root =
      current.value.kind === 'fs'
        ? current.value.fs.type === 'local'
          ? '/'
          : current.value.fs.base_path || '/'
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
    list,
    loading,
    drivesFailed,
    fsFailed,
    fsForbidden,
    agents,
    agentsError,
    current,
    statusMap,
    testing,
    path,
    entries,
    bloading,
    viewMode,
    selected,
    uploadProgress,
    batchDelProgress,
    storageDialog,
    editingId,
    form,
    mkdirVisible,
    mkdirName,
    renameVisible,
    renameTarget,
    renameNewName,
    moveCopyVisible,
    moveCopyMode,
    moveCopyPath,
    moveCopyEntries,
    moveCopyLoading,
    moveCopySources,
    strmVisible,
    strmDir,
    strmForm,
    strmLoading,
    submitting,
    editVisible,
    editTarget,
    editContent,
    editLoading,
    archiveVisible,
    archiveTarget,
    archiveName,
    archiveFmt,
    archiveLoading,
    chmodVisible,
    chmodTarget,
    chmodMode,
    chmodRecursive,
    chmodLoading,
    dirSizeMap,
    dirSizeLoading,
    previewVisible,
    previewTarget,
    previewContent,
    previewLoading,
    curFs,
    curCloud,
    curReadOnly,
    curName,
    curId,
    crumbs,
    itemKey,
    loadStorages,
    loadAgents,
    agentName,
    probeAll,
    testOne,
    statusOf,
    selectStorage,
    refresh,
    onSelectionChange,
    enter,
    jump,
    up,
    dlUrl,
    ILLEGAL_NAME_RE,
  }
}
