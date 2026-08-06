// 运行日志（环形缓冲）。
// 从 Settings.vue 抽出：加载、清空、复制运行日志。
import { ref } from 'vue'
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
    ElMessage.error((e as Error).message || '加载日志失败')
  } finally {
    logLoading.value = false
  }
}

async function clearLogs() {
  try {
    await ElMessageBox.confirm('确定清空运行日志缓冲？此后新日志仍会正常记录。', '清空日志', {
      type: 'warning',
      confirmButtonText: '清空',
      cancelButtonText: '取消',
    })
  } catch {
    return // 用户取消
  }
  try {
    await logsApi.clear()
    logs.value = []
    ElMessage.success('已清空')
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || '清空失败')
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

export function useLogs() {
  return { logs, logLevel, logLoading, loadLogs, clearLogs, copyLogs, fmtTime, attrText }
}
