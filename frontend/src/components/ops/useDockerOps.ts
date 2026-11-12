// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, type DockerImage, type DockerVolume, type DockerNetwork } from '@/api/docker'

// OpsDocker 的扩展操作：拉取镜像、清理、卷/网络管理。
// 从 OpsDocker.vue 抽出以控制文件行数（≤400 门禁）。
export function useDockerOps(refresh: () => void, mode: () => 'local' | 'remote', selectedTarget: () => number) {
  const { t } = useI18n()
  // 拉取镜像
  const pullDialog = ref(false)
  const pullRef = ref('')
  const pullLoading = ref(false)
  const pullResult = ref('')

  function openPull() {
    pullRef.value = ''
    pullResult.value = ''
    pullDialog.value = true
  }

  async function doPull() {
    if (!pullRef.value.trim()) {
      ElMessage.warning(t('opusedockerops.k1'))
      return
    }
    pullLoading.value = true
    pullResult.value = ''
    try {
      const res = await dockerApi.localPull(pullRef.value.trim())
      pullResult.value = res.progress || (res.ok ? '拉取成功' : '拉取失败')
      if (res.ok) {
        ElMessage.success(t('opusedockerops.k2'))
        refresh()
      } else ElMessage.error(res.error || t('opusedockerops.k3'))
    } catch (e: any) {
      pullResult.value = e.message
      ElMessage.error(e.message)
    } finally {
      pullLoading.value = false
    }
  }

  // 清理（Prune）
  async function doPrune(type: 'container' | 'image' | 'volume' | 'network') {
    const labels: Record<string, string> = {
      container: '已停止的容器',
      image: '悬空镜像',
      volume: '无用卷',
      network: '无用网络',
    }
    try {
      await ElMessageBox.confirm(`确定要清理${labels[type]}吗？此操作不可撤销。`, t('opusedockerops.k4'), { type: 'warning' })
      const res = await dockerApi.localPrune(type)
      const space = res.result?.SpaceReclaimed as number | undefined
      const msg = space ? `已清理，释放 ${(Number(space) / 1073741824).toFixed(2)} GB` : '清理完成'
      ElMessage.success(msg)
      refresh()
    } catch (e: any) {
      if (e !== 'cancel') ElMessage.error(e.message)
    }
  }

  // 卷操作
  async function createVolume() {
    try {
      const { value } = await ElMessageBox.prompt(t('opusedockerops.k5'), t('opusedockerops.k6'), { confirmButtonText: t('opusedockerops.k7') })
      if (!value) return
      await dockerApi.localVolumeCreate(value)
      ElMessage.success(t('opusedockerops.k8'))
      refresh()
    } catch (e: any) {
      if (e !== 'cancel') ElMessage.error(e.message)
    }
  }

  async function removeVolume(v: DockerVolume) {
    try {
      await ElMessageBox.confirm(`确定删除卷「${v.Name}」吗？`, t('opusedockerops.k9'), { type: 'error' })
      await dockerApi.localVolumeRemove(v.Name)
      ElMessage.success(t('opusedockerops.k10'))
      refresh()
    } catch (e: any) {
      if (e !== 'cancel') ElMessage.error(e.message)
    }
  }

  // 网络操作
  async function createNetwork() {
    try {
      const { value } = await ElMessageBox.prompt(t('opusedockerops.k11'), t('opusedockerops.k12'), { confirmButtonText: t('opusedockerops.k7') })
      if (!value) return
      await dockerApi.localNetworkCreate(value)
      ElMessage.success(t('opusedockerops.k13'))
      refresh()
    } catch (e: any) {
      if (e !== 'cancel') ElMessage.error(e.message)
    }
  }

  async function removeNetwork(n: DockerNetwork) {
    try {
      await ElMessageBox.confirm(`确定删除网络「${n.Name}」吗？`, t('opusedockerops.k9'), { type: 'error' })
      await dockerApi.localNetworkRemove(n.Name)
      ElMessage.success(t('opusedockerops.k14'))
      refresh()
    } catch (e: any) {
      if (e !== 'cancel') ElMessage.error(e.message)
    }
  }

  // Compose 部署
  const composeVisible = ref(false)
  const composePath = ref('')
  const composeOutput = ref('')
  const composeLoading = ref(false)

  function openCompose() {
    composePath.value = ''
    composeOutput.value = ''
    composeVisible.value = true
  }

  async function composeAction(action: 'up' | 'down') {
    if (!composePath.value.trim()) {
      ElMessage.warning(t('opusedockerops.k15'))
      return
    }
    composeLoading.value = true
    composeOutput.value = ''
    try {
      const fn = action === 'up' ? dockerApi.composeUp : dockerApi.composeDown
      const res = await fn(selectedTarget(), composePath.value.trim(), action === 'down')
      composeOutput.value = res.output || t('opsdocker.composeActionOk')
    } catch (e: any) {
      // 失败已是 errcode 信封（compose_action_failed）：CLI 原文在 output 字段。
      composeOutput.value = e?.response?.data?.output || e.message
    } finally {
      composeLoading.value = false
    }
  }

  // 本机 Compose 对话框可见性（实际逻辑在 DockerComposeLocal 组件内）
  const localComposeVisible = ref(false)
  function openLocalCompose() {
    localComposeVisible.value = true
  }

  return {
    pullDialog,
    pullRef,
    pullLoading,
    pullResult,
    openPull,
    doPull,
    doPrune,
    createVolume,
    removeVolume,
    createNetwork,
    removeNetwork,
    composeVisible,
    composePath,
    composeOutput,
    composeLoading,
    openCompose,
    composeAction,
    localComposeVisible,
    openLocalCompose,
  }
}
