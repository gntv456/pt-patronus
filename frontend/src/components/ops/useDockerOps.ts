import { ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, type DockerImage, type DockerVolume, type DockerNetwork } from '@/api/docker'

// OpsDocker 的扩展操作：拉取镜像、清理、卷/网络管理。
// 从 OpsDocker.vue 抽出以控制文件行数（≤400 门禁）。
export function useDockerOps(refresh: () => void, mode: () => 'local' | 'remote', selectedTarget: () => number) {
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
    if (!pullRef.value.trim()) { ElMessage.warning('请输入镜像名'); return }
    pullLoading.value = true
    pullResult.value = ''
    try {
      const res = await dockerApi.localPull(pullRef.value.trim())
      pullResult.value = res.progress || (res.ok ? '拉取成功' : '拉取失败')
      if (res.ok) { ElMessage.success('镜像拉取完成'); refresh() }
      else ElMessage.error(res.error || '拉取失败')
    } catch (e: any) { pullResult.value = e.message; ElMessage.error(e.message) }
    finally { pullLoading.value = false }
  }

  // 清理（Prune）
  async function doPrune(type: 'container' | 'image' | 'volume' | 'network') {
    const labels: Record<string, string> = {
      container: '已停止的容器', image: '悬空镜像', volume: '无用卷', network: '无用网络',
    }
    try {
      await ElMessageBox.confirm(`确定要清理${labels[type]}吗？此操作不可撤销。`, '清理确认', { type: 'warning' })
      const res = await dockerApi.localPrune(type)
      const space = res.result?.SpaceReclaimed as number | undefined
      const msg = space ? `已清理，释放 ${(Number(space) / 1073741824).toFixed(2)} GB` : '清理完成'
      ElMessage.success(msg)
      refresh()
    } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
  }

  // 卷操作
  async function createVolume() {
    try {
      const { value } = await ElMessageBox.prompt('输入卷名', '创建卷', { confirmButtonText: '创建' })
      if (!value) return
      await dockerApi.localVolumeCreate(value)
      ElMessage.success('卷已创建')
      refresh()
    } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
  }

  async function removeVolume(v: DockerVolume) {
    try {
      await ElMessageBox.confirm(`确定删除卷「${v.Name}」吗？`, '危险操作', { type: 'error' })
      await dockerApi.localVolumeRemove(v.Name)
      ElMessage.success('卷已删除')
      refresh()
    } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
  }

  // 网络操作
  async function createNetwork() {
    try {
      const { value } = await ElMessageBox.prompt('输入网络名', '创建网络', { confirmButtonText: '创建' })
      if (!value) return
      await dockerApi.localNetworkCreate(value)
      ElMessage.success('网络已创建')
      refresh()
    } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
  }

  async function removeNetwork(n: DockerNetwork) {
    try {
      await ElMessageBox.confirm(`确定删除网络「${n.Name}」吗？`, '危险操作', { type: 'error' })
      await dockerApi.localNetworkRemove(n.Name)
      ElMessage.success('网络已删除')
      refresh()
    } catch (e: any) { if (e !== 'cancel') ElMessage.error(e.message) }
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
    if (!composePath.value.trim()) { ElMessage.warning('请输入 compose 文件所在目录'); return }
    composeLoading.value = true
    composeOutput.value = ''
    try {
      const fn = action === 'up' ? dockerApi.composeUp : dockerApi.composeDown
      const res = await fn(selectedTarget(), composePath.value.trim(), action === 'down')
      composeOutput.value = res.output || (res.ok ? '成功' : '失败')
    } catch (e: any) {
      composeOutput.value = e.message
    } finally {
      composeLoading.value = false
    }
  }


  return {
    pullDialog, pullRef, pullLoading, pullResult,
    openPull, doPull, doPrune,
    createVolume, removeVolume, createNetwork, removeNetwork,
    composeVisible, composePath, composeOutput, composeLoading, openCompose, composeAction,
  }
}
