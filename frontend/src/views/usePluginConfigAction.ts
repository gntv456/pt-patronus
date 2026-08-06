// 插件配置 + 动作执行（从 Plugins.vue 抽出）。
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { pluginApi, type PluginActionSpec, type PluginInfo, type PluginLog } from '@/api/plugin'
import { schemaFields, configGroups, defaultsFromSchema } from './pluginsHelpers'

export function usePluginConfigAction(load: () => Promise<void>, loadLogs: () => Promise<void>) {
  const configVisible = ref(false)
  const configTarget = ref<PluginInfo | null>(null)
  const configForm = ref<Record<string, any>>({})
  // 配置弹窗折叠面板当前展开的分组名(默认全展开,对标 MoviePilot)。
  const configExpanded = ref<string[]>([])
  const savingConfig = ref(false)

  const actionVisible = ref(false)
  const actionTarget = ref<PluginInfo | null>(null)
  const actionSpec = ref<PluginActionSpec | null>(null)
  const actionInputForm = ref<Record<string, any>>({})
  const actionInputText = ref('{}')
  const actionOutput = ref('')
  const actionRunning = ref(false)

  const actionHasSchema = computed(() => schemaFields(actionSpec.value?.input_schema).length > 0)

  function openConfig(p: PluginInfo) {
    configTarget.value = p
    configForm.value = { ...defaultsFromSchema(p.config_schema), ...(p.config || {}) }
    // 默认展开所有有标题分组(对标 MP:全部可见、可手动收起)。
    configExpanded.value = configGroups(p.config_schema).filter((g) => g.name).map((g) => g.name)
    configVisible.value = true
  }

  async function saveConfig() {
    if (!configTarget.value) return
    savingConfig.value = true
    try {
      await pluginApi.saveConfig(configTarget.value.id, configForm.value)
      configVisible.value = false
      ElMessage.success('配置已保存')
      await load()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '保存失败')
    } finally {
      savingConfig.value = false
    }
  }

  function openAction(p: PluginInfo, a: PluginActionSpec) {
    actionTarget.value = p
    actionSpec.value = a
    actionInputForm.value = defaultsFromSchema(a.input_schema)
    actionInputText.value = JSON.stringify(actionInputForm.value, null, 2)
    actionOutput.value = ''
    actionVisible.value = true
  }

  async function runAction() {
    if (!actionTarget.value || !actionSpec.value) return
    let input: Record<string, any>
    if (actionHasSchema.value) {
      input = { ...actionInputForm.value }
    } else {
      try {
        input = JSON.parse(actionInputText.value || '{}')
      } catch {
        ElMessage.warning('动作输入不是合法 JSON')
        return
      }
    }
    actionRunning.value = true
    try {
      const res = await pluginApi.action(actionTarget.value.id, actionSpec.value.name, input)
      actionOutput.value = JSON.stringify(res.output || {}, null, 2)
      ElMessage.success('动作执行完成')
      await loadLogs()
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '动作执行失败')
    } finally {
      actionRunning.value = false
    }
  }

  // 有标题分组 → 折叠面板;无标题字段(通常核心必填项)→ 顶部平铺,不套折叠面板。
  const configTitledGroups = computed(() =>
    configGroups(configTarget.value?.config_schema).filter((g) => g.name),
  )
  const configUntitledFields = computed(() => {
    const untitled = configGroups(configTarget.value?.config_schema).find((g) => !g.name)
    return untitled ? untitled.fields : []
  })

  return {
    configVisible, configTarget, configForm, configExpanded, savingConfig,
    openConfig, saveConfig,
    actionVisible, actionTarget, actionSpec, actionInputForm, actionInputText,
    actionOutput, actionRunning, actionHasSchema,
    openAction, runAction,
    configTitledGroups, configUntitledFields,
  }
}
