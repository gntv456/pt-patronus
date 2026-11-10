// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref, computed, type Ref } from 'vue'
import { siteApi, type Site, type SiteDef } from '@/api/site'
import { dashboardApi } from '@/api/dashboard'
import { siteDefSearchQueries, siteDefSearchText, sortSiteDefsBySearchRelevance } from '@/utils/siteDefSearch'

export interface SiteForm {
  name: string
  definition: string
  base_url: string
  cookie: string
  api_key: string
  passkey: string
  username: string
  password: string
  is_active: boolean
}

export function defUrls(d?: SiteDef) {
  return Array.from(new Set((d?.urls || []).map((u) => u.trim()).filter(Boolean)))
}

// 取定义默认地址：首个真实 URL（馒头 urls[0] 即 api.m-team.cc），无则回退占位。
export function defUrl(d?: SiteDef) {
  const urls = defUrls(d)
  if (urls.length) return urls[0]
  return d?.schema === 'mteam' ? 'https://api.m-team.cc' : 'https://www.hxpt.org'
}

export function useSitesForm(
  defs: Ref<SiteDef[]>,
  operationsMatrixReload: () => Promise<void>,
  reload: () => Promise<void>,
) {
  const dialogVisible = ref(false)
  // 编辑态：null=新建，否则持有正在编辑的站点 id
  const editingId = ref<number | null>(null)
  const isEdit = computed(() => editingId.value !== null)
  const dialogTitle = computed(() => (isEdit.value ? '编辑站点' : '添加站点'))
  // 提交防双击：请求在途时忽略重复提交。
  const submitting = ref(false)
  const form = ref<SiteForm>({
    name: '',
    definition: '',
    base_url: '',
    cookie: '',
    api_key: '',
    passkey: '',
    username: '',
    password: '',
    is_active: true,
  })

  // 当前选中的站点定义（据其 schema 决定表单形态：API 型站点无需 cookie/账密）
  const currentDef = computed(() => defs.value.find((d) => d.id === form.value.definition))
  const needsApiKey = computed(
    () =>
      currentDef.value?.schema === 'mteam' ||
      currentDef.value?.schema === 'yemapt' ||
      currentDef.value?.schema === 'fluxtorrent',
  )
  // FluxTorrent 的 passkey 可自动发现（首次用户信息抓取回写），手填仅作兜底
  const needsPasskey = computed(() => currentDef.value?.schema === 'fluxtorrent')
  const apiKeyPlaceholder = computed(() => {
    if (isEdit.value) return '留空则不修改'
    if (currentDef.value?.schema === 'mteam') return 'M-Team x-api-key（控制台 → 实验室 → 存储令牌）'
    if (currentDef.value?.schema === 'yemapt') return 'YemaPT API Auth Key（个人面板 → 详情 → 安全设定）'
    if (currentDef.value?.schema === 'fluxtorrent') return 'FluxTorrent API Token（fxo_ 开头，网页端 我的 → API Token 签发）'
    return 'API Key'
  })
  const baseUrlPlaceholder = computed(() => defUrl(currentDef.value))
  const baseUrlOptions = computed(() => defUrls(currentDef.value))
  // 占位优先取当前定义真实地址。

  // 选站下拉排序：可用站在前、死站在后（死站保留可选，仅标注「已关站」）。
  const sortedDefs = computed(() => [...defs.value].sort((a, b) => (a.dead === b.dead ? 0 : a.dead ? 1 : -1)))
  const definitionQuery = ref('')
  const filteredDefs = computed(() => {
    const raw = definitionQuery.value.trim()
    const q = raw.toLowerCase()
    if (!q) return sortedDefs.value
    const queries = siteDefSearchQueries(q)
    const matches = sortedDefs.value.filter((d) => {
      const text = siteDefSearchText(d)
      return queries.some((term) => text.includes(term))
    })
    return sortSiteDefsBySearchRelevance(matches, raw)
  })

  function filterDefinitionOptions(query: string) {
    definitionQuery.value = query
  }

  // 已知默认值集合：切定义时若名称/地址仍等于任一定义默认值（或为空）则跟随当前定义更新，
  // 用户手填的自定义值不覆盖；编辑态既有非默认值同样保留。
  const knownNames = computed(() => defs.value.map((d) => d.name))
  const knownUrls = computed(() => defs.value.flatMap((d) => defUrls(d)))

  // 切换定义时：名称/地址跟随当前定义（仅在仍是默认值或为空时），馒头无需特判。
  function onDefChange() {
    const d = currentDef.value
    if (!d) return
    if (!form.value.name || knownNames.value.includes(form.value.name)) form.value.name = d.name
    const url = defUrl(d)
    if (!form.value.base_url || knownUrls.value.includes(form.value.base_url)) {
      form.value.base_url = url
    }
  }

  async function openCreate() {
    try {
      defs.value = await siteApi.definitions()
    } catch {
      if (!defs.value.length) {
        ElMessage.error('加载站点定义失败')
        return
      }
      ElMessage.warning('站点定义刷新失败，使用当前缓存')
    }
    editingId.value = null
    definitionQuery.value = ''
    form.value = {
      name: '',
      definition: '',
      base_url: '',
      cookie: '',
      api_key: '',
      passkey: '',
      username: '',
      password: '',
      is_active: true,
    }
    dialogVisible.value = true
  }

  function openEdit(s: Site) {
    editingId.value = s.id
    definitionQuery.value = ''
    form.value = {
      name: s.name,
      definition: s.definition,
      base_url: s.base_url,
      cookie: '', // 留空=不修改（后端 Update 仅在非空时覆写）
      api_key: '', // 留空=不修改（馒头 x-api-key）
      passkey: '', // 留空=不修改
      username: s.username || '',
      password: '', // 留空=不修改
      is_active: s.is_active,
    }
    dialogVisible.value = true
  }

  async function submit() {
    if (submitting.value) return
    if (!form.value.name || !form.value.base_url) {
      ElMessage.warning('名称与地址必填')
      return
    }
    // API 型站点鉴权：新建必填 API Key，否则后续连接测试/搜索全部 401
    if (needsApiKey.value && !isEdit.value && !form.value.api_key.trim()) {
      ElMessage.warning(`${currentDef.value?.name || '该站点'}需填写 API Key`)
      return
    }
    // 根据定义自动推断 platform
    const def = defs.value.find((d) => d.id === form.value.definition)
    submitting.value = true
    try {
      if (isEdit.value) {
        await siteApi.update(editingId.value!, {
          name: form.value.name,
          definition: form.value.definition,
          platform: def?.schema || 'nexusphp',
          base_url: form.value.base_url,
          cookie: form.value.cookie,
          api_key: form.value.api_key,
          passkey: form.value.passkey,
          username: form.value.username,
          password: form.value.password,
          is_active: form.value.is_active,
        })
        ElMessage.success('已更新')
      } else {
        await siteApi.create({
          name: form.value.name,
          definition: form.value.definition,
          platform: def?.schema || 'nexusphp',
          base_url: form.value.base_url,
          cookie: form.value.cookie,
          api_key: form.value.api_key,
          passkey: form.value.passkey,
          username: form.value.username,
          password: form.value.password,
          is_active: form.value.is_active,
        })
        ElMessage.success('已添加')
        // 新建站点：立即抓取统计，避免切到仪表盘看到全 0。
        dashboardApi
          .sync()
          .then(() => operationsMatrixReload())
          .catch(() => {})
      }
      dialogVisible.value = false
      await reload()
      await operationsMatrixReload()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      ElMessage.error(msg || (isEdit.value ? '更新失败' : '添加失败'))
    } finally {
      submitting.value = false
    }
  }

  return {
    dialogVisible,
    editingId,
    isEdit,
    dialogTitle,
    submitting,
    form,
    currentDef,
    needsApiKey,
    needsPasskey,
    apiKeyPlaceholder,
    baseUrlPlaceholder,
    baseUrlOptions,
    sortedDefs,
    definitionQuery,
    filteredDefs,
    filterDefinitionOptions,
    onDefChange,
    openCreate,
    openEdit,
    submit,
  }
}
