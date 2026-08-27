<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  cloudApi,
  TYPE_LABEL,
  TASK_KIND_LABEL,
  TASK_STATUS_LABEL,
  type CloudDrive,
  type CloudFile,
  type CloudTask,
  type DriveInput,
} from '@/api/cloud'
import type { StrmRule, StrmRecord } from '@/api/cloud'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { useCloudDriveBrowse, fmtSize } from '@/composables/useCloudDriveBrowse'
import { useCloudStrmRules } from '@/composables/useCloudStrmRules'

// ---- 云端文件管理器操作 ----
const curDrive = computed(() => browseDrive.value)
async function fileOp(op: 'mkdir' | 'delete' | 'move' | 'copy' | 'rename', extra: Record<string, string> = {}) {
  if (!curDrive.value || !browsePath.value) {
    ElMessage.warning('请先选择网盘并进入目录')
    return
  }
  try {
    if (op === 'delete') {
      await ElMessageBox.confirm(`确认删除 ${extra.path ?? ''}？此操作不可恢复。`, '删除', { type: 'warning' })
    }
    await cloudApi.fileOp(op, { drive_id: curDrive.value.id, path: browsePath.value, ...extra })
    ElMessage.success('操作成功')
    await fetchBrowse()
  } catch (e) {
    if ((e as string) !== 'cancel') ElMessage.error((e as Error).message || '操作失败')
  }
}
async function doMkdir() {
  try {
    const { value } = await ElMessageBox.prompt('新建文件夹名称', '新建', { inputPattern: /^[^/\\]+$/, inputErrorMessage: '名称非法' })
    await fileOp('mkdir', { new_name: value.trim() })
  } catch { /* cancel */ }
}
async function doOffline() {
  try {
    const { value } = await ElMessageBox.prompt('磁力或 HTTP 下载链接（任务提交到网盘离线）', '离线下载到当前目录')
    await cloudApi.fileOp('offline', { drive_id: curDrive.value!.id, path: browsePath.value, url: value.trim() })
    ElMessage.success('离线任务已提交（在网盘 App 查看进度）')
  } catch { /* cancel */ }
}
async function doShareSave() {
  try {
    const { value } = await ElMessageBox.prompt('分享链接（115/夸克）', '转存到当前目录')
    const pwd = (await ElMessageBox.prompt('提取码（无则留空）', '提取码').catch(() => ({ value: '' }))).value
    await cloudApi.fileOp('share-save', { drive_id: curDrive.value!.id, path: browsePath.value, url: value.trim(), pwd })
    ElMessage.success('转存已提交')
  } catch { /* cancel */ }
}
const repairing = ref(false)
async function doStrmRepair() {
  repairing.value = true
  try {
    const r = await cloudApi.strmRepair()
    ElMessage.success(`STRM 修复完成：重写 ${r.repaired} 个，跳过 ${r.skipped} 个`)
  } catch (e) {
    ElMessage.error((e as Error).message || '修复失败')
  } finally {
    repairing.value = false
  }
}
const organizing = ref(false)
async function doOrganize() {
  if (!curDrive.value) {
    ElMessage.warning('请先选择网盘')
    return
  }
  try {
    const r = await cloudApi.organize({
      drive_id: curDrive.value.id,
      src_dir: browsePath.value || '/',
      rename: true,
    })
    ElMessage.success(`云端整理完成：移动 ${r.moved ?? r.Moved ?? 0}，跳过 ${r.skipped ?? r.Skipped ?? 0}`)
  } catch (e) {
    ElMessage.error((e as Error).message || '整理失败')
  } finally {
    organizing.value = false
  }
}

// ---- 115 开放平台扫码授权 ----
const showAuth115 = ref(false)
const authAppId = ref('')
const authStarting = ref(false)
const authPolling = ref(false)
const authQR = ref('')
const authState = ref<{ uid: string; time: string; sign: string; verifier: string } | null>(null)
const authStatusText = ref('')

// 授权二维码：无前端 qrcode 依赖，用 115 下发的 qrcode URL（多为短链/参数串）。
// 可直接点击跳转手机端拉起；桌面端可配合浏览器二维码插件扫。
function qrImage(url: string): string {
  return `https://api.qrserver.com/v1/create-qr-code/?size=220x220&data=${encodeURIComponent(url)}`
}

