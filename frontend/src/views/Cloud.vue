<script setup lang="ts">
import { useI18n } from 'vue-i18n'
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
import CloudDialogs from '@/components/cloud/CloudDialogs.vue'
import { useCloud115Auth } from '@/composables/useCloud115Auth'

// ---- 云端文件管理器操作 ----
const { t } = useI18n()

const curDrive = computed(() => browseDrive.value)
async function fileOp(op: 'mkdir' | 'delete' | 'move' | 'copy' | 'rename', extra: Record<string, string> = {}) {
  if (!curDrive.value || !browsePath.value) {
    ElMessage.warning(t('cloud.pickDriveFirst'))
    return
  }
  try {
    if (op === 'delete') {
      await ElMessageBox.confirm(t('cloud.confirmDeletePath', { path: extra.path ?? '' }), t('common.delete'), {
        type: 'warning',
      })
    }
    await cloudApi.fileOp(op, { drive_id: curDrive.value.id, path: browsePath.value, ...extra })
    ElMessage.success(t('common.actionOk'))
    await fetchBrowse()
  } catch (e) {
    if ((e as string) !== 'cancel') ElMessage.error((e as Error).message || t('common.actionFailed'))
  }
}
async function doMkdir() {
  try {
    const { value } = await ElMessageBox.prompt(t('cloud.mkdirPrompt'), t('filemanager.newFolder'), {
      inputPattern: /^[^/\\]+$/,
      inputErrorMessage: t('cloud.invalidName'),
    })
    await fileOp('mkdir', { new_name: value.trim() })
  } catch {
    /* cancel */
  }
}
async function doOffline() {
  try {
    const { value } = await ElMessageBox.prompt(t('cloud.offlinePrompt'), t('cloud.offlineTitle'))
    await cloudApi.fileOp('offline', { drive_id: curDrive.value!.id, path: browsePath.value, url: value.trim() })
    ElMessage.success(t('cloud.offlineSubmitted'))
  } catch {
    /* cancel */
  }
}
async function doShareSave() {
  try {
    const { value } = await ElMessageBox.prompt(t('cloud.sharePrompt'), t('cloud.shareTitle'))
    const pwd = (await ElMessageBox.prompt(t('cloud.pwdPrompt'), t('cloud.pwdTitle')).catch(() => ({ value: '' })))
      .value
    await cloudApi.fileOp('share-save', {
      drive_id: curDrive.value!.id,
      path: browsePath.value,
      url: value.trim(),
      pwd,
    })
    ElMessage.success(t('cloud.shareSubmitted'))
  } catch {
    /* cancel */
  }
}
const repairing = ref(false)
async function doStrmRepair() {
  repairing.value = true
  try {
    const r = await cloudApi.strmRepair()
    ElMessage.success(t('cloud.strmRepaired', { repaired: r.repaired, skipped: r.skipped }))
  } catch (e) {
    ElMessage.error((e as Error).message || t('cloud.repairFailed'))
  } finally {
    repairing.value = false
  }
}
const organizing = ref(false)
async function doOrganize() {
  if (!curDrive.value) {
    ElMessage.warning(t('cloud.pickDriveOnly'))
    return
  }
  try {
    const r = await cloudApi.organize({
      drive_id: curDrive.value.id,
      src_dir: browsePath.value || '/',
      rename: true,
    })
    ElMessage.success(t('cloud.organizeDone', { moved: r.moved ?? r.Moved ?? 0, skipped: r.skipped ?? r.Skipped ?? 0 }))
  } catch (e) {
    ElMessage.error((e as Error).message || t('cloud.organizeFailed'))
  } finally {
    organizing.value = false
  }
}

// ---- 115 开放平台扫码授权（拆至 composables/useCloud115Auth.ts；授权成功回调刷新网盘）----
const {
  showAuth115,
  authAppId,
  authStarting,
  authPolling,
  authQR,
  authState,
  authStatusText,
  qrImage,
  openAuth115,
  startAuth115,
} = useCloud115Auth(loadDrives)

