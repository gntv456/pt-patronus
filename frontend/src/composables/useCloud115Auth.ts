import { ref, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { cloudApi } from '@/api/cloud'

// useCloud115Auth 115 开放平台扫码授权（自 Cloud.vue 拆出）：
// 发起 → 轮询（2.5s×120 上限）→ 确认后自动建盘。组件卸载/重新发起时中止轮询。
// onDrivesChanged：授权成功建盘后回调（Cloud.vue 里刷新网盘列表）。
export function useCloud115Auth(onDrivesChanged: () => Promise<void> | void) {
  const showAuth115 = ref(false)
  const authAppId = ref('')
  const authStarting = ref(false)
  const authPolling = ref(false)
  const authQR = ref('')
  const authState = ref<{ uid: string; time: string; sign: string; verifier: string } | null>(null)
  const authStatusText = ref('')
  // 轮询取消标志：组件卸载/重新发起授权时停止，避免离开页面仍持续打接口。
  let authPollAborted = false

  onUnmounted(() => {
    authPollAborted = true
  })

  // 授权二维码：无前端 qrcode 依赖，用 115 下发的 qrcode URL（多为短链/参数串）。
  // 可直接点击跳转手机端拉起；桌面端可配合浏览器二维码插件扫。
  function qrImage(url: string): string {
    return `https://api.qrserver.com/v1/create-qr-code/?size=220x220&data=${encodeURIComponent(url)}`
  }

  function openAuth115() {
    authAppId.value = ''
    authQR.value = ''
    authState.value = null
    authStatusText.value = ''
    authPolling.value = false
    showAuth115.value = true
  }

  async function startAuth115() {
    if (!authAppId.value.trim()) {
      ElMessage.warning('请填 115 开放平台 AppID')
      return
    }
    authStarting.value = true
    authPollAborted = false
    try {
      const r = await cloudApi.start115Auth(authAppId.value.trim())
      authQR.value = r.qrcode
      authState.value = { uid: r.uid, time: r.time, sign: r.sign, verifier: r.verifier }
      authStatusText.value = '请用 115 App 扫码并确认'
      pollAuth115()
    } catch (e) {
      ElMessage.error((e as Error).message || '发起授权失败')
    } finally {
      authStarting.value = false
    }
  }

  async function pollAuth115() {
    if (!authState.value) return
    authPolling.value = true
    const max = 120
    for (let i = 0; i < max; i++) {
      if (authPollAborted) break // 页面已卸载：静默退出
      try {
        const r = await cloudApi.poll115Auth(authState.value)
        if (r.confirmed && r.token) {
          authStatusText.value = '授权成功，正在创建网盘…'
          await cloudApi.createDrive({
            name: `115开放·${new Date().toLocaleDateString()}`,
            type: '115_open',
            token: r.token,
            root_path: '/',
          })
          ElMessage.success('115 开放平台已授权并添加网盘')
          showAuth115.value = false
          authPolling.value = false
          await onDrivesChanged()
          return
        }
        if (r.status === -2 || r.status === -1) {
          authStatusText.value = '二维码已过期，请重新发起'
          break
        }
        authStatusText.value = r.status === 2 ? '已扫码，请在手机上确认' : '等待扫码…'
      } catch {
        authStatusText.value = '轮询失败，请重试'
        break
      }
      await new Promise((res) => setTimeout(res, 2500))
    }
    authPolling.value = false
  }

  return {
    showAuth115,
    authAppId,
    authStarting,
    authPolling,
    authQR,
    authState,
    authStatusText,
    qrImage,
    openAuth115,
    startAuth115,
  }
}
