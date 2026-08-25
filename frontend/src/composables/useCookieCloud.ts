// CookieCloud 同步（浏览器扩展推送最新 cookie → 覆盖本地失效站 + 自动新增匹配站）。
// 从 Settings.vue 抽出：管理同步开关、服务器凭证、同步/推送操作。
// mode：external=外部服务器（自建/公共 CookieCloud）；builtin=内置服务端（PTPatronus 自身
// 充当服务器，浏览器扩展直接推到 PTPatronus，免单独部署容器）。
import { computed, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  cookieCloudApi,
  type CookieCloudConfig,
  type CookieCloudConfigInput,
} from '@/api/cookiecloud'
import { copyText } from '@/utils/clipboard'
import { alertBox } from '@/utils/alertBox'

const ccCfg = ref<CookieCloudConfig | null>(null)
const ccForm = ref<CookieCloudConfigInput>({
  enabled: false, mode: 'external', server: '', uuid: '', password: '', interval_min: 120, update_mode: 'disconnected', direction: 'both',
})
const ccLoading = ref(false)
const ccSaving = ref(false)
const ccSyncing = ref(false)
// builtin 模式最近一次生成/重生成的明文密码（保存响应一次性回传；刷新后不可再见）
const ccGeneratedPassword = ref('')
// 浏览器扩展「服务器」一栏要填的内置端点地址
const ccBuiltinEndpoint = computed(() => `${window.location.origin}/api/v1/cookiecloud`)

async function loadCookieCloud() {
  ccLoading.value = true
  try {
    ccCfg.value = await cookieCloudApi.getConfig()
    ccForm.value = {
      enabled: ccCfg.value.enabled,
      mode: ccCfg.value.mode || 'external',
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

// 保存 builtin 配置后，若后端生成/重生成了密码，弹一次性展示（错过需重生成）。
// 确认按钮=复制密码；ESC/X 关闭由 alertBox 吞掉（复制入口还有面板上的按钮）。
function maybeShowGenerated(resp: { uuid?: string; password?: string }) {
  if (!resp?.password) return
  ccGeneratedPassword.value = resp.password
  alertBox(
    `新密码仅显示这一次，请立即复制并填入浏览器扩展：\n${resp.password}`,
    'CookieCloud 密码已生成',
    { confirmButtonText: '复制密码' },
  )
}

async function saveCookieCloud() {
  ccSaving.value = true
  try {
    const r = await cookieCloudApi.setConfig(ccForm.value)
    ElMessage.success('已保存')
    if (r?.uuid) ccForm.value.uuid = r.uuid
    maybeShowGenerated(r)
    await loadCookieCloud()
  } catch {
    ElMessage.error('保存失败')
  } finally {
    ccSaving.value = false
  }
}

// 重新生成 builtin 凭证（确认后随保存提交；旧数据作废）
async function regenerateBuiltinCreds() {
  try {
    await ElMessageBox.confirm(
      '重新生成 UUID 和密码？浏览器扩展需要改用新凭证，旧推送数据将作废。',
      '重新生成凭证',
      { type: 'warning', confirmButtonText: '重新生成', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  ccForm.value.regenerate = true
  ccForm.value.password = ''
  await saveCookieCloud()
  ccForm.value.regenerate = false
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
  return {
    ccCfg, ccForm, ccLoading, ccSaving, ccSyncing,
    ccGeneratedPassword, ccBuiltinEndpoint,
    loadCookieCloud, saveCookieCloud, regenerateBuiltinCreds, syncCookieCloud, pushCookieCloud,
  }
}
