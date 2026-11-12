// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// NasServer 运维配置中心（OpsConfig CRUD + 历史）—— 从 NasServer.vue 抽出的状态簇。
// opsConfigs 由本 composable 持有并 return；调用方（loadAll / 部署簇 / computed）用解构回的 ref。
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage, ElMessageBox } from 'element-plus'
import { opsConfigApi, type OpsConfig, type OpsConfigHistory, type OpsConfigInput } from '@/api/monitor'
import { useAuthStore } from '@/stores/auth'


const t = (k: string) => i18n.global.t(k)

export function useNasOpsConfigs() {
  const auth = useAuthStore()
  const opsConfigs = ref<OpsConfig[]>([])
  const configVisible = ref(false)
  const configFormVisible = ref(false)
  const editingConfig = ref<OpsConfig | null>(null)
  const configHistory = ref<OpsConfigHistory[]>([])
  const configHistoryTarget = ref<OpsConfig | null>(null)
  const configForm = ref<OpsConfigInput>({
    key: '',
    name: '',
    env: 'prod',
    format: 'kv',
    value: '{}',
    tags: '',
    note: '',
  })

  function blankConfig(): OpsConfigInput {
    return { key: '', name: '', env: 'prod', format: 'kv', value: '{}', tags: '', note: '' }
  }

  function openConfigCenter() {
    configVisible.value = true
    if (auth.isAdmin)
      opsConfigApi
        .list()
        .then((items) => {
          opsConfigs.value = items
        })
        .catch(() => {})
  }

  function openConfigAdd() {
    editingConfig.value = null
    configForm.value = blankConfig()
    configHistoryTarget.value = null
    configHistory.value = []
    configFormVisible.value = true
  }

  function openConfigEdit(c: OpsConfig) {
    editingConfig.value = c
    configForm.value = {
      key: c.key,
      name: c.name,
      env: c.env || 'prod',
      format: c.format || 'text',
      value: c.value || '',
      tags: c.tags || '',
      note: c.note || '',
    }
    configFormVisible.value = true
  }

  function validateConfigForm(): string {
    const f = configForm.value
    if (!f.key.trim()) return '请填写配置 Key'
    if (!f.name.trim()) return '请填写配置名称'
    if ((f.format === 'kv' || f.format === 'json') && f.value.trim()) {
      try {
        const parsed = JSON.parse(f.value)
        if (f.format === 'kv' && (!parsed || Array.isArray(parsed) || typeof parsed !== 'object'))
          return 'KV 格式必须是 JSON 对象'
      } catch {
        return `${f.format.toUpperCase()} 格式必须是合法 JSON`
      }
    }
    return ''
  }

  async function saveConfig() {
    const err = validateConfigForm()
    if (err) {
      ElMessage.warning(err)
      return
    }
    try {
      if (editingConfig.value) {
        await opsConfigApi.update(editingConfig.value.id, configForm.value)
        ElMessage.success(t('opusenasopsconfigs.k1'))
      } else {
        await opsConfigApi.create(configForm.value)
        ElMessage.success(t('opusenasopsconfigs.k2'))
      }
      configFormVisible.value = false
      opsConfigs.value = await opsConfigApi.list()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.saveFailed'))
    }
  }

  async function removeConfig(c: OpsConfig) {
    try {
      await ElMessageBox.confirm(`确定删除配置「${c.name}」？历史记录也会一起删除。`, t('opusenasopsconfigs.k3'), { type: 'warning' })
    } catch {
      return
    }
    try {
      await opsConfigApi.remove(c.id)
      ElMessage.success(t('opusenasopsconfigs.k4'))
      opsConfigs.value = await opsConfigApi.list()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || t('common.deleteFailed'))
    }
  }

  async function showConfigHistory(c: OpsConfig) {
    configHistoryTarget.value = c
    configHistory.value = await opsConfigApi.history(c.id).catch(() => [])
  }

  return {
    opsConfigs,
    configVisible,
    configFormVisible,
    editingConfig,
    configHistory,
    configHistoryTarget,
    configForm,
    openConfigCenter,
    openConfigAdd,
    openConfigEdit,
    saveConfig,
    removeConfig,
    showConfigHistory,
  }
}
