<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
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
import CloudRulesTab from '@/components/cloud/CloudRulesTab.vue'
import { useCloud115Auth } from '@/composables/useCloud115Auth'
import { useCloudDrives } from './useCloudDrives'

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
} = useCloud115Auth(() => loadDrives())

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

// 有在途任务（排队中/执行中）时每 4s 自动轮询，全部终态或离开页面即停。
const hasActiveTask = computed(() => tasks.value.some((t) => t.status === 'pending' || t.status === 'running'))
let taskTimer: number | undefined
function syncTaskPolling() {
  if (hasActiveTask.value && taskTimer === undefined) {
    taskTimer = window.setInterval(() => {
      loadTasks()
    }, 4000)
  } else if (!hasActiveTask.value && taskTimer !== undefined) {
    window.clearInterval(taskTimer)
    taskTimer = undefined
  }
}
watch(hasActiveTask, syncTaskPolling)
onUnmounted(() => {
  if (taskTimer !== undefined) window.clearInterval(taskTimer)
})
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
  // 删除任务记录前确认（W1：此前无确认直接删，误触即丢历史记录）。
  try {
    await ElMessageBox.confirm(t('cloud.confirmRemoveTask', { id: task.id }), t('common.delete'), {
      type: 'warning',
      confirmButtonText: t('common.delete'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return
  }
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

// 网盘 CRUD/测试/配额拆出 useCloudDrives
const {
  drives,
  loading,
  showDriveForm,
  editingDrive,
  driveForm,
  testing,
  quotas,
  loadDrives,
  openCreateDrive,
  openEditDrive,
  saveDrive,
  removeDrive,
  testDrive,
  loadQuota,
} = useCloudDrives((k: string, v?: Record<string, unknown>) => t(k, v ?? {}))

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
      <CloudDrivesTab :drives="drives" mode="cloud" :testing="testing" :organizing="organizing" :repairing="repairing" :loading="loading" :quotas="quotas" :fmt-size="(n?: number) => fmtSize(n ?? 0)" @generate="(r: any) => generateOne(r)" @test="testDrive" @quota="loadQuota" @browse="(d: any) => openBrowse(d, () => {})" @create="openCreateDrive" @edit="openEditDrive" @remove="removeDrive" @auth="openAuth115" @organize="doOrganize" @repair="doStrmRepair" />
      <CloudRulesTab
        :rules="rules"
        :generating="generating"
        :drive-name="driveName"
        @create="openCreateRule"
        @generate="generateOne"
        @edit="openEditRule"
        @remove="removeRule"
      />
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
