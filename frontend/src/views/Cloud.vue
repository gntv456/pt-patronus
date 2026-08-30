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
import CloudDrivesTab from '@/components/cloud/CloudDrivesTab.vue'
import CloudRecordsTab from '@/components/cloud/CloudRecordsTab.vue'
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
      <CloudDrivesTab :drives="drives" mode="cloud" :testing="testing" :organizing="organizing" :repairing="repairing" :loading="loading" :quotas="quotas" @generate="(r: any) => generateOne(r)" @test="testDrive" @quota="loadQuota" @browse="(d: any) => openBrowse(d, () => {})" @create="openCreateDrive" @edit="openEditDrive" @remove="removeDrive" @auth="openAuth115" @organize="doOrganize" @repair="doStrmRepair" />
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

      <CloudRecordsTab :records="records" :tasks="tasks" :loading="loading" :drive-name="driveName" :fmt-size="(n?: number) => fmtSize(n ?? 0)" v-model:task-status-filter="taskStatusFilter" @remove-record="removeRecord" @retry-task="retryTask" @remove-task="removeTask" @load-tasks="loadTasks"
      @show-import-tree="showImportTree = true" />
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
      :fmt-size="(n?: number) => fmtSize(n ?? 0)"
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

<style scoped src="./Cloud.css"></style>