function openAuth115() {
  authAppId.value = ''
  authQR.value = ''
  authState.value = null
  authStatusText.value = ''
  showAuth115.value = true
}

async function startAuth115() {
  if (!authAppId.value.trim()) {
    ElMessage.warning('请填 115 开放平台 AppID')
    return
  }
  authStarting.value = true
  try {
    const r = await cloudApi.start115Auth(authAppId.value.trim())
    authQR.value = r.qrcode
    authState.value = { uid: r.uid, time: r.time, sign: r.sign, verifier: r.verifier }
    authStatusText.value = '请用 115 App 扫码并确认'
    pollAuth115()
  } catch (e) {
    ElMessage.error((e as Error).message || '发起授权失败')
  } finally {
    authStarting.value = false
  }
}

async function pollAuth115() {
  if (!authState.value) return
  authPolling.value = true
  const max = 120
  for (let i = 0; i < max; i++) {
    try {
      const r = await cloudApi.poll115Auth(authState.value)
      if (r.confirmed && r.token) {
        authStatusText.value = '授权成功，正在创建网盘…'
        await cloudApi.createDrive({ name: `115开放·${new Date().toLocaleDateString()}`, type: '115_open', token: r.token, root_path: '/' })
        ElMessage.success('115 开放平台已授权并添加网盘')
        showAuth115.value = false
        await loadDrives()
        return
      }
      if (r.status === -2 || r.status === -1) {
        authStatusText.value = '二维码已过期，请重新发起'
        break
      }
      authStatusText.value = r.status === 2 ? '已扫码，请在手机上确认' : '等待扫码…'
    } catch {
      authStatusText.value = '轮询失败，请重试'
      break
    }
    await new Promise((res) => setTimeout(res, 2500))
  }
  authPolling.value = false
}

// ---- 云端任务队列 + 目录树导入 ----
const tasks = ref<CloudTask[]>([])
const taskStatusFilter = ref('')
const showImportTree = ref(false)
const importing = ref(false)
const treeForm = ref({ drive_id: 0, base: '', output_dir: '', tree: '' })

async function loadTasks() {
  try {
    tasks.value = await cloudApi.listTasks(taskStatusFilter.value)
  } catch { /* 静默 */ }
}
async function retryTask(t: CloudTask) {
  try {
    await cloudApi.retryTask(t.id)
    ElMessage.success('已重新入队')
    await loadTasks()
  } catch (e) { ElMessage.error((e as Error).message || '重试失败') }
}
async function removeTask(t: CloudTask) {
  try { await cloudApi.removeTask(t.id); await loadTasks() } catch { /* ignore */ }
}
async function doImportTree() {
  if (!treeForm.value.drive_id || !treeForm.value.output_dir.trim() || !treeForm.value.tree.trim()) {
    ElMessage.warning('请填目标网盘、输出目录与目录树文本')
    return
  }
  importing.value = true
  try {
    await cloudApi.importTree({
      drive_id: treeForm.value.drive_id,
      base: treeForm.value.base.trim(),
      output_dir: treeForm.value.output_dir.trim(),
      tree: treeForm.value.tree,
    })
    ElMessage.success('已入队导入任务（在任务队列查看进度）')
    showImportTree.value = false
    treeForm.value = { drive_id: 0, base: '', output_dir: '', tree: '' }
    activeTab.value = 'tasks'
    await loadTasks()
  } catch (e) {
    ElMessage.error((e as Error).message || '导入失败')
  } finally {
    importing.value = false
  }
}

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const activeTab = ref('drives')
const drives = ref<CloudDrive[]>([])
const loading = ref(false)

async function loadDrives() {
  loading.value = true
  try {
    drives.value = await cloudApi.listDrives()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载网盘失败')
  } finally {
    loading.value = false
  }
}

// ---- 网盘 CRUD ----
const showDriveForm = ref(false)
const editingDrive = ref<CloudDrive | null>(null)
const driveForm = ref<DriveInput>(defaultDrive())

