// CookieCloud 同步（浏览器扩展推送最新 cookie → 覆盖本地失效站 + 自动新增匹配站）。
// 从 Settings.vue 抽出：管理同步开关、服务器凭证、同步/推送操作。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { cookieCloudApi, type CookieCloudConfig, type CookieCloudConfigInput } from '@/api/cookiecloud'

const ccCfg = ref<CookieCloudConfig | null>(null)
const ccForm = ref<CookieCloudConfigInput>({
  enabled: false, server: '', uuid: '', password: '', interval_min: 120, update_mode: 'disconnected', direction: 'both',
})
const ccLoading = ref(false)
const ccSaving = ref(false)
const ccSyncing = ref(false)

async function loadCookieCloud() {
  ccLoading.value = true
  try {
    ccCfg.value = await cookieCloudApi.getConfig()
    ccForm.value = {
      enabled: ccCfg.value.enabled,
      server: ccCfg.value.server,
      uuid: ccCfg.value.uuid,
      password: '',
      update_mode: ccCfg.value.update_mode || 'disconnected',
      direction: ccCfg.value.direction || 'both',
      interval_min: ccCfg.value.interval_min || 120,
    }
  } catch {
    ElMessage.error('加载 CookieCloud 配置失败')
  } finally {
    ccLoading.value = false
  }
}

async function saveCookieCloud() {
  ccSaving.value = true
  try {
    await cookieCloudApi.setConfig(ccForm.value)
    ElMessage.success('已保存')
    await loadCookieCloud()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    ccSaving.value = false
  }
}

async function syncCookieCloud() {
  ccSyncing.value = true
  try {
    const r = await cookieCloudApi.sync()
    const rep = r.report
    ElMessage.success(
      `同步完成：更新 ${rep.updated} / 新增 ${rep.added} / 跳过 ${rep.skipped} / 推送 ${rep.pushed ?? 0}`,
    )
    await loadCookieCloud()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || '同步失败')
  } finally {
    ccSyncing.value = false
  }
}

async function pushCookieCloud() {
  ccSyncing.value = true
  try {
    const r = await cookieCloudApi.push()
    ElMessage.success(`推送完成：${r.report.pushed ?? 0} 个站点 host`)
    await loadCookieCloud()
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.error || e?.message || '推送失败')
  } finally {
    ccSyncing.value = false
  }
}

export function useCookieCloud() {
  return { ccCfg, ccForm, ccLoading, ccSaving, ccSyncing, loadCookieCloud, saveCookieCloud, syncCookieCloud, pushCookieCloud }
}
