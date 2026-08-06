// Telegram bot（入站 chat-ops）。
// 从 Settings.vue 抽出：管理 bot 开关、token、白名单与保存。
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { tgBotApi, type TgBotConfig } from '@/api/tgbot'

const tgCfg = ref<TgBotConfig | null>(null)
const tgForm = ref<{ enabled: boolean; bot_token: string; allowed_ids: string; allow_group_chat: boolean; api_url: string }>({
  enabled: false, bot_token: '', allowed_ids: '', allow_group_chat: false, api_url: '',
})
const tgLoading = ref(false)
const tgSaving = ref(false)

async function loadTgBot() {
  tgLoading.value = true
  try {
    tgCfg.value = await tgBotApi.getConfig()
    tgForm.value = {
      enabled: tgCfg.value.enabled,
      bot_token: '',
      allowed_ids: (tgCfg.value.allowed_user_ids ?? []).join(','),
      allow_group_chat: tgCfg.value.allow_group_chat,
      api_url: tgCfg.value.api_url || '',
    }
  } catch {
    ElMessage.error('加载 Telegram bot 配置失败')
  } finally {
    tgLoading.value = false
  }
}

async function saveTgBot() {
  tgSaving.value = true
  try {
    const ids = tgForm.value.allowed_ids.split(',').map((s) => Number(s.trim())).filter((n) => !isNaN(n) && n > 0)
    await tgBotApi.setConfig({
      enabled: tgForm.value.enabled,
      bot_token: tgForm.value.bot_token,
      allowed_user_ids: ids,
      allow_group_chat: tgForm.value.allow_group_chat,
      api_url: tgForm.value.api_url,
    })
    ElMessage.success('已保存（重启进程生效）')
    await loadTgBot()
  } catch {
    ElMessage.error('保存 Telegram bot 配置失败')
  } finally {
    tgSaving.value = false
  }
}

export function useTgBot() {
  return { tgCfg, tgForm, tgLoading, tgSaving, loadTgBot, saveTgBot }
}