function defaultDrive(): DriveInput {
  return { name: '', type: '115', token: '', root_path: '/', is_default: false }
}
function openCreateDrive() {
  editingDrive.value = null
  driveForm.value = defaultDrive()
  showDriveForm.value = true
}
function openEditDrive(d: CloudDrive) {
  editingDrive.value = d
  driveForm.value = { ...d, token: '' }
  showDriveForm.value = true
}
async function saveDrive() {
  if (!driveForm.value.name || !driveForm.value.type) {
    ElMessage.warning('名称、类型必填')
    return
  }
  try {
    if (editingDrive.value) {
      await cloudApi.updateDrive(editingDrive.value.id, driveForm.value)
      ElMessage.success('已更新')
    } else {
      if (!driveForm.value.token) {
        ElMessage.warning('新建需填写 Cookie / Token')
        return
      }
      await cloudApi.createDrive(driveForm.value)
      ElMessage.success('已创建')
    }
    showDriveForm.value = false
    await loadDrives()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}
async function removeDrive(d: CloudDrive) {
  await ElMessageBox.confirm(
    `确定删除网盘「${d.name}」？其下 STRM 规则与记录将一并清除。`,
    '提示',
    { type: 'warning' },
  )
  await cloudApi.removeDrive(d.id)
  ElMessage.success('已删除')
  await loadDrives()
}

// ---- 测试 / 配额 ----
const testing = ref<number | null>(null)
async function testDrive(d: CloudDrive) {
  testing.value = d.id
  try {
    const r = await cloudApi.test(d.id)
    r.ok ? ElMessage.success(`${d.name}：${r.info || '连接正常'}`) : ElMessage.warning(r.error || '连接失败')
  } catch (e: any) {
    ElMessage.error(e?.message || '请求失败')
  } finally {
    testing.value = null
  }
}
const quotas = ref<Record<number, { used: number; total: number }>>({})
async function loadQuota(d: CloudDrive) {
  try {
    quotas.value[d.id] = await cloudApi.quota(d.id)
  } catch (e: any) {
    ElMessage.error(e?.message || '获取容量失败')
  }
}

// ---- 浏览 + 规则（composable 封装） ----
const { showBrowse, browseDrive, browsePath, browseFiles, browseLoading, openBrowse, fetchBrowse, enterDir, browseUp, pickDir } = useCloudDriveBrowse()
const {
  rules, records, loadRules, loadRecords, driveName,
  showRuleForm, editingRule, ruleForm,
  openCreateRule, openEditRule, ruleBrowse, saveRule, removeRule,
  generating, generateOne, generateAll, syncSubs, removeRecord,
} = useCloudStrmRules(drives, openBrowse)

const tokenLabel = computed(() => {
  switch (driveForm.value.type) {
    case '123':
      return 'AccessToken（123开放平台 token）'
    case 'quark':
      return 'Cookie（夸克网页登录后的 cookie）'
    case 'aliyun_open':
      return 'AccessToken（阿里云盘开放平台 token）'
    case '115_open':
      return 'AccessToken（115开放平台授权后自动写入）'
    case 'onedrive':
      return 'AccessToken（MS Graph 访问令牌）'
    case 'baidu':
      return 'AccessToken（百度网盘开放平台授权 token）'
    case 'tianyi':
      return 'AccessToken（天翼云盘开放平台 token）'
    case 'mobile':
      return 'AccessToken（移动云盘开放平台 token）'
    default:
      return 'Cookie（115网页登录后的 cookie）'
  }
})

onMounted(async () => {
  await loadDrives()
  await Promise.all([loadRules(), loadRecords(), loadTasks()])
})
</script>

<template>
  <div class="page" :class="{ embedded: props.embedded }">
    <div class="head">
      <h2 v-if="!props.embedded" class="title">云盘 · STRM</h2>
      <h3 v-else class="title embedded-title">云盘账号 / STRM 规则</h3>
      <div class="head-actions">
        <el-button @click="syncSubs">订阅追更</el-button>
        <el-button @click="generateAll">生成全部 STRM</el-button>
        <el-button type="primary" @click="openCreateDrive">+ 添加网盘</el-button>
      </div>
    </div>
    <p v-if="!props.embedded" class="sub">
      115 / 夸克 / 123云盘原生接入：浏览云盘资源、按规则批量生成 .strm 直链文件供媒体服务器播放；
      订阅追更自动把已生成的 STRM 链接到媒体订阅并提醒。
    </p>
    <p v-else class="sub embedded-sub">
      账号与规则配置；文件浏览/STRM 生成请用「文件」页。
    </p>

    <el-tabs v-model="activeTab" class="tabs">
      <!-- 网盘 -->
      <el-tab-pane label="网盘" name="drives">
        <div v-loading="loading" class="grid">
          <div v-for="d in drives" :key="d.id" class="glass glass-hover card">
            <div class="card-head">
              <GlassIcon name="cloud" :size="34" from="#4cc9f0" to="#4361ee" />
              <div class="card-title">
                <div class="name">
                  {{ d.name }}
                  <el-tag size="small" effect="plain">{{ TYPE_LABEL[d.type] }}</el-tag>
                  <el-tag v-if="d.is_default" size="small" type="success">默认</el-tag>
                </div>
                <div class="meta">根目录：{{ d.root_path || '/' }}</div>
                <div v-if="quotas[d.id]" class="meta">
                  容量：{{ fmtSize(quotas[d.id].used) }} / {{ fmtSize(quotas[d.id].total) }}
                </div>
              </div>
            </div>
            <div class="actions">
              <el-button size="small" :loading="testing === d.id" @click="testDrive(d)">连接测试</el-button>
              <el-button size="small" @click="loadQuota(d)">查看容量</el-button>
              <el-button size="small" @click="openBrowse(d, () => {})">浏览</el-button>
              <el-button size="small" @click="openEditDrive(d)">编辑</el-button>
              <el-button size="small" type="danger" plain @click="removeDrive(d)">删除</el-button>
            </div>
          </div>
          <EmptyState
            v-if="!drives.length && !loading"
            icon="☁️"
            title="尚未添加网盘"
            description="添加 115 / 夸克 / 123 网盘以浏览资源并生成 STRM"
          >
            <el-button type="primary" @click="openCreateDrive">添加网盘</el-button>
            <el-button @click="openAuth115">扫码授权 115</el-button>
            <el-button @click="doOrganize" :loading="organizing">云端整理</el-button>
            <el-button @click="doStrmRepair" :loading="repairing">STRM 修复</el-button>
          </EmptyState>
        </div>
      </el-tab-pane>

      <!-- STRM 规则 -->
      <el-tab-pane label="STRM 规则" name="rules">
        <div class="pane-bar">
          <span class="hint">每条规则 = 一个网盘源目录 → 本地输出目录，扫描媒体文件生成 .strm</span>
          <el-button type="primary" size="small" @click="openCreateRule">+ 添加规则</el-button>
        </div>
        <el-table :data="rules" size="small" empty-text="尚未配置 STRM 规则">
          <el-table-column prop="name" label="名称" min-width="110" show-overflow-tooltip />
          <el-table-column label="网盘" width="110">
            <template #default="{ row }">{{ driveName((row as StrmRule).drive_id) }}</template>
          </el-table-column>
          <el-table-column label="云盘目录" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRule).cloud_path }}</template>
          </el-table-column>
          <el-table-column label="输出目录" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">
              {{ (row as StrmRule).output_tag ? `#${(row as StrmRule).output_tag}` : (row as StrmRule).output_dir }}
            </template>
          </el-table-column>
          <el-table-column label="挂载根" min-width="110" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRule).mount_root || '-' }}</template>
          </el-table-column>
          <el-table-column label="递归" width="60">
            <template #default="{ row }">{{ (row as StrmRule).recursive ? '是' : '否' }}</template>
          </el-table-column>
          <el-table-column label="启用" width="60">
            <template #default="{ row }">{{ (row as StrmRule).enabled ? '是' : '否' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="220">
            <template #default="{ row }">
              <el-button
                size="small"
                link
                type="primary"
                :loading="generating === (row as StrmRule).id"
                @click="generateOne(row as StrmRule)"
              >生成</el-button>
              <el-button size="small" link @click="openEditRule(row as StrmRule)">编辑</el-button>
              <el-button size="small" link type="danger" @click="removeRule(row as StrmRule)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- STRM 记录 -->
      <el-tab-pane label="STRM 记录" name="records">
        <el-table :data="records" size="small" empty-text="暂无已生成的 STRM 记录">
          <el-table-column label="网盘" width="110">
            <template #default="{ row }">{{ driveName((row as StrmRecord).drive_id) }}</template>
          </el-table-column>
          <el-table-column label="云盘文件" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRecord).cloud_path }}</template>
          </el-table-column>
          <el-table-column label="本地 STRM" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRecord).strm_path }}</template>
          </el-table-column>
          <el-table-column label="大小" width="90">
            <template #default="{ row }">{{ fmtSize((row as StrmRecord).size) }}</template>
          </el-table-column>
          <el-table-column label="来源" width="90">
            <template #default="{ row }">
              <el-tag v-if="(row as StrmRecord).subscription_id" size="small" type="success">追更</el-tag>
              <el-tag v-else size="small" effect="plain">规则</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="80">
            <template #default="{ row }">
              <el-button size="small" link type="danger" @click="removeRecord(row as StrmRecord)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 云端任务队列 -->
      <el-tab-pane label="任务队列" name="tasks">
        <div class="tab-toolbar">
          <el-select v-model="taskStatusFilter" size="small" style="width: 120px" @change="loadTasks">
            <el-option label="全部" value="" />
            <el-option label="排队中" value="pending" />
            <el-option label="执行中" value="running" />
            <el-option label="成功" value="success" />
            <el-option label="失败" value="failed" />
          </el-select>
          <el-button size="small" :icon="Refresh" @click="loadTasks">刷新</el-button>
          <el-button size="small" type="primary" @click="showImportTree = true">目录树导入</el-button>
        </div>
        <el-table :data="tasks" size="small" empty-text="暂无任务（整理自动上传 / 目录树导入会在此排队执行）">
          <el-table-column label="类型" width="100">
            <template #default="{ row }">{{ TASK_KIND_LABEL[(row as CloudTask).kind] || row.kind }}</template>
          </el-table-column>
          <el-table-column label="网盘" width="110">
            <template #default="{ row }">{{ driveName((row as CloudTask).drive_id) }}</template>
          </el-table-column>
          <el-table-column label="目标 / 源" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as CloudTask).cloud_path || (row as CloudTask).local_path }}</template>
          </el-table-column>
          <el-table-column label="进度" width="140">
            <template #default="{ row }">
              <el-progress :percentage="(row as CloudTask).progress || 0" :stroke-width="8"
                :status="(row as CloudTask).status === 'success' ? 'success' : (row as CloudTask).status === 'failed' ? 'exception' : undefined" />
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag size="small" :type="(row as CloudTask).status === 'success' ? 'success' : (row as CloudTask).status === 'failed' ? 'danger' : (row as CloudTask).status === 'running' ? 'warning' : 'info'">
                {{ TASK_STATUS_LABEL[(row as CloudTask).status] || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="错误" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as CloudTask).error || '-' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <el-button v-if="(row as CloudTask).status === 'failed'" size="small" link type="primary"
                @click="retryTask(row as CloudTask)">重试</el-button>
              <el-button size="small" link type="danger" @click="removeTask(row as CloudTask)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <!-- 115 开放平台扫码授权 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showAuth115" title="115 开放平台授权" width="480px">
      <el-form label-width="90px">
        <el-form-item label="AppID">
          <el-input v-model="authAppId" placeholder="115 开放平台应用的 client_id" :disabled="!!authState" />
        </el-form-item>
      </el-form>
      <div v-if="authQR" class="auth-qr">
        <img :src="qrImage(authQR)" alt="扫码授权" style="width: 220px; height: 220px" />
        <p class="hint">{{ authStatusText }}</p>
      </div>
      <template #footer>
        <el-button @click="showAuth115 = false">关闭</el-button>
        <el-button type="primary" :loading="authStarting || authPolling" :disabled="!!authState" @click="startAuth115">
          {{ authState ? '轮询中…' : '生成二维码' }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 115 目录树批量导入 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showImportTree" title="115 目录树批量生成 STRM" width="640px">
      <el-alert type="info" :closable="false" show-icon style="margin-bottom: 12px">
        在 115 网盘端导出目录树文本（每行一个路径），粘贴到下方。媒体文件行将批量生成 .strm 并登记记录，非媒体行自动跳过。
      </el-alert>
      <el-form label-width="110px">
        <el-form-item label="目标网盘">
          <el-select v-model="treeForm.drive_id" style="width: 100%">
            <el-option v-for="d in drives" :key="d.id" :label="`${d.name}（${TYPE_LABEL[d.type] || d.type}）`" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="目录树根">
          <el-input v-model="treeForm.base" placeholder="/媒体库（树路径相对的云盘根）" />
        </el-form-item>
        <el-form-item label="STRM 输出目录">
          <el-input v-model="treeForm.output_dir" placeholder="/media/strm" />
        </el-form-item>
        <el-form-item label="目录树文本">
          <el-input v-model="treeForm.tree" type="textarea" :rows="10" placeholder="电影/Inception (2010)/Inception.2010.1080p.mkv&#10;剧集/庆余年/S02E01.mp4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showImportTree = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="doImportTree">导入生成</el-button>
      </template>
    </el-dialog>

    <!-- 网盘表单 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showDriveForm" :title="editingDrive ? '编辑网盘' : '添加网盘'" width="560px">
      <el-form :model="driveForm" label-width="120px" label-position="right">
        <el-form-item label="名称">
          <el-input v-model="driveForm.name" placeholder="如：我的 115" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="driveForm.type">
            <el-option label="115网盘（开放平台）" value="115_open" />
            <el-option label="115网盘（Cookie）" value="115" />
            <el-option label="阿里云盘（开放平台）" value="aliyun_open" />
            <el-option label="夸克网盘" value="quark" />
            <el-option label="123云盘" value="123" />
            <el-option label="OneDrive" value="onedrive" />
            <el-option label="百度网盘" value="baidu" />
            <el-option label="天翼云盘" value="tianyi" />
            <el-option label="移动云盘" value="mobile" />
          </el-select>
        </el-form-item>
        <el-form-item label="Cookie / Token">
          <el-input
            v-model="driveForm.token"
            type="textarea"
            :rows="3"
            :placeholder="editingDrive ? '留空=保留原凭据' : tokenLabel"
          />
          <span class="hint">115_open/aliyun_open 填 accessToken；115(Cookie)/夸克复制整段 cookie；123 填 accessToken</span>
        </el-form-item>
        <el-form-item label="根目录">
          <el-input v-model="driveForm.root_path" placeholder="/（网盘根）" />
          <span class="hint">浏览/扫描默认起点</span>
        </el-form-item>
        <el-form-item label="设为默认"><el-switch v-model="driveForm.is_default" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDriveForm = false">取消</el-button>
        <el-button type="primary" @click="saveDrive">保存</el-button>
      </template>
    </el-dialog>

    <!-- 规则表单 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showRuleForm" :title="editingRule ? '编辑规则' : '添加规则'" width="600px">
      <el-form :model="ruleForm" label-width="110px" label-position="right">
        <el-form-item label="名称">
          <el-input v-model="ruleForm.name" placeholder="如：电影 STRM" />
        </el-form-item>
        <el-form-item label="网盘">
          <el-select v-model="ruleForm.drive_id" :disabled="!drives.length">
            <el-option v-for="d in drives" :key="d.id" :label="d.name" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="云盘目录">
          <div style="display: flex; gap: var(--sp-1); width: 100%">
            <el-input v-model="ruleForm.cloud_path" placeholder="/电影" style="flex: 1" />
            <el-button @click="ruleBrowse">浏览</el-button>
          </div>
        </el-form-item>
        <el-form-item label="输出目录">
          <el-input v-model="ruleForm.output_dir" placeholder="/strm/电影（本地 .strm 落地根）" />
        </el-form-item>
        <el-form-item label="目录标签">
          <el-input v-model="ruleForm.output_tag" placeholder="如 strm-movie（整理设置里定义的 local 型标签，优先于输出目录）" />
        </el-form-item>
        <el-form-item label="挂载根">
          <el-input v-model="ruleForm.mount_root" placeholder="如 /CloudDrive2/115（模板 {mount} 替换）" />
        </el-form-item>
        <el-form-item label="路径模板">
          <el-input v-model="ruleForm.template" placeholder="留空=默认 {mount}{cloudpath}" />
          <span class="hint">可用：{mount} {cloudpath} {relpath} {filename} {direct} {directPublic}</span>
        </el-form-item>
        <el-form-item label="内网地址">
          <el-input v-model="ruleForm.local_base_url" placeholder="如 http://192.168.1.10:8080（{direct} 优先用；空=全局 APP_BASE_URL）" />
        </el-form-item>
        <el-form-item label="外网地址">
          <el-input v-model="ruleForm.public_base_url" placeholder="如 https://ms.example.com（{directPublic} 用；空=同内网）" />
        </el-form-item>
        <el-form-item label="扩展名白名单">
          <el-input v-model="ruleForm.include_exts" placeholder="如 mkv,mp4（逗号分隔；空=全部媒体格式）" />
        </el-form-item>
        <el-form-item label="排除关键词">
          <el-input v-model="ruleForm.exclude_keywords" placeholder="如 sample,trailer,预告（逗号分隔；命中文件名即跳过）" />
        </el-form-item>
        <el-form-item label="递归扫描"><el-switch v-model="ruleForm.recursive" /></el-form-item>
        <el-form-item label="增量模式">
          <el-switch v-model="ruleForm.incremental" />
          <span class="hint">本地 .strm 已存在即跳过（大库定期生成省读盘）</span>
        </el-form-item>
        <el-form-item label="保留扩展名">
          <el-switch v-model="ruleForm.keep_ext" />
          <span class="hint">a.mkv→a.mkv.strm（关：a.strm）</span>
        </el-form-item>
        <el-form-item label="启用"><el-switch v-model="ruleForm.enabled" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showRuleForm = false">取消</el-button>
        <el-button type="primary" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>

    <!-- 浏览 -->
    <el-dialog class="pt-dialog" append-to-body align-center v-model="showBrowse" :title="`浏览 · ${browseDrive?.name ?? ''}`" width="640px">
      <div class="tab-toolbar" style="margin-bottom: 8px">
        <el-button size="small" @click="doMkdir">新建文件夹</el-button>
        <el-button size="small" @click="doOffline">离线下载</el-button>
        <el-button size="small" @click="doShareSave">分享转存</el-button>
      </div>
      <div class="browse-bar">
        <el-button size="small" :disabled="browsePath === '/'" @click="browseUp">↑ 上级</el-button>
        <span class="browse-path">{{ browsePath }}</span>
      </div>
      <div v-loading="browseLoading" class="browse-list">
        <div v-for="f in browseFiles.filter((x) => x.is_dir)" :key="f.path" class="browse-item" @click="enterDir(f)">
          📁 {{ f.name }}
        </div>
        <div v-for="f in browseFiles.filter((x) => !x.is_dir)" :key="f.path" class="browse-item file">
          🎬 {{ f.name }}
          <span class="meta">{{ fmtSize(f.size) }}</span>
        </div>
        <div v-if="!browseFiles.length && !browseLoading" class="meta" style="padding: 20px; text-align: center">
          （空目录）
        </div>
      </div>
      <template #footer>
        <el-button @click="showBrowse = false">取消</el-button>
        <el-button type="primary" @click="pickDir">使用此目录</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.page.embedded {
  padding: 0;
}
.page.embedded .embedded-title {
  font-size: var(--fs-lg);
  font-weight: 600;
  margin: 0;
}
.page.embedded .embedded-sub {
  margin-top: 0;
  margin-bottom: 12px;
  font-size: var(--fs-xs);
}
.page { display: flex; flex-direction: column; gap: var(--sp-3); }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-actions { display: flex; gap: var(--sp-2); }
.title { margin: 0; font-size: var(--fs-title); font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.tabs { margin-top: 4px; }
.pane-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: var(--sp-3); }
.card { padding: 16px; display: flex; flex-direction: column; gap: var(--sp-3); }
.card-head { display: flex; align-items: center; gap: var(--sp-3); }
.card-title { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: var(--sp-1); }
.name { font-weight: 600; font-size: var(--fs-body); display: flex; align-items: center; gap: var(--sp-1); flex-wrap: wrap; }
.meta { font-size: var(--fs-xs); color: var(--text-muted); word-break: break-all; }
.actions { display: flex; flex-wrap: wrap; gap: var(--sp-1); }
.hint { font-size: var(--fs-xs); color: var(--text-muted); display: block; margin-top: 4px; }
.browse-bar { display: flex; align-items: center; gap: var(--sp-2); margin-bottom: 10px; }
.browse-path { font-size: var(--fs-sm); color: var(--text-strong); word-break: break-all; }
.browse-list { display: flex; flex-direction: column; max-height: 50vh; overflow-y: auto; }
.browse-item { padding: 8px 10px; border-radius: var(--radius-xs); cursor: pointer; font-size: var(--fs-sm); display: flex; justify-content: space-between; align-items: center; transition: background var(--dur-fast) var(--ease-out-smooth), color var(--dur-fast) var(--ease-out-smooth), border-color var(--dur-fast) var(--ease-out-smooth) }
.browse-item:hover { background: var(--bg-elev, rgba(255, 255, 255, 0.06)); }
.browse-item.file { cursor: default; }
</style>
