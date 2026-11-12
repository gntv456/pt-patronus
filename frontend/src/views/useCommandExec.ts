// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 命令批量执行（exec hub）—— 从 NasServer.vue 抽出的最大状态簇。
// 依赖注入：sshTargets（SSH 主机列表 computed）/ focusExecPanel（执行面板滚动回调，组件持有）。
// commandRuns / execTargetIds 等由本 composable 持有并 return；loadAll / 部署簇用解构回的 ref。
import { ref, computed, type ComputedRef } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { commandRunApi, type CommandRun, type MonitorTarget } from '@/api/monitor'
import { useAuthStore } from '@/stores/auth'
import {
  commandRisk,
  execLogsFromRun,
  nowTime,
  type CommandRisk,
  type ExecLog,
  type ExecStatus,
} from './nasServerHelpers'


const t = (k: string, params?: Record<string, unknown>) => i18n.global.t(k, params ?? {})

export function useCommandExec(sshTargets: ComputedRef<MonitorTarget[]>, focusExecPanel: () => void) {
  const auth = useAuthStore()
  const scriptFileInput = ref<HTMLInputElement | null>(null)
  const execTargetIds = ref<number[]>([])
  const execCommand = ref('uptime && df -h')
  const execTimeout = ref(30)
  const execRunning = ref(false)
  const execLogs = ref<ExecLog[]>([])
  const commandHistory = ref<string[]>([])
  const commandRuns = ref<CommandRun[]>([])
  const selectedCommandRun = ref<CommandRun | null>(null)

  const selectedExecTargets = computed(() => sshTargets.value.filter((t) => execTargetIds.value.includes(t.id)))

  function loadCommandHistory() {
    try {
      commandHistory.value = JSON.parse(localStorage.getItem('ptp_command_history') || '[]')
    } catch {
      commandHistory.value = []
    }
  }

  function rememberCommand(command: string) {
    const cmd = command.trim()
    if (!cmd) return
    commandHistory.value = [cmd, ...commandHistory.value.filter((x) => x !== cmd)].slice(0, 8)
    localStorage.setItem('ptp_command_history', JSON.stringify(commandHistory.value))
  }

  function selectAllExecTargets() {
    execTargetIds.value = sshTargets.value.map((t) => t.id)
  }

  function selectOnlineExecTargets() {
    execTargetIds.value = sshTargets.value.filter((t) => t.last_online !== false).map((t) => t.id)
  }

  function clearExecTargets() {
    execTargetIds.value = []
  }

  function applyCommand(command: string) {
    execCommand.value = command
  }

  function pickScriptFile() {
    scriptFileInput.value?.click()
  }

  async function importScriptFile(e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    input.value = ''
    if (!file) return
    if (file.size > 256 * 1024) {
      ElMessage.warning(t('opusecommandexec.k1'))
      return
    }
    execCommand.value = await file.text()
    ElMessage.success(`已导入脚本：${file.name}`)
  }

  async function confirmCommandRun(command: string, targetCount: number, risk: CommandRisk, source: string) {
    const sourceText = source === 'deploy' ? '发布/回滚脚本' : '命令'
    if (risk.level === 'high') {
      await ElMessageBox.prompt(
        `检测到高风险操作：${risk.reason}。将在 ${targetCount} 台服务器执行${sourceText}，请输入“我确认”继续。`,
        t('opusecommandexec.k2'),
        {
          type: 'error',
          confirmButtonText: t('opusecommandexec.k3'),
          cancelButtonText: t('common.cancel'),
          inputPattern: /^我确认$/,
          inputErrorMessage: t('opusecommandexec.k4'),
        },
      )
      return
    }
    await ElMessageBox.confirm(
      t('opusecommandexec.execConfirm', { n: targetCount, source: sourceText, risk: risk.level === 'medium' ? t('opusecommandexec.riskNote', { reason: risk.reason }) : '' }),
      risk.level === 'medium' ? t('opusecommandexec.k5') : t('opusecommandexec.k6'),
      { type: risk.level === 'medium' ? 'warning' : 'info', confirmButtonText: t('opusecommandexec.k7'), cancelButtonText: t('common.cancel') },
    )
  }

  function rememberRun(run: CommandRun) {
    commandRuns.value = [run, ...commandRuns.value.filter((item) => item.id !== run.id)].slice(0, 20)
  }

  async function showCommandRun(run: CommandRun) {
    const detail = run.targets?.length ? run : await commandRunApi.get(run.id)
    selectedCommandRun.value = detail
    execLogs.value = execLogsFromRun(detail)
    focusExecPanel()
  }

  async function runBatchCommand(source = 'batch', note = '') {
    if (!auth.isAdmin) {
      ElMessage.warning(t('opusecommandexec.k8'))
      return
    }
    const command = execCommand.value.trim()
    if (!command) {
      ElMessage.warning(t('opusecommandexec.k9'))
      return
    }
    if (!selectedExecTargets.value.length) {
      ElMessage.warning(t('opusecommandexec.k10'))
      return
    }
    const startedTargets = [...selectedExecTargets.value]
    const risk = commandRisk(command)
    try {
      await confirmCommandRun(command, startedTargets.length, risk, source)
    } catch {
      return
    }

    execRunning.value = true
    rememberCommand(command)
    const base = Date.now()
    selectedCommandRun.value = null
    execLogs.value = [
      ...startedTargets.map((t, index) => ({
        id: base + index,
        targetId: t.id,
        targetName: t.name,
        command,
        status: 'running' as ExecStatus,
        time: nowTime(),
        stdout: '',
        stderr: '',
        error: '',
        durationMs: 0,
      })),
      ...execLogs.value,
    ]
    try {
      const run = await commandRunApi.create({
        target_ids: startedTargets.map((t) => t.id),
        command,
        timeout_sec: execTimeout.value,
        source,
        note,
        confirm_risk: risk.level === 'high',
      })
      selectedCommandRun.value = run
      execLogs.value = execLogsFromRun(run)
      rememberRun(run)
      if (run.failure_count) ElMessage.warning(`执行完成：成功 ${run.success_count} 台，失败 ${run.failure_count} 台`)
      else ElMessage.success(`执行完成：${run.success_count} 台全部成功`)
    } catch (e: unknown) {
      const err = (e as Error).message || '执行失败'
      execLogs.value = execLogs.value.map((log) =>
        startedTargets.some((target) => target.id === log.targetId) && log.status === 'running'
          ? { ...log, status: 'error', time: nowTime(), error: err }
          : log,
      )
      ElMessage.error(err)
    } finally {
      execRunning.value = false
    }
  }

  return {
    scriptFileInput,
    execTargetIds,
    execCommand,
    execTimeout,
    execRunning,
    execLogs,
    commandHistory,
    commandRuns,
    selectedCommandRun,
    selectedExecTargets,
    loadCommandHistory,
    selectAllExecTargets,
    selectOnlineExecTargets,
    clearExecTargets,
    applyCommand,
    pickScriptFile,
    importScriptFile,
    showCommandRun,
    runBatchCommand,
  }
}
