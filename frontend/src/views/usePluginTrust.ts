// Plugins 签名信任配置（官方公钥只读 + 用户受信公钥 + 是否允许未签名）—— 从 Plugins.vue 抽出的状态簇。
// 自包含:无外部依赖注入（仅 pluginApi + ElMessage）。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { pluginApi } from '@/api/plugin'

export function usePluginTrust() {
  const trustVisible = ref(false)
  const trustLoading = ref(false)
  const trustSaving = ref(false)
  const trustOfficialKey = ref('')
  const trustKeysText = ref('')
  const trustAllowUnsigned = ref(false)

  async function openTrust() {
    trustLoading.value = true
    let cfg: Awaited<ReturnType<typeof pluginApi.getTrustConfig>>
    try {
      cfg = await pluginApi.getTrustConfig()
    } catch (e: any) {
      // 读不出来时绝不打开面板：面板里的草稿会是「一条公钥都没有 + 没勾允许未签名」的假空态，
      // 而保存是整份替换——用户点一次保存就把还存在的受信公钥清空。
      ElMessage.error('信任配置读不出来，未打开编辑：' + (e?.message || '未知原因'))
      return
    } finally {
      trustLoading.value = false
    }
    trustOfficialKey.value = cfg.official_key || ''
    trustKeysText.value = (cfg.trusted_public_keys || []).join('\n')
    trustAllowUnsigned.value = !!cfg.allow_unsigned
    trustVisible.value = true
  }

  async function saveTrust() {
    trustSaving.value = true
    try {
      const keys = trustKeysText.value
        .split('\n')
        .map((k) => k.trim())
        .filter(Boolean)
      await pluginApi.setTrustConfig(keys, trustAllowUnsigned.value)
      ElMessage.success('信任配置已保存')
      trustVisible.value = false
    } catch (e: any) {
      ElMessage.error(e?.message || '保存失败')
    } finally {
      trustSaving.value = false
    }
  }

  return {
    trustVisible,
    trustLoading,
    trustSaving,
    trustOfficialKey,
    trustKeysText,
    trustAllowUnsigned,
    openTrust,
    saveTrust,
  }
}
