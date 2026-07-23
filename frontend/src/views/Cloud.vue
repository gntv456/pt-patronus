<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })
import {
  cloudApi,
  TYPE_LABEL,
  type CloudDrive,
  type CloudDriveType,
  type CloudFile,
  type StrmRule,
  type StrmRecord,
  type StrmResult,
  type DriveInput,
} from '@/api/cloud'
import GlassIcon from '@/components/GlassIcon.vue'
import EmptyState from '@/components/EmptyState.vue'
import { ElMessageBox, ElMessage } from 'element-plus'

const activeTab = ref('drives')
const drives = ref<CloudDrive[]>([])
const rules = ref<StrmRule[]>([])
const records = ref<StrmRecord[]>([])
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
async function loadRules() {
  try {
    rules.value = await cloudApi.listRules()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载规则失败')
  }
}
async function loadRecords() {
  try {
    records.value = await cloudApi.listRecords()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载记录失败')
  }
}

const driveName = (id: number) => drives.value.find((d) => d.id === id)?.name ?? `#${id}`

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

// ---- 测试 / 配额 / 浏览 ----
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

// 浏览对话框（选目录填入规则 cloud_path）
const showBrowse = ref(false)
const browseDrive = ref<CloudDrive | null>(null)
const browsePath = ref('/')
const browseFiles = ref<CloudFile[]>([])
const browseLoading = ref(false)
const browseTarget = ref<((p: string) => void) | null>(null)

async function openBrowse(d: CloudDrive, target: (p: string) => void) {
  browseDrive.value = d
  browseTarget.value = target
  browsePath.value = d.root_path || '/'
  showBrowse.value = true
  await fetchBrowse()
}
async function fetchBrowse() {
  if (!browseDrive.value) return
  browseLoading.value = true
  try {
    browseFiles.value = await cloudApi.browse(browseDrive.value.id, browsePath.value)
  } catch (e: any) {
    ElMessage.error(e?.message || '浏览失败')
    browseFiles.value = []
  } finally {
    browseLoading.value = false
  }
}
function enterDir(f: CloudFile) {
  if (!f.is_dir) return
  browsePath.value = f.path
  fetchBrowse()
}
function browseUp() {
  if (browsePath.value === '/') return
  const parts = browsePath.value.split('/').filter(Boolean)
  parts.pop()
  browsePath.value = '/' + parts.join('/')
  fetchBrowse()
}
function pickDir() {
  if (browseTarget.value) browseTarget.value(browsePath.value)
  showBrowse.value = false
}

// ---- STRM 规则 CRUD ----
const showRuleForm = ref(false)
const editingRule = ref<StrmRule | null>(null)
const ruleForm = ref(defaultRule())
function defaultRule() {
  return {
    name: '',
    drive_id: 0,
    cloud_path: '/',
    output_dir: '',
    mount_root: '',
    template: '',
    recursive: true,
    keep_ext: true,
    enabled: true,
  }
}
function openCreateRule() {
  editingRule.value = null
  ruleForm.value = { ...defaultRule(), drive_id: drives.value[0]?.id ?? 0 }
  showRuleForm.value = true
}
function openEditRule(r: StrmRule) {
  editingRule.value = r
  ruleForm.value = { ...r }
  showRuleForm.value = true
}
function ruleBrowse() {
  const d = drives.value.find((x) => x.id === ruleForm.value.drive_id)
  if (!d) {
    ElMessage.warning('请先选择网盘')
    return
  }
  openBrowse(d, (p) => (ruleForm.value.cloud_path = p))
}
async function saveRule() {
  const f = ruleForm.value
  if (!f.name || !f.drive_id || !f.cloud_path || !f.output_dir) {
    ElMessage.warning('名称/网盘/云盘目录/输出目录必填')
    return
  }
  try {
    if (editingRule.value) {
      await cloudApi.updateRule(editingRule.value.id, f)
      ElMessage.success('已更新')
    } else {
      await cloudApi.createRule(f)
      ElMessage.success('已创建')
    }
    showRuleForm.value = false
    await loadRules()
  } catch (e: any) {
    ElMessage.error(e?.message || '保存失败')
  }
}
async function removeRule(r: StrmRule) {
  await ElMessageBox.confirm(`确定删除规则「${r.name}」？`, '提示', { type: 'warning' })
  await cloudApi.removeRule(r.id)
  ElMessage.success('已删除')
  await loadRules()
}

// ---- 生成 ----
const generating = ref<number | null>(null)
function report(res: StrmResult, prefix: string) {
  if (res.errors?.length) {
    ElMessage.warning(`${prefix}：新增 ${res.generated}，跳过 ${res.skipped}，失败 ${res.errors.length}`)
  } else {
    ElMessage.success(`${prefix}：新增 ${res.generated}，跳过 ${res.skipped}`)
  }
}
async function generateOne(r: StrmRule) {
  generating.value = r.id
  try {
    const res = await cloudApi.generateRule(r.id)
    report(res, r.name)
    await loadRecords()
  } catch (e: any) {
    ElMessage.error(e?.message || '生成失败')
  } finally {
    generating.value = null
  }
}
async function generateAll() {
  try {
    await cloudApi.generateAll()
    ElMessage.success('已触发全部规则生成')
    setTimeout(loadRecords, 1500)
  } catch (e: any) {
    ElMessage.error(e?.message || '请求失败')
  }
}
async function syncSubs() {
  try {
    await cloudApi.syncSubscriptions()
    ElMessage.success('订阅追更匹配完成')
  } catch (e: any) {
    ElMessage.error(e?.message || '请求失败')
  }
}
async function removeRecord(r: StrmRecord) {
  await cloudApi.removeRecord(r.id)
  await loadRecords()
}

