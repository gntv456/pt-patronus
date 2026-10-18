import { ref, type Ref } from 'vue'
import { cloudApi, type CloudDrive, type StrmRule, type StrmRecord, type StrmResult } from '@/api/cloud'

// generateAllOutcome 把「全部规则生成」的跨规则汇总翻译成用户看到的那一句。
// 回归：以前不管结果如何都弹「已触发全部规则生成」，全部规则失败（凭据过期、目录不存在）
// 也显示成功，用户以为 .strm 已落地。
export function generateAllOutcome(s?: {
  rules: number
  generated: number
  skipped: number
  file_errors: number
  failed?: string[]
}): { kind: 'success' | 'error' | 'info'; text: string } {
  if (!s?.rules) return { kind: 'info', text: '没有启用中的 STRM 规则，未生成任何文件' }
  const failed = s.failed?.length ?? 0
  if (failed) return { kind: 'error', text: `${failed}/${s.rules} 条规则生成失败：${s.failed?.[0] ?? ''}` }
  return {
    kind: 'success',
    text:
      `全部规则生成完成：${s.rules} 条规则，新增/更新 ${s.generated}，跳过 ${s.skipped}` +
      (s.file_errors ? `，单文件失败 ${s.file_errors}` : ''),
  }
}

// 把「订阅追更」这一轮的账面翻成一句诚实的话。
// 后端修复前恒回 {ok:true}，界面无论读失败、无订阅还是零链接都 toast「匹配完成」。
export function subSyncOutcome(s?: { subs: number; records: number; linked: number; failed?: string[] }): {
  kind: 'success' | 'error' | 'info'
  text: string
} {
  if (s?.failed?.length) return { kind: 'error', text: `订阅追更未能完成：${s.failed[0]}` }
  if (!s) return { kind: 'error', text: '订阅追更没有返回结果' }
  if (!s.subs) return { kind: 'info', text: '没有生效中的订阅，本轮无需追更' }
  if (!s.records)
    return { kind: 'info', text: `网盘里没有待匹配的 STRM 记录（${s.subs} 条订阅），先用 STRM 规则扫描目录` }
  return {
    kind: 'success',
    text:
      `订阅追更完成：${s.subs} 条订阅 × ${s.records} 条待匹配记录，新链接 ${s.linked}` +
      (s.linked ? '' : '（没有新命中）'),
  }
}

function defaultRule() {
  return {
    name: '',
    drive_id: 0,
    cloud_path: '/',
    output_dir: '',
    output_tag: '',
    mount_root: '',
    template: '',
    recursive: true,
    keep_ext: true,
    enabled: true,
    incremental: false,
    include_exts: '',
    exclude_keywords: '',
    local_base_url: '',
    public_base_url: '',
  }
}

