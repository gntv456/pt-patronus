// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 运行日志（环形缓冲）。
// 从 Settings.vue 抽出：加载、清空、复制运行日志。
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { logsApi, type LogEntry } from '@/api/logs'
import { copyText } from '@/utils/clipboard'

const logs = ref<LogEntry[]>([])
const logLevel = ref('')
const logLoading = ref(false)

async function loadLogs() {
  logLoading.value = true
  try {
    logs.value = await logsApi.list(logLevel.value, 500)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('opuselogs.k1'))
  } finally {
    logLoading.value = false
  }
}

async function clearLogs() {
  try {
    await ElMessageBox.confirm(t('opuselogs.k2'), t('opuselogs.k3'), {
      type: 'warning',
      confirmButtonText: t('common.clear'),
      cancelButtonText: t('common.cancel'),
    })
  } catch {
    return // 用户取消
  }
  try {
    await logsApi.clear()
    logs.value = []
    ElMessage.success(t('opuselogs.k4'))
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('opuselogs.k5'))
  }
}

// 复制当前日志（按展示顺序，最新在前），每行「时间 级别 正文  属性」。
async function copyLogs() {
  if (!logs.value.length) return
  const text = logs.value
    .map((e) => {
      const attrs = attrText(e.attrs)
      return `${fmtTime(e.time)} ${e.level} ${e.message}${attrs ? '  ' + attrs : ''}`
    })
    .join('\n')
  await copyText(text, `已复制 ${logs.value.length} 条日志`)
}

function fmtTime(t: string) {
  if (!t) return ''
  return new Date(t).toLocaleTimeString('zh-CN', { hour12: false })
}
function attrText(attrs?: { key: string; value: string }[]) {
  if (!attrs?.length) return ''
  return attrs.map((a) => `${a.key}=${a.value}`).join(' ')
}


const t = (k: string) => i18n.global.t(k)

export function useLogs() {
  return { logs, logLevel, logLoading, loadLogs, clearLogs, copyLogs, fmtTime, attrText }
}