const tokenLabel = computed(() => {
  switch (driveForm.value.type) {
    case '123':
      return 'AccessToken（123开放平台 token）'
    case 'quark':
      return 'Cookie（夸克网页登录后的 cookie）'
    default:
      return 'Cookie（115网页登录后的 cookie）'
  }
})

function fmtSize(n: number) {
  if (!n) return '-'
  const g = 1 << 30
  if (n >= g) return (n / g).toFixed(1) + ' GB'
  if (n >= 1 << 20) return (n / (1 << 20)).toFixed(1) + ' MB'
  return (n / (1 << 10)).toFixed(1) + ' KB'
}

onMounted(async () => {
  await loadDrives()
  await Promise.all([loadRules(), loadRecords()])
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
            <template #default="{ row }">{{ (row as StrmRule).output_dir }}</template>
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
    </el-tabs>

    <!-- 网盘表单 -->
    <el-dialog v-model="showDriveForm" :title="editingDrive ? '编辑网盘' : '添加网盘'" width="560px" top="8vh">
      <el-form :model="driveForm" label-width="120px" label-position="right">
        <el-form-item label="名称">
          <el-input v-model="driveForm.name" placeholder="如：我的 115" />
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="driveForm.type">
            <el-option label="115网盘" value="115" />
            <el-option label="夸克网盘" value="quark" />
            <el-option label="123云盘" value="123" />
          </el-select>
        </el-form-item>
        <el-form-item label="Cookie / Token">
          <el-input
            v-model="driveForm.token"
            type="textarea"
            :rows="3"
            :placeholder="editingDrive ? '留空=保留原凭据' : tokenLabel"
          />
          <span class="hint">从浏览器登录后复制（115/夸克复制整段 cookie，123 填 accessToken）</span>
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
    <el-dialog v-model="showRuleForm" :title="editingRule ? '编辑规则' : '添加规则'" width="600px" top="8vh">
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
          <div style="display: flex; gap: 6px; width: 100%">
            <el-input v-model="ruleForm.cloud_path" placeholder="/电影" style="flex: 1" />
            <el-button @click="ruleBrowse">浏览</el-button>
          </div>
        </el-form-item>
        <el-form-item label="输出目录">
          <el-input v-model="ruleForm.output_dir" placeholder="/strm/电影（本地 .strm 落地根）" />
        </el-form-item>
        <el-form-item label="挂载根">
          <el-input v-model="ruleForm.mount_root" placeholder="如 /CloudDrive2/115（模板 {mount} 替换）" />
        </el-form-item>
        <el-form-item label="路径模板">
          <el-input v-model="ruleForm.template" placeholder="留空=默认 {mount}{cloudpath}" />
          <span class="hint">可用：{mount} {cloudpath} {relpath} {filename}</span>
        </el-form-item>
        <el-form-item label="递归扫描"><el-switch v-model="ruleForm.recursive" /></el-form-item>
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
    <el-dialog v-model="showBrowse" :title="`浏览 · ${browseDrive?.name ?? ''}`" width="640px" top="8vh">
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
  font-size: 15px;
  font-weight: 600;
  margin: 0;
}
.page.embedded .embedded-sub {
  margin-top: 0;
  margin-bottom: 12px;
  font-size: 12px;
}
.page { display: flex; flex-direction: column; gap: 14px; }
.head { display: flex; align-items: center; justify-content: space-between; }
.head-actions { display: flex; gap: 8px; }
.title { margin: 0; font-size: 22px; font-weight: 700; }
.sub { margin: -8px 0 0; font-size: var(--fs-sm); color: var(--text-muted); }
.tabs { margin-top: 4px; }
.pane-bar { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 14px; }
.card { padding: 16px; display: flex; flex-direction: column; gap: 12px; }
.card-head { display: flex; align-items: center; gap: 12px; }
.card-title { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
.name { font-weight: 600; font-size: var(--fs-body); display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.meta { font-size: var(--fs-xs); color: var(--text-muted); word-break: break-all; }
.actions { display: flex; flex-wrap: wrap; gap: 6px; }
.hint { font-size: var(--fs-xs); color: var(--text-muted); display: block; margin-top: 4px; }
.browse-bar { display: flex; align-items: center; gap: 10px; margin-bottom: 10px; }
.browse-path { font-size: var(--fs-sm); color: var(--text-strong); word-break: break-all; }
.browse-list { display: flex; flex-direction: column; max-height: 50vh; overflow-y: auto; }
.browse-item { padding: 8px 10px; border-radius: 6px; cursor: pointer; font-size: var(--fs-sm); display: flex; justify-content: space-between; align-items: center; }
.browse-item:hover { background: var(--bg-elev, rgba(255, 255, 255, 0.06)); }
.browse-item.file { cursor: default; }
</style>