// ---- 云端任务队列 + 目录树导入 ----
const tasks = ref<CloudTask[]>([])
const taskStatusFilter = ref('')
const showImportTree = ref(false)
const importing = ref(false)
const treeForm = ref({ drive_id: 0, base: '', output_dir: '', tree: '' })

async function loadTasks() {
  try {
    tasks.value = await cloudApi.listTasks(taskStatusFilter.value)
  } catch {
    /* 静默 */
  }
}
async function retryTask(task: CloudTask) {
  try {
    await cloudApi.retryTask(task.id)
    ElMessage.success(t('cloud.requeued'))
    await loadTasks()
  } catch (e) {
    ElMessage.error((e as Error).message || t('cloud.retryFailed'))
  }
}
async function removeTask(task: CloudTask) {
  try {
    await cloudApi.removeTask(task.id)
    await loadTasks()
  } catch {
    /* ignore */
  }
}
async function doImportTree() {
  if (!treeForm.value.drive_id || !treeForm.value.output_dir.trim() || !treeForm.value.tree.trim()) {
    ElMessage.warning(t('cloud.treeFormRequired'))
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
    ElMessage.success(t('cloud.treeQueued'))
    showImportTree.value = false
    treeForm.value = { drive_id: 0, base: '', output_dir: '', tree: '' }
    activeTab.value = 'tasks'
    await loadTasks()
  } catch (e) {
    ElMessage.error((e as Error).message || t('cloud.importFailed'))
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
    ElMessage.error(e?.message || t('cloud.loadDrivesFailed'))
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
    ElMessage.warning(t('cloud.nameTypeRequired'))
    return
  }
  try {
    if (editingDrive.value) {
      await cloudApi.updateDrive(editingDrive.value.id, driveForm.value)
      ElMessage.success(t('common.updated'))
    } else {
      if (!driveForm.value.token) {
        ElMessage.warning(t('cloud.tokenRequired'))
        return
      }
      await cloudApi.createDrive(driveForm.value)
      ElMessage.success(t('common.created'))
    }
    showDriveForm.value = false
    await loadDrives()
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.saveFailed'))
  }
}
async function removeDrive(d: CloudDrive) {
  await ElMessageBox.confirm(t('cloud.confirmDeleteDrive', { name: d.name }), t('messages.tip'), { type: 'warning' })
  await cloudApi.removeDrive(d.id)
  ElMessage.success(t('common.deleted'))
  await loadDrives()
}

// ---- 测试 / 配额 ----
const testing = ref<number | null>(null)
async function testDrive(d: CloudDrive) {
  testing.value = d.id
  try {
    const r = await cloudApi.test(d.id)
    r.ok
      ? ElMessage.success(`${d.name}：${r.info || t('mediaserver.connOk')}`)
      : ElMessage.warning(r.error || t('mediaserver.connFailed'))
  } catch (e: any) {
    ElMessage.error(e?.message || t('common.requestFailed'))
  } finally {
    testing.value = null
  }
}
const quotas = ref<Record<number, { used: number; total: number }>>({})
async function loadQuota(d: CloudDrive) {
  try {
    quotas.value[d.id] = await cloudApi.quota(d.id)
  } catch (e: any) {
    ElMessage.error(e?.message || t('cloud.quotaFailed'))
  }
}

// ---- 浏览 + 规则（composable 封装） ----
const {
  showBrowse,
  browseDrive,
  browsePath,
  browseFiles,
  browseLoading,
  openBrowse,
  fetchBrowse,
  enterDir,
  browseUp,
  pickDir,
} = useCloudDriveBrowse()
const {
  rules,
  records,
  loadRules,
  loadRecords,
  driveName,
  showRuleForm,
  editingRule,
  ruleForm,
  openCreateRule,
  openEditRule,
  ruleBrowse,
  saveRule,
  removeRule,
  generating,
  generateOne,
  generateAll,
  syncSubs,
  removeRecord,
} = useCloudStrmRules(drives, openBrowse)

