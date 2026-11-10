// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Docker 运维页初始化与模式切换：应用商店模板选中、docker.sock 提示复制、
// 首次挂载探测本机可用性并启动轮询、模式切换/刷新时重新探测并重启统计。
// 从 OpsDocker.vue 迁出。
import { onMounted, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
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
  // 本机可用性的唯一判据（useDockerData.initLocalAvailability）：它同时写 available
  // 和后端给的那句原因。这里以前另抄了一份 try/catch，只取 available、把原因丢掉，
  // 于是同一次挂载发两个探测请求，两处各自决定界面状态。
  probeLocalAvailability: () => Promise<void>
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
    await opts.probeLocalAvailability()
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

  async function switchMode(m: 'local' | 'remote') {
    opts.mode.value = m
    // 上一次探测可能是几分钟前：引擎那时在、现在不一定。切回本机时重探一次，
    // 免得界面停在「可用」的旧判定上、列表却一直报错。
    if (m === 'local') await opts.probeLocalAvailability()
    onModeSwitch()
  }

  // 工具栏「刷新」是唯一的解锁入口：本机按钮在 localAvailable=false 时是 disabled 的，
  // 用户按下 Docker 之后既点不到「本机」，也没有别的地方会重探——从前只能整页刷新。
  // 所以刷新必须连同可用性一起重新问一遍（远程模式下多一个 GET，换来按钮会自己活过来）。
  async function reprobe() {
    await opts.probeLocalAvailability()
    opts.refresh()
  }

  return {
    openAppStore,
    onStoreComposeSelected,
    onStoreTemplateSelected,
    copyDockerSockHint,
    switchMode,
    reprobe,
    onModeSwitch,
  }
}
