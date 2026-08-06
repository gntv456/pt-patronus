// 调度频率（聚合站点快照 cron，平衡站点隐蔽性与数据时效）。
// 从 Settings.vue 抽出：管理快照与保活 cron 预设与保存。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { scheduleApi, type ScheduleConfig } from '@/api/schedule'

const schedCfg = ref<ScheduleConfig | null>(null)
const schedForm = ref<{ snapshot_cron: string; keepalive_cron: string }>({ snapshot_cron: '', keepalive_cron: '' })
const schedLoading = ref(false)
const schedSaving = ref(false)
const schedPresets = [
  { label: '每 2 小时（更及时）', value: '7 */2 * * *' },
  { label: '每 4 小时（默认）', value: '7 */4 * * *' },
  { label: '每 6 小时（最隐蔽）', value: '17 */6 * * *' },
  { label: '每 12 小时', value: '23 */12 * * *' },
]
const keepalivePresets = [
  { label: '每小时（推荐）', value: '13 * * * *' },
  { label: '每 30 分钟', value: '*/30 * * * *' },
  { label: '每 2 小时', value: '17 */2 * * *' },
]

async function loadSchedule() {
  schedLoading.value = true
  try {
    schedCfg.value = await scheduleApi.getConfig()
    schedForm.value.snapshot_cron = schedCfg.value.snapshot_cron || schedCfg.value.snapshot_default
    schedForm.value.keepalive_cron = schedCfg.value.keepalive_cron || schedCfg.value.keepalive_default
  } catch {
    ElMessage.error('加载调度配置失败')
  } finally {
    schedLoading.value = false
  }
}

async function saveSchedule() {
  schedSaving.value = true
  try {
    await scheduleApi.setConfig({
      snapshot_cron: schedForm.value.snapshot_cron.trim(),
      keepalive_cron: schedForm.value.keepalive_cron.trim(),
    })
    ElMessage.success('调度配置已保存')
    await loadSchedule()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || '保存失败')
  } finally {
    schedSaving.value = false
  }
}

export function useSchedule() {
  return { schedCfg, schedForm, schedLoading, schedSaving, schedPresets, keepalivePresets, loadSchedule, saveSchedule }
}
