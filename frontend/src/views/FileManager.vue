<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { fileApi, type FileStorage, type FileEntry } from '@/api/filemanager'
import { agentApi, type Agent } from '@/api/monitor'
import { Folder, Document, Top } from '@element-plus/icons-vue'

const list = ref<FileStorage[]>([])
const loading = ref(false)
const error = ref('')
const dialogVisible = ref(false)
const form = ref({ name: '', type: 'webdav', url: '', username: '', password: '', base_path: '/', agent_id: 0 })

// Agent 选择器（type=agent 时用）
const agents = ref<Agent[]>([])

// 浏览
const browseVisible = ref(false)
const current = ref<FileStorage | null>(null)
const path = ref('/')
const entries = ref<FileEntry[]>([])
const bloading = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    list.value = await fileApi.list()
  } catch (e: unknown) {
    error.value = (e as Error).message || '加载失败'
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

function openCreate() {
  form.value = { name: '', type: 'webdav', url: '', username: '', password: '', base_path: '/', agent_id: 0 }
  loadAgents()
  dialogVisible.value = true
}

async function submitCreate() {
  if (!form.value.name) {
    ElMessage.warning('请填写名称')
    return
  }
  if (form.value.type === 'agent') {
    if (!form.value.agent_id) {
      ElMessage.warning('请选择 Agent')
      return
    }
  } else if (!form.value.url) {
    ElMessage.warning('请填写地址')
    return
  }
  try {
    await fileApi.create(form.value)
    dialogVisible.value = false
    ElMessage.success('已添加')
    await load()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '失败')
  }
}

async function browse(fs: FileStorage) {
  current.value = fs
  path.value = fs.base_path || '/'
  browseVisible.value = true
  await refresh()
}

async function refresh() {
  if (!current.value) return
  bloading.value = true
  try {
    entries.value = await fileApi.files(current.value.id, path.value)
  } catch (e: unknown) {
    entries.value = []
    ElMessage.warning((e as { response?: { data?: { error?: string } } })?.response?.data?.error || '读取失败')
  } finally {
    bloading.value = false
  }
}

function enter(e: FileEntry) {
  if (e.is_dir) {
    path.value = e.path
    refresh()
  }
}
function up() {
  const parts = path.value.replace(/\/$/, '').split('/').filter(Boolean)
  parts.pop()
  path.value = '/' + parts.join('/')
  if (path.value !== '/') path.value += '/'
  refresh()
}
function dlUrl(e: FileEntry) {
  return current.value ? fileApi.downloadUrl(current.value.id, e.path) : ''
}

async function del(e: FileEntry) {
  if (!current.value) return
  try {
    await ElMessageBox.confirm(`确认删除「${e.name}」？${e.is_dir ? '（目录及其内容）' : ''}`, '删除', { type: 'warning' })
  } catch {
    return
  }
  try {
    await fileApi.removeFile(current.value.id, e.path)
    ElMessage.success('已删除')
    await refresh()
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || '删除失败')
  }
}

async function handleUpload(opt: { file: File }) {
  if (!current.value) return
  // 同名文件已存在则二次确认：多数 WebDAV/SMB 后端遇同名会静默覆盖，避免无提示丢文件
  const dup = entries.value.find((e) => !e.is_dir && e.name === opt.file.name)
  if (dup) {
    try {
      await ElMessageBox.confirm(`「${opt.file.name}」已存在，上传将覆盖原文件，是否继续？`, '覆盖确认', { type: 'warning' })
    } catch {
      return
    }
  }
  try {
    await fileApi.upload(current.value.id, path.value, opt.file)
    ElMessage.success('上传成功')
    await refresh()
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '上传失败')
  }
}