export function useCloudStrmRules(
  drives: Ref<CloudDrive[]>,
  openBrowse: (d: CloudDrive, target: (p: string) => void) => void,
) {
  const rules = ref<StrmRule[]>([])
  const records = ref<StrmRecord[]>([])

  async function loadRules() {
    try {
      rules.value = await cloudApi.listRules()
    } catch (e: any) {
      ElMessage.error(e?.message || '加载规则失败')
    }
  }
  async function loadRecords() {
    try {
      records.value = await cloudApi.listRecords()
    } catch (e: any) {
      ElMessage.error(e?.message || '加载记录失败')
    }
  }

  const driveName = (id: number) => drives.value.find((d) => d.id === id)?.name ?? `#${id}`

  // ---- STRM 规则 CRUD ----
  const showRuleForm = ref(false)
  const editingRule = ref<StrmRule | null>(null)
  const ruleForm = ref(defaultRule())
  // 保存防双击：请求在途时忽略重复提交。
  const ruleSaving = ref(false)

  function openCreateRule() {
    editingRule.value = null
    ruleForm.value = { ...defaultRule(), drive_id: drives.value[0]?.id ?? 0 }
    showRuleForm.value = true
  }
  function openEditRule(r: StrmRule) {
    editingRule.value = r
    ruleForm.value = {
      ...defaultRule(),
      ...r,
      output_tag: r.output_tag ?? '',
      include_exts: r.include_exts ?? '',
      exclude_keywords: r.exclude_keywords ?? '',
      local_base_url: r.local_base_url ?? '',
      public_base_url: r.public_base_url ?? '',
      incremental: r.incremental ?? false,
    }
    showRuleForm.value = true
  }
  function ruleBrowse() {
    const d = drives.value.find((x) => x.id === ruleForm.value.drive_id)
    if (!d) {
      ElMessage.warning('请先选择网盘')
      return
    }
    openBrowse(d, (p) => (ruleForm.value.cloud_path = p))
  }
  async function saveRule() {
    if (ruleSaving.value) return
    const f = ruleForm.value
    // 目录标签优先：填了 output_tag 时 output_dir 可留空（后端解析标签路径）。
    if (!f.name || !f.drive_id || !f.cloud_path || (!f.output_dir && !f.output_tag)) {
      ElMessage.warning('名称/网盘/云盘目录必填，输出目录与目录标签至少填一项')
      return
    }
    ruleSaving.value = true
    try {
      if (editingRule.value) {
        await cloudApi.updateRule(editingRule.value.id, f)
        ElMessage.success('已更新')
      } else {
        await cloudApi.createRule(f)
        ElMessage.success('已创建')
      }
      showRuleForm.value = false
      await loadRules()
    } catch (e: any) {
      ElMessage.error(e?.message || '保存失败')
    } finally {
      ruleSaving.value = false
    }
  }
  async function removeRule(r: StrmRule) {
    // 删除前确认（取消即中止，不再走全局 catch）。
    try {
      await ElMessageBox.confirm(`确定删除规则「${r.name}」？`, '提示', { type: 'warning' })
    } catch {
      return // 用户取消
    }
    try {
      await cloudApi.removeRule(r.id)
      ElMessage.success('已删除')
      await loadRules()
    } catch (e: any) {
      ElMessage.error(e?.message || '删除失败')
    }
  }

  // ---- 生成 ----
  const generating = ref<number | null>(null)
  // 全部生成 / 订阅追更防双击：请求在途时忽略重复点击。
  const generatingAll = ref(false)
  const subsSyncing = ref(false)
  function report(res: StrmResult, prefix: string) {
    if (res.errors?.length) {
      ElMessage.warning(`${prefix}：新增 ${res.generated}，跳过 ${res.skipped}，失败 ${res.errors.length}`)
    } else {
      ElMessage.success(`${prefix}：新增 ${res.generated}，跳过 ${res.skipped}`)
    }
  }
  async function generateOne(r: StrmRule) {
    generating.value = r.id
    try {
      const res = await cloudApi.generateRule(r.id)
      report(res, r.name)
      await loadRecords()
    } catch (e: any) {
      ElMessage.error(e?.message || '生成失败')
    } finally {
      generating.value = null
    }
  }
  async function generateAll() {
    if (generatingAll.value) return
    generatingAll.value = true
    try {
      const outcome = generateAllOutcome(await cloudApi.generateAll())
      ElMessage[outcome.kind](outcome.text)
      setTimeout(loadRecords, 1500)
    } catch (e: any) {
      ElMessage.error(e?.message || '请求失败')
    } finally {
      generatingAll.value = false
    }
  }
  async function syncSubs() {
    if (subsSyncing.value) return
    subsSyncing.value = true
    try {
      const outcome = subSyncOutcome(await cloudApi.syncSubscriptions())
      ElMessage[outcome.kind](outcome.text)
    } catch (e: any) {
      ElMessage.error(e?.message || '请求失败')
    } finally {
      subsSyncing.value = false
    }
  }
  async function removeRecord(r: StrmRecord) {
    // 删除 STRM 记录前确认（W1：此前无确认直接删——记录是 302 直链与订阅追更的索引，
    // 误删后需重新生成）。与 removeRule 同形态。
    try {
      await ElMessageBox.confirm(`确定删除记录「${r.cloud_path}」？删除后需重新生成 STRM。`, '提示', {
        type: 'warning',
      })
    } catch {
      return // 用户取消
    }
    try {
      await cloudApi.removeRecord(r.id)
      await loadRecords()
    } catch (e: any) {
      ElMessage.error(e?.message || '删除失败')
    }
  }

  return {
    rules,
    records,
    loadRules,
    loadRecords,
    driveName,
    showRuleForm,
    editingRule,
    ruleForm,
    openCreateRule,
    openEditRule,
    ruleBrowse,
    saveRule,
    removeRule,
    ruleSaving,
    generating,
    generatingAll,
    generateOne,
    generateAll,
    subsSyncing,
    syncSubs,
    removeRecord,
  }
}
