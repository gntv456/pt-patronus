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
    trustVisible.value = true
    trustLoading.value = true
    try {
      const cfg = await pluginApi.getTrustConfig()
      trustOfficialKey.value = cfg.official_key || ''
      trustKeysText.value = (cfg.trusted_public_keys || []).join('\n')
      trustAllowUnsigned.value = !!cfg.allow_unsigned
    } catch (e: any) {
      ElMessage.error(e?.message || '加载信任配置失败')
    } finally {
      trustLoading.value = false
    }
  }

  async function saveTrust() {
    trustSaving.value = true
    try {
      const keys = trustKeysText.value.split('\n').map((k) => k.trim()).filter(Boolean)
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
