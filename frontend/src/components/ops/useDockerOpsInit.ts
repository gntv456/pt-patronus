// Docker 运维页初始化与模式切换：应用商店模板选中、docker.sock 提示复制、
// 首次挂载探测本机可用性并启动轮询、模式切换时重置历史并重启统计。
// 从 OpsDocker.vue 迁出。
import { onMounted, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import { dockerApi } from '@/api/docker'
import type { DockerTemplate } from '@/data/dockerTemplates'
import type { ComposeTemplate } from '@/data/composeTemplates'

export function useDockerOpsInit(opts: {
  mode: Ref<'local' | 'remote'>
  selectedTarget: Ref<number>
  localAvailable: Ref<boolean>
  pendingTemplate: Ref<DockerTemplate | null>
  pendingCompose: Ref<ComposeTemplate | null>
  createDialog: Ref<boolean>
  appStoreVisible: Ref<boolean>
  localComposeVisible: Ref<boolean>
  loadTargets: () => Promise<void>
  refresh: () => void
  startStatsPolling: () => void
  resetHistory: () => void
  t: (k: string) => string
}) {
  function openAppStore() {
    opts.pendingTemplate.value = null
    opts.pendingCompose.value = null
    opts.appStoreVisible.value = true
  }

  function onStoreComposeSelected(tpl: ComposeTemplate) {
    opts.pendingCompose.value = tpl
    opts.localComposeVisible.value = true
  }

  function onStoreTemplateSelected(tpl: DockerTemplate) {
    opts.pendingTemplate.value = tpl
    opts.createDialog.value = true
  }

  function copyDockerSockHint() {
    navigator.clipboard
      .writeText('- /var/run/docker.sock:/var/run/docker.sock')
      .then(() => {
        ElMessage.success(opts.t('opsdocker.copied'))
      })
      .catch(() => {})
  }

  onMounted(async () => {
    await opts.loadTargets()
    try {
      const st = await dockerApi.localStatus()
      opts.localAvailable.value = st.available
    } catch {
      opts.localAvailable.value = false
    }
    if (opts.localAvailable.value) {
      opts.mode.value = 'local'
      opts.refresh()
      opts.startStatsPolling()
    } else if (opts.selectedTarget.value) {
      opts.mode.value = 'remote'
      opts.refresh()
      opts.startStatsPolling()
    }
  })

  function onModeSwitch() {
    opts.resetHistory()
    opts.refresh()
    opts.startStatsPolling()
  }

  function switchMode(m: 'local' | 'remote') {
    opts.mode.value = m
    onModeSwitch()
  }

  return { openAppStore, onStoreComposeSelected, onStoreTemplateSelected, copyDockerSockHint, switchMode, onModeSwitch }
}