function fmtBytes(n: number) {
  if (!n) return '-'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  while (n >= 1024 && i < u.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(1)} ${u[i]}`
}

onMounted(async () => {
  await load()
  await loadAgents()
})
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">文件管理</h2>
      <el-button type="primary" @click="openCreate">+ 添加存储</el-button>
    </div>
    <div v-loading="loading" class="grid">
      <div v-if="error" class="empty glass error-state">
        <div class="err-text">⚠ {{ error }}</div>
        <el-button size="small" @click="load">重试</el-button>
      </div>
      <template v-else>
        <div v-for="fs in list" :key="fs.id" class="glass glass-hover card" @click="browse(fs)">
          <div class="row1"><span class="dtype">{{ fs.type }}</span><span class="dname">{{ fs.name }}</span></div>
          <div class="url text-muted">
            {{ fs.type === 'agent' ? (agentName(fs.agent_id) ? 'Agent: ' + agentName(fs.agent_id) : 'Agent #' + fs.agent_id) : fs.url }}{{ fs.base_path }}
          </div>
        </div>
        <div v-if="!list.length && !loading" class="empty glass">暂无存储后端</div>
      </template>
    </div>

    <el-dialog v-model="dialogVisible" title="添加存储" width="460">
      <el-form label-width="80">
        <el-form-item label="名称"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="类型">
          <el-radio-group v-model="form.type">
            <el-radio value="webdav">WebDAV</el-radio>
            <el-radio value="alist">Alist</el-radio>
            <el-radio value="smb">SMB</el-radio>
            <el-radio value="agent">Agent</el-radio>
          </el-radio-group>
          <div v-if="form.type === 'webdav'" class="hint">WebDAV：地址填完整 URL（如 http://host:5005），用户名密码按服务端配置填写。</div>
          <div v-if="form.type === 'alist'" class="hint">Alist：地址填 Alist 服务地址（如 http://host:5244），用户名密码按 Alist 配置填写（默认可留空）。</div>
        </el-form-item>
        <template v-if="form.type === 'agent'">
          <el-form-item label="Agent">
            <el-select v-model="form.agent_id" placeholder="选择 Agent" style="width: 100%">
              <el-option v-for="a in agents" :key="a.id" :label="`${a.name}${a.online ? '' : '（离线）'}`" :value="a.id" />
            </el-select>
          </el-form-item>
          <div class="hint">Agent：选择已部署 ptpatronus-agent 的设备（飞牛/绿联等），根路径填 agent 机器上的绝对路径（如 /mnt/nas）。设备离线或 agent 版本过低（需 0.2.0+）时文件操作不可用。</div>
        </template>
        <template v-else>
          <el-form-item label="地址"><el-input v-model="form.url" /></el-form-item>
          <el-form-item label="用户名"><el-input v-model="form.username" /></el-form-item>
          <el-form-item label="密码"><el-input v-model="form.password" type="password" show-password /></el-form-item>
          <div v-if="form.type === 'smb'" class="hint">SMB：地址填 smb://主机[:端口]（默认 445），根路径填 /共享名/子目录（如 /Media/Movies）。仅支持 SMB2/3。</div>
        </template>
        <el-form-item label="根路径"><el-input v-model="form.base_path" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitCreate">添加</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="browseVisible" :title="current?.name" size="55%">
      <div class="pathbar">
        <el-button size="small" @click="up"><el-icon><Top /></el-icon>&nbsp;上级</el-button>
        <span class="curpath">{{ path }}</span>
        <el-upload :show-file-list="false" :http-request="handleUpload">
          <el-button size="small" type="primary">上传</el-button>
        </el-upload>
        <el-button size="small" @click="refresh">刷新</el-button>
      </div>
      <el-table v-loading="bloading" :data="entries" empty-text="该目录为空" style="width: 100%">
        <el-table-column label="名称" min-width="200">
          <template #default="{ row }">
            <a v-if="row.is_dir" class="link" @click="enter(row as FileEntry)"><el-icon><Folder /></el-icon>&nbsp;{{ row.name }}</a>
            <span v-else><el-icon><Document /></el-icon>&nbsp;{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="大小" width="90"><template #default="{ row }">{{ fmtBytes(row.size) }}</template></el-table-column>
        <el-table-column label="操作" width="140">
          <template #default="{ row }">
            <el-button v-if="!row.is_dir" size="small" tag="a" :href="dlUrl(row as FileEntry)" target="_blank">下载</el-button>
            <el-button size="small" type="danger" plain @click="del(row as FileEntry)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
.card { padding: 18px; cursor: pointer; }
.row1 { display: flex; align-items: center; gap: 10px; }
.dtype { padding: 2px 8px; border-radius: 8px; background: linear-gradient(135deg, #2ecc71, #4285f4); color: #fff; font-size: 12px; font-weight: 600; }
.dname { font-weight: 600; }
.url { margin-top: 8px; font-size: 13px; word-break: break-all; }
.hint { margin: -4px 0 8px; padding: 8px 12px; font-size: 12px; color: var(--pt-muted, #909399); background: var(--pt-bg-soft, rgba(255,255,255,0.04)); border-radius: 6px; line-height: 1.6; }
.empty { padding: 40px; text-align: center; }
.error-state { display: flex; flex-direction: column; align-items: center; gap: 12px; }
.err-text { color: var(--pt-danger); font-size: 14px; }
.pathbar { display: flex; align-items: center; gap: 10px; margin-bottom: 12px; }
.curpath { flex: 1; font-family: monospace; font-size: 13px; }
.link { color: var(--pt-primary); cursor: pointer; }
:deep(.el-table), :deep(.el-table tr), :deep(.el-table td), :deep(.el-table th) { background-color: transparent !important; }
</style>
