// Docker 运维页对话框/抽屉的可见性与内容状态 + 日志拉取/镜像删除动作。
// 从 OpsDocker.vue 迁出;mode/selectedTarget/refresh/t 经 getter 注入保持响应式。
import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, type DockerContainer, type DockerImage } from '@/api/docker'
import type { DockerTemplate } from '@/data/dockerTemplates'
import type { ComposeTemplate } from '@/data/composeTemplates'

export function useDockerDialogsState(opts: {
  mode: () => 'local' | 'remote'
  selectedTarget: () => number
  refresh: () => void
  actionLoading: { value: boolean }
  t: (k: string, v?: Record<string, unknown>) => string
}) {
  const { mode, selectedTarget, refresh, actionLoading, t } = opts
// 日志抽屉
const logDrawer = ref(false)
const logContainer = ref('')
const logContent = ref('')
const logLoading = ref(false)
// 详情抽屉
const detailVisible = ref(false)
const detailContainer = ref('')
// 创建向导
const createDialog = ref(false)
// 应用商店
const appStoreVisible = ref(false)
// 从应用商店选中模板后传入向导
const pendingTemplate = ref<DockerTemplate | null>(null)
// 从应用商店选中 Compose 模板后传入编辑器
const pendingCompose = ref<ComposeTemplate | null>(null)
// exec 终端
const execDrawer = ref(false)
const execContainer = ref('')

async function removeImage(img: DockerImage) {
  const imageRef = img.Repository + ':' + img.Tag
  try {
    await ElMessageBox.confirm(t('opsdocker.removeImageConfirm', { ref: imageRef }), t('opsdisk.confirmPartTitle'), {
      type: 'warning',
    })
    actionLoading.value = true
    if (mode() === 'local') {
      await dockerApi.localRemoveImage(imageRef, true)
    } else {
      await dockerApi.removeImage(selectedTarget(), imageRef, true)
    }
    ElMessage.success(t('opsdocker.imageRemoved'))
    refresh()
  } catch (e: any) {
    if (e !== 'cancel') ElMessage.error(e.message)
  } finally {
    actionLoading.value = false
  }
}

async function showLogs(container: DockerContainer) {
  logContainer.value = container.Names.replace(/^\//, '')
  logDrawer.value = true
  logLoading.value = true
  try {
    const res =
      mode() === 'local'
        ? await dockerApi.localLogs(logContainer.value, 200)
        : await dockerApi.logs(selectedTarget(), logContainer.value, 200)
    logContent.value = res.logs || '(' + t('opsdocker.noLogs') + ')'
  } catch (e: any) {
    logContent.value = t('opsdocker.logFailed') + ': ' + e.message
  } finally {
    logLoading.value = false
  }
}

function showDetail(container: DockerContainer) {
  detailContainer.value = container.Names.replace(/^\//, '')
  detailVisible.value = true
}

function openExec(c: DockerContainer) {
  execContainer.value = c.Names.replace(/^\//, '')
  execDrawer.value = true
}

  return {
    logDrawer,
    logContainer,
    logContent,
    logLoading,
    detailVisible,
    detailContainer,
    createDialog,
    appStoreVisible,
    pendingTemplate,
    pendingCompose,
    execDrawer,
    execContainer,
    removeImage,
    showLogs,
    showDetail,
    openExec,
  }
}