const tokenLabel = computed(() => {
  switch (driveForm.value.type) {
    case '123':
      return t('cloud.tokenLabel.123')
    case 'quark':
      return t('cloud.tokenLabel.quark')
    case 'aliyun_open':
      return t('cloud.tokenLabel.aliyunOpen')
    case '115_open':
      return t('cloud.tokenLabel.115Open')
    case 'onedrive':
      return t('cloud.tokenLabel.onedrive')
    case 'baidu':
      return t('cloud.tokenLabel.baidu')
    case 'tianyi':
      return t('cloud.tokenLabel.tianyi')
    case 'mobile':
      return t('cloud.tokenLabel.mobile')
    default:
      return t('cloud.tokenLabel.115Cookie')
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
      <h2 v-if="!props.embedded" class="title">{{ t('cloud.title') }}</h2>
      <h3 v-else class="title embedded-title">{{ t('cloud.titleEmbedded') }}</h3>
      <div class="head-actions">
        <el-button @click="syncSubs">{{ t('cloud.syncSubs') }}</el-button>
        <el-button @click="generateAll">{{ t('cloud.generateAll') }}</el-button>
        <el-button type="primary" @click="openCreateDrive">{{ t('cloud.addDrive') }}</el-button>
      </div>
    </div>
    <p v-if="!props.embedded" class="sub">{{ t('cloud.sub') }}</p>
    <p v-else class="sub embedded-sub">{{ t('cloud.subEmbedded') }}</p>

    <el-tabs v-model="activeTab" class="tabs">
      <!-- 网盘 -->
      <el-tab-pane :label="t('cloud.tabDrives')" name="drives">
        <div v-loading="loading" class="grid">
          <div v-for="d in drives" :key="d.id" class="glass glass-hover card">
            <div class="card-head">
              <GlassIcon name="cloud" :size="34" from="#4cc9f0" to="#4361ee" />
              <div class="card-title">
                <div class="name">
                  {{ d.name }}
                  <el-tag size="small" effect="plain">{{ TYPE_LABEL[d.type] }}</el-tag>
                  <el-tag v-if="d.is_default" size="small" type="success">{{ t('cloud.defaultTag') }}</el-tag>
                </div>
                <div class="meta">{{ t('cloud.rootPath') }}：{{ d.root_path || '/' }}</div>
                <div v-if="quotas[d.id]" class="meta">
                  {{ t('cloud.quota') }}：{{ fmtSize(quotas[d.id].used) }} / {{ fmtSize(quotas[d.id].total) }}
                </div>
              </div>
            </div>
            <div class="actions">
              <el-button size="small" :loading="testing === d.id" @click="testDrive(d)">{{
                t('cloud.testConn')
              }}</el-button>
              <el-button size="small" @click="loadQuota(d)">{{ t('cloud.viewQuota') }}</el-button>
              <el-button size="small" @click="openBrowse(d, () => {})">{{ t('cloud.browse') }}</el-button>
              <el-button size="small" @click="openEditDrive(d)">{{ t('common.edit') }}</el-button>
              <el-button size="small" type="danger" plain @click="removeDrive(d)">{{ t('common.delete') }}</el-button>
            </div>
          </div>
          <EmptyState
            v-if="!drives.length && !loading"
            icon="☁️"
            :title="t('cloud.emptyTitle')"
            :description="t('cloud.emptyDesc')"
          >
            <el-button type="primary" @click="openCreateDrive">{{ t('cloud.addDrive2') }}</el-button>
            <el-button @click="openAuth115">{{ t('cloud.qrAuth115') }}</el-button>
            <el-button @click="doOrganize" :loading="organizing">{{ t('cloud.organize') }}</el-button>
            <el-button @click="doStrmRepair" :loading="repairing">{{ t('cloud.strmRepair') }}</el-button>
          </EmptyState>
        </div>
      </el-tab-pane>

      <!-- STRM 规则 -->
      <el-tab-pane :label="t('cloud.tabRules')" name="rules">
        <div class="pane-bar">
          <span class="hint">{{ t('cloud.rulesHint') }}</span>
          <el-button type="primary" size="small" @click="openCreateRule">{{ t('cloud.addRule') }}</el-button>
        </div>
        <el-table :data="rules" size="small" :empty-text="t('cloud.rulesEmpty')">
          <el-table-column prop="name" :label="t('common.name')" min-width="110" show-overflow-tooltip />
          <el-table-column :label="t('cloud.drive')" width="110">
            <template #default="{ row }">{{ driveName((row as StrmRule).drive_id) }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.cloudDir')" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRule).cloud_path }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.outputDir')" min-width="130" show-overflow-tooltip>
            <template #default="{ row }">
              {{ (row as StrmRule).output_tag ? `#${(row as StrmRule).output_tag}` : (row as StrmRule).output_dir }}
            </template>
          </el-table-column>
          <el-table-column :label="t('cloud.mountRoot')" min-width="110" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRule).mount_root || '-' }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.recursive')" width="60">
            <template #default="{ row }">{{ (row as StrmRule).recursive ? t('common.yes') : t('common.no') }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.enabled')" width="60">
            <template #default="{ row }">{{ (row as StrmRule).enabled ? t('common.yes') : t('common.no') }}</template>
          </el-table-column>
          <el-table-column :label="t('common.action')" width="220">
            <template #default="{ row }">
              <el-button
                size="small"
                link
                type="primary"
                :loading="generating === (row as StrmRule).id"
                @click="generateOne(row as StrmRule)"
                >{{ t('cloud.generate') }}</el-button
              >
              <el-button size="small" link @click="openEditRule(row as StrmRule)">{{ t('common.edit') }}</el-button>
              <el-button size="small" link type="danger" @click="removeRule(row as StrmRule)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- STRM 记录 -->
      <el-tab-pane :label="t('cloud.tabRecords')" name="records">
        <el-table :data="records" size="small" :empty-text="t('cloud.recordsEmpty')">
          <el-table-column :label="t('cloud.drive')" width="110">
            <template #default="{ row }">{{ driveName((row as StrmRecord).drive_id) }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.cloudFile')" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRecord).cloud_path }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.localStrm')" min-width="200" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as StrmRecord).strm_path }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.size')" width="90">
            <template #default="{ row }">{{ fmtSize((row as StrmRecord).size) }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.source')" width="90">
            <template #default="{ row }">
              <el-tag v-if="(row as StrmRecord).subscription_id" size="small" type="success">{{
                t('cloud.srcSub')
              }}</el-tag>
              <el-tag v-else size="small" effect="plain">{{ t('cloud.srcRule') }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('common.action')" width="80">
            <template #default="{ row }">
              <el-button size="small" link type="danger" @click="removeRecord(row as StrmRecord)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 云端任务队列 -->
      <el-tab-pane :label="t('cloud.tabTasks')" name="tasks">
        <div class="tab-toolbar">
          <el-select v-model="taskStatusFilter" size="small" style="width: 120px" @change="loadTasks">
            <el-option :label="t('cloud.all')" value="" />
            <el-option :label="t('cloud.stPending')" value="pending" />
            <el-option :label="t('cloud.stRunning')" value="running" />
            <el-option :label="t('cloud.stSuccess')" value="success" />
            <el-option :label="t('cloud.stFailed')" value="failed" />
          </el-select>
          <el-button size="small" :icon="Refresh" @click="loadTasks">{{ t('common.refresh') }}</el-button>
          <el-button size="small" type="primary" @click="showImportTree = true">{{ t('cloud.importTree') }}</el-button>
        </div>
        <el-table :data="tasks" size="small" :empty-text="t('cloud.tasksEmpty')">
          <el-table-column :label="t('cloud.kind')" width="100">
            <template #default="{ row }">{{ TASK_KIND_LABEL[(row as CloudTask).kind] || row.kind }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.drive')" width="110">
            <template #default="{ row }">{{ driveName((row as CloudTask).drive_id) }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.targetSrc')" min-width="220" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as CloudTask).cloud_path || (row as CloudTask).local_path }}</template>
          </el-table-column>
          <el-table-column :label="t('cloud.progress')" width="140">
            <template #default="{ row }">
              <el-progress
                :percentage="(row as CloudTask).progress || 0"
                :stroke-width="8"
                :status="
                  (row as CloudTask).status === 'success'
                    ? 'success'
                    : (row as CloudTask).status === 'failed'
                      ? 'exception'
                      : undefined
                "
              />
            </template>
          </el-table-column>
          <el-table-column :label="t('cloud.status')" width="90">
            <template #default="{ row }">
              <el-tag
                size="small"
                :type="
                  (row as CloudTask).status === 'success'
                    ? 'success'
                    : (row as CloudTask).status === 'failed'
                      ? 'danger'
                      : (row as CloudTask).status === 'running'
                        ? 'warning'
                        : 'info'
                "
              >
                {{ TASK_STATUS_LABEL[(row as CloudTask).status] || row.status }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column :label="t('cloud.error')" min-width="160" show-overflow-tooltip>
            <template #default="{ row }">{{ (row as CloudTask).error || '-' }}</template>
          </el-table-column>
          <el-table-column :label="t('common.action')" width="130" fixed="right">
            <template #default="{ row }">
              <el-button
                v-if="(row as CloudTask).status === 'failed'"
                size="small"
                link
                type="primary"
                @click="retryTask(row as CloudTask)"
                >{{ t('common.retry') }}</el-button
              >
              <el-button size="small" link type="danger" @click="removeTask(row as CloudTask)">{{
                t('common.delete')
              }}</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>

    <CloudDialogs
      v-model:show-auth-115="showAuth115"
      v-model:show-import-tree="showImportTree"
      v-model:show-drive-form="showDriveForm"
      v-model:show-rule-form="showRuleForm"
      v-model:show-browse="showBrowse"
      v-model:auth-app-id="authAppId"
      v-model:tree-form="treeForm"
      v-model:drive-form="driveForm"
      v-model:rule-form="ruleForm"
      :auth-state="authState ?? ''"
      :auth-q-r="authQR"
      :auth-status-text="authStatusText"
      :auth-starting="authStarting"
      :auth-polling="authPolling"
      :drives="drives"
      :browse-files="browseFiles"
      :browse-path="browsePath"
      :browse-loading="browseLoading"
      :qr-image="qrImage"
      :editing-drive="editingDrive"
      :editing-rule="editingRule"
      :browse-drive="browseDrive"
      :token-label="tokenLabel"
      :importing="importing"
      :loading="loading"
      :fmt-size="fmtSize"
      @start-auth="startAuth115"
      @do-import-tree="doImportTree"
      @save-drive="saveDrive"
      @save-rule="saveRule"
      @browse-up="browseUp"
      @enter-dir="enterDir"
      @pick-dir="pickDir"
      @rule-browse="ruleBrowse"
      @do-mkdir="doMkdir"
      @do-offline="doOffline"
      @do-share-save="doShareSave"
    />
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
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.head-actions {
  display: flex;
  gap: var(--sp-2);
}
.title {
  margin: 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.sub {
  margin: -8px 0 0;
  font-size: var(--fs-sm);
  color: var(--text-muted);
}
.tabs {
  margin-top: 4px;
}
.pane-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--sp-3);
}
.card {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.card-head {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
}
.card-title {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.name {
  font-weight: 600;
  font-size: var(--fs-body);
  display: flex;
  align-items: center;
  gap: var(--sp-1);
  flex-wrap: wrap;
}
.meta {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  word-break: break-all;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
}
.hint {
  font-size: var(--fs-xs);
  color: var(--text-muted);
  display: block;
  margin-top: 4px;
}
.browse-bar {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  margin-bottom: 10px;
}
.browse-path {
  font-size: var(--fs-sm);
  color: var(--text-strong);
  word-break: break-all;
}
.browse-list {
  display: flex;
  flex-direction: column;
  max-height: 50vh;
  overflow-y: auto;
}
.browse-item {
  padding: 8px 10px;
  border-radius: var(--radius-xs);
  cursor: pointer;
  font-size: var(--fs-sm);
  display: flex;
  justify-content: space-between;
  align-items: center;
  transition:
    background var(--dur-fast) var(--ease-out-smooth),
    color var(--dur-fast) var(--ease-out-smooth),
    border-color var(--dur-fast) var(--ease-out-smooth);
}
.browse-item:hover {
  background: var(--bg-elev, rgba(255, 255, 255, 0.06));
}
.browse-item.file {
  cursor: default;
}
</style>
