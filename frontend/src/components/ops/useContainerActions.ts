// OpsDocker 的容器批量操作与单容器动作（从 OpsDocker.vue 拆出，行为不变）。
// 含：单容器 start/stop/restart/remove、批量选择/全选/批量动作（破坏性操作统一确认）。
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { dockerApi, dockerRemoteActions, type DockerContainer, type DockerRemoteAction } from '@/api/docker'

export function useContainerActions(opts: {
  mode: () => 'local' | 'remote'
  selectedTarget: () => number
  refresh: () => void
  filteredContainers: () => DockerContainer[]
  containers: () => DockerContainer[]
  t: (k: string, v?: Record<string, unknown>) => string
}) {
  const actionLoading = ref(false)
  const batchMode = ref(false)
  const selectedIds = ref<Set<string>>(new Set())

  const selectedContainers = computed(() => opts.containers().filter((c) => selectedIds.value.has(c.ID)))
  const isAllSelected = computed(() => {
    const list = opts.filteredContainers()
    return list.length > 0 && list.every((c) => selectedIds.value.has(c.ID))
  })

  function toggleBatchMode() {
    batchMode.value = !batchMode.value
    if (!batchMode.value) selectedIds.value.clear()
  }

  function toggleSelect(id: string) {
    if (selectedIds.value.has(id)) selectedIds.value.delete(id)
    else selectedIds.value.add(id)
  }

  function selectAllVisible() {
    const list = opts.filteredContainers()
    const allSelected = list.every((c) => selectedIds.value.has(c.ID))
    if (allSelected) {
      list.forEach((c) => selectedIds.value.delete(c.ID))
    } else {
      list.forEach((c) => selectedIds.value.add(c.ID))
    }
  }

  function exitBatchMode() {
    batchMode.value = false
    selectedIds.value.clear()
  }

  async function doAction(container: DockerContainer, action: string, label: string) {
    actionLoading.value = true
    try {
      const name = container.Names.replace(/^\//, '')
      if (opts.mode() === 'local') {
        await dockerApi.localAction(name, action)
      } else {
        await dockerRemoteActions[action as DockerRemoteAction](opts.selectedTarget(), name)
      }
      ElMessage.success(opts.t('opsdocker.actionOk', { label }))
      opts.refresh()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message)
    } finally {
      actionLoading.value = false
    }
  }

  async function removeContainer(container: DockerContainer) {
    const name = container.Names.replace(/^\//, '')
    try {
      await ElMessageBox.confirm(opts.t('opsdocker.removeConfirm', { name }), opts.t('opsdisk.confirmPartTitle'), {
        type: 'warning',
        confirmButtonText: opts.t('opsdocker.forceRemove'),
        cancelButtonText: opts.t('common.cancel'),
      })
      actionLoading.value = true
      if (opts.mode() === 'local') {
        await dockerApi.localRemove(name, true, true)
      } else {
        await dockerApi.remove(opts.selectedTarget(), name, true, true)
      }
      ElMessage.success(opts.t('opsdocker.containerRemoved'))
      opts.refresh()
    } catch (e: unknown) {
      if (e !== 'cancel') ElMessage.error((e as Error).message || opts.t('opsdocker.removeCancelled'))
    } finally {
      actionLoading.value = false
    }
  }

  async function batchAction(action: 'start' | 'stop' | 'restart' | 'remove') {
    const containers = selectedContainers.value
    if (!containers.length) {
      ElMessage.warning(opts.t('opsdocker.pickFirst'))
      return
    }
    const labels: Record<string, string> = {
      start: opts.t('opsdocker.act.start'),
      stop: opts.t('opsdocker.act.stop'),
      restart: opts.t('opsdocker.act.restart'),
      remove: opts.t('common.delete'),
    }
    const names = containers.map((c) => c.Names.replace(/^\//, '')).join('、')
    // remove/stop/restart 均为破坏性批量操作，统一确认
    if (action === 'remove' || action === 'stop' || action === 'restart') {
      const verb =
        action === 'remove'
          ? opts.t('common.delete')
          : action === 'stop'
            ? opts.t('opsdocker.act.stop')
            : opts.t('opsdocker.act.restart')
      try {
        await ElMessageBox.confirm(
          opts.t('opsdocker.batchConfirm', { verb, n: containers.length }) +
            '\n' +
            names +
            (action === 'remove' ? '\n' + opts.t('opsdocker.irreversible') : ''),
          opts.t('opsdocker.batchTitle', { verb }),
          {
            type: action === 'remove' ? 'error' : 'warning',
            confirmButtonText: opts.t('opsdocker.confirmVerb', { verb }),
            cancelButtonText: opts.t('common.cancel'),
          },
        )
      } catch {
        return
      }
    }
    actionLoading.value = true
    let ok = 0
    let fail = 0
    for (const c of containers) {
      try {
        const name = c.Names.replace(/^\//, '')
        if (action === 'remove') {
          if (opts.mode() === 'local') await dockerApi.localRemove(name, true, true)
          else await dockerApi.remove(opts.selectedTarget(), name, true, true)
        } else {
          if (opts.mode() === 'local') await dockerApi.localAction(name, action)
          else await dockerRemoteActions[action as DockerRemoteAction](opts.selectedTarget(), name)
        }
        ok++
      } catch {
        fail++
      }
    }
    actionLoading.value = false
    if (fail === 0) ElMessage.success(opts.t('opsdocker.batchDone', { verb: labels[action], n: ok }))
    else ElMessage.warning(opts.t('opsdocker.batchPartial', { verb: labels[action], ok, fail }))
    exitBatchMode()
    opts.refresh()
  }

  return {
    actionLoading,
    batchMode,
    selectedIds,
    selectedContainers,
    isAllSelected,
    toggleBatchMode,
    toggleSelect,
    selectAllVisible,
    exitBatchMode,
    doAction,
    removeContainer,
    batchAction,
  }
}
