// 云盘挂载 CRUD + 连接测试/配额查询子状态簇。
// 从 Cloud.vue 迁出;t 经注入保持 i18n 上下文。
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { cloudApi, type CloudDrive, type DriveInput } from '@/api/cloud'

function defaultDrive(): DriveInput {
  return { name: '', type: '115', token: '', root_path: '/', is_default: false }
}

export function useCloudDrives(t: (k: string, v?: Record<string, unknown>) => string) {
  const drives = ref<CloudDrive[]>([])
  const loading = ref(false)
  const showDriveForm = ref(false)
  const editingDrive = ref<CloudDrive | null>(null)
  const driveForm = ref<DriveInput>(defaultDrive())
  const testing = ref<number | null>(null)
  const quotas = ref<Record<number, { used: number; total: number }>>({})

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

  async function loadQuota(d: CloudDrive) {
    try {
      quotas.value[d.id] = await cloudApi.quota(d.id)
    } catch (e: any) {
      ElMessage.error(e?.message || t('cloud.quotaFailed'))
    }
  }

  return {
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
  }
}
