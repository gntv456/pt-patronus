// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  filterApi,
  type FilterRule,
  type FilterRuleGroup,
  type CustomRuleInput,
  type RuleGroupInput,
  type FilterBindings,
  type FilterTestResult,
  type FilterTorrent,
} from '@/api/filter'

// 错误统一友好化（http 拦截器已改写 message 为中文，兜底防 undefined）。
function errMsg(e: unknown, fb: string): string {
  return (e as Error)?.message || fb
}

export function useFilterRules() {
  const activeTab = ref<'builtin' | 'custom' | 'group'>('builtin')

  // ─────────────── 内置规则（只读） ───────────────
  const builtins = ref<FilterRule[]>([])
  const loadingBuiltins = ref(false)
  async function loadBuiltins() {
    loadingBuiltins.value = true
    try {
      builtins.value = await filterApi.builtins()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '加载内置规则失败'))
    } finally {
      loadingBuiltins.value = false
    }
  }

  // ─────────────── 自定义规则（CRUD） ───────────────
  const customs = ref<FilterRule[]>([])
  const loadingCustom = ref(false)
  async function loadCustom() {
    loadingCustom.value = true
    try {
      customs.value = await filterApi.custom()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '加载自定义规则失败'))
    } finally {
      loadingCustom.value = false
    }
  }

  const showCustomForm = ref(false)
  const editingCustom = ref<FilterRule | null>(null)
  const savingCustom = ref(false)
  const customForm = ref<CustomRuleInput>(defaultCustom())

  function defaultCustom(): CustomRuleInput {
    return { id: '', name: '', include: '', exclude: '', size_range: '', seeders: 0, publish_time: '' }
  }
  function openCreateCustom() {
    editingCustom.value = null
    customForm.value = defaultCustom()
    showCustomForm.value = true
  }
  function openEditCustom(r: FilterRule) {
    editingCustom.value = r
    customForm.value = {
      id: r.id,
      name: r.name ?? '',
      include: r.include?.[0] ?? '',
      exclude: r.exclude?.[0] ?? '',
      size_range: r.size_range ?? '',
      seeders: r.seeders ?? 0,
      publish_time: r.publish_time ?? '',
    }
    showCustomForm.value = true
  }
  async function saveCustom() {
    if (!customForm.value.id.trim()) {
      ElMessage.warning('规则 id 必填（仅字母数字）')
      return
    }
    try {
      savingCustom.value = true
      if (editingCustom.value) {
        await filterApi.updateCustom(editingCustom.value.id, customForm.value)
        ElMessage.success('已更新')
      } else {
        await filterApi.createCustom(customForm.value)
        ElMessage.success('已创建')
      }
      showCustomForm.value = false
      await loadCustom()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '保存失败'))
    } finally {
      savingCustom.value = false
    }
  }
  async function deleteCustom(r: FilterRule) {
    try {
      await ElMessageBox.confirm(`删除自定义规则「${r.id}」？被规则组引用时将被拒绝。`, '确认', { type: 'warning' })
    } catch {
      return
    }
    try {
      await filterApi.deleteCustom(r.id)
      ElMessage.success('已删除')
      await loadCustom()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '删除失败'))
    }
  }

  // ─────────────── 规则组（CRUD） ───────────────
  const groups = ref<FilterRuleGroup[]>([])
  const loadingGroups = ref(false)
  async function loadGroups() {
    loadingGroups.value = true
    try {
      groups.value = await filterApi.groups()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '加载规则组失败'))
    } finally {
      loadingGroups.value = false
    }
  }

  const showGroupForm = ref(false)
  const editingGroup = ref<FilterRuleGroup | null>(null)
  const savingGroup = ref(false)
  const groupForm = ref<RuleGroupInput>(defaultGroup())

  function defaultGroup(): RuleGroupInput {
    return { name: '', rule_string: '', media_type: '', category: '' }
  }
  function openCreateGroup() {
    editingGroup.value = null
    groupForm.value = defaultGroup()
    showGroupForm.value = true
  }
  function openEditGroup(g: FilterRuleGroup) {
    editingGroup.value = g
    groupForm.value = {
      name: g.name,
      rule_string: g.rule_string,
      media_type: g.media_type ?? '',
      category: g.category ?? '',
    }
    showGroupForm.value = true
  }
  async function saveGroup() {
    if (!groupForm.value.name.trim() || !groupForm.value.rule_string.trim()) {
      ElMessage.warning('组名与规则表达式必填')
      return
    }
    try {
      savingGroup.value = true
      if (editingGroup.value) {
        await filterApi.updateGroup(editingGroup.value.name, groupForm.value)
        ElMessage.success('已更新')
      } else {
        await filterApi.createGroup(groupForm.value)
        ElMessage.success('已创建')
      }
      showGroupForm.value = false
      await loadGroups()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '保存失败'))
    } finally {
      savingGroup.value = false
    }
  }
  async function deleteGroup(g: FilterRuleGroup) {
    try {
      await ElMessageBox.confirm(`删除规则组「${g.name}」？全局绑定中的悬空引用会被一并清理。`, '确认', {
        type: 'warning',
      })
    } catch {
      return
    }
    try {
      await filterApi.deleteGroup(g.name)
      ElMessage.success('已删除')
      await loadGroups()
      await loadBindings()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '删除失败'))
    }
  }

  // ─────────────── 规则试算 ───────────────
  const testRuleString = ref('')
  const testTitles = ref('')
  const testing = ref(false)
  const testResult = ref<FilterTestResult | null>(null)
  function fillTest(g: FilterRuleGroup) {
    testRuleString.value = g.rule_string
    testResult.value = null
  }
  async function runTest() {
    if (!testRuleString.value.trim()) {
      ElMessage.warning('请填写规则表达式')
      return
    }
    const torrents: FilterTorrent[] = testTitles.value
      .split('\n')
      .map((l) => l.trim())
      .filter(Boolean)
      .map((title) => ({ title }))
    if (!torrents.length) {
      ElMessage.warning('请至少输入一行种子标题')
      return
    }
    try {
      testing.value = true
      testResult.value = await filterApi.test({ rule_string: testRuleString.value, torrents })
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '试算失败'))
    } finally {
      testing.value = false
    }
  }

  // ─────────────── 全局绑定 ───────────────
  const bindings = ref<FilterBindings>({ search: [], subscribe: [], best_version: [] })
  const savingBindings = ref(false)
  async function loadBindings() {
    try {
      bindings.value = await filterApi.bindings()
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '加载全局绑定失败'))
    }
  }
  async function saveBindings() {
    try {
      savingBindings.value = true
      await filterApi.setBindings(bindings.value)
      ElMessage.success('全局绑定已保存')
    } catch (e: unknown) {
      ElMessage.error(errMsg(e, '保存绑定失败'))
    } finally {
      savingBindings.value = false
    }
  }

  onMounted(() => {
    loadBuiltins()
    loadCustom()
    loadGroups()
    loadBindings()
  })

  return {
    activeTab,
    builtins,
    loadingBuiltins,
    loadBuiltins,
    customs,
    loadingCustom,
    loadCustom,
    showCustomForm,
    editingCustom,
    savingCustom,
    customForm,
    openCreateCustom,
    openEditCustom,
    saveCustom,
    deleteCustom,
    groups,
    loadingGroups,
    loadGroups,
    showGroupForm,
    editingGroup,
    savingGroup,
    groupForm,
    openCreateGroup,
    openEditGroup,
    saveGroup,
    deleteGroup,
    testRuleString,
    testTitles,
    testing,
    testResult,
    fillTest,
    runTest,
    bindings,
    savingBindings,
    saveBindings,
  }
}
