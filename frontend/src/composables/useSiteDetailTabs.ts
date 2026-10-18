import { ref } from 'vue'
import { assessmentApi, type AssessmentCard } from '@/api/assessment'
import { freeleechApi, type FreeleechResult } from '@/api/freeleech'
import { contractApi, type ContractRule, type ContractCheckResult } from '@/api/contract'
import { brushflowApi, type BrushFlowTask } from '@/api/brushflow'
import { saveptApi, type SaveptSiteCard } from '@/api/savept'
import { reseedApi } from '@/api/reseed'

// 站点详情页各 Tab 的懒加载数据与状态。
// 每个 Tab（考核·契约 / 刷流 / 辅种 / 站免 / 监护）从全局列表/任务里按 siteId 过滤出当前站点数据。
// 抽出为 composable 收敛 SiteDetail.vue 脚本体积。
export function useSiteDetailTabs(getSiteId: () => number, getSiteName: () => string | undefined) {
  const assess = ref<AssessmentCard | null>(null)
  const assessLoading = ref(false)
  const fl = ref<FreeleechResult | null>(null)
  const flLoading = ref(false)
  const contract = ref<ContractRule | null>(null)
  const contractCheck = ref<ContractCheckResult | null>(null)
  const contractLoading = ref(false)
  const bfTasks = ref<BrushFlowTask[]>([])
  const bfLoading = ref(false)
  const saveptCard = ref<SaveptSiteCard | null>(null)
  const saveptLoading = ref(false)
  const reseedHint = ref('')
  const reseedLoading = ref(false)
  // 各数据源读失败置位：区分「站点确实没有」与「列表接口没读到」（后者渲染空态是假绿）。
  const assessFailed = ref(false)
  const contractFailed = ref(false)
  const flFailed = ref(false)
  const bfFailed = ref(false)
  const saveptFailed = ref(false)

  // 请求序号：切站点时 clearTabState + 重载走同一 loadTab，序号自增即可丢弃旧站点的过期响应。
  let tabSeq = 0

  function clearTabState() {
    assess.value = null
    contract.value = null
    contractCheck.value = null
    fl.value = null
    bfTasks.value = []
    saveptCard.value = null
    reseedHint.value = ''
    assessFailed.value = false
    contractFailed.value = false
    flFailed.value = false
    bfFailed.value = false
    saveptFailed.value = false
  }

  async function loadTab(name: string) {
    const siteId = getSiteId()
    if (!siteId) return
    const seq = ++tabSeq
    if (name === 'assessment') {
      assessLoading.value = true
      contractLoading.value = true
      try {
        let cards: AssessmentCard[] = []
        let rules: ContractRule[] = []
        let assessErr = false
        let contractErr = false
        await Promise.all([
          assessmentApi
            .all(false)
            .then((r) => (cards = r))
            .catch(() => (assessErr = true)),
          contractApi
            .list()
            .then((r) => (rules = r))
            .catch(() => (contractErr = true)),
        ])
        if (seq !== tabSeq) return
        assessFailed.value = assessErr
        contractFailed.value = contractErr
        assess.value = cards.find((c) => c.site_id === siteId) || null
        contract.value = rules.find((r) => r.site_id === siteId) || null
        if (contract.value?.id) {
          try {
            contractCheck.value = await contractApi.check(contract.value.id)
          } catch {
            contractCheck.value = null
          }
        } else {
          contractCheck.value = null
        }
      } finally {
        if (seq === tabSeq) {
          assessLoading.value = false
          contractLoading.value = false
        }
      }
    } else if (name === 'freeleech') {
      flLoading.value = true
      try {
        const all = await freeleechApi.all(false)
        if (seq !== tabSeq) return
        fl.value = all.find((x) => x.site_id === siteId) || null
      } catch {
        if (seq !== tabSeq) return
        flFailed.value = true
        fl.value = null
      } finally {
        if (seq === tabSeq) flLoading.value = false
      }
    } else if (name === 'brushflow') {
      bfLoading.value = true
      try {
        const tasks = await brushflowApi.list()
        if (seq !== tabSeq) return
        bfTasks.value = tasks.filter((t) => t.site_id === siteId)
      } catch {
        if (seq !== tabSeq) return
        bfFailed.value = true
        bfTasks.value = []
      } finally {
        if (seq === tabSeq) bfLoading.value = false
      }
    } else if (name === 'savept') {
      saveptLoading.value = true
      try {
        const result = await saveptApi.get(false)
        if (seq !== tabSeq) return
        const cards = result.cards || []
        saveptCard.value =
          cards.find((c) => c.local_site_id === siteId) ||
          cards.find((c) => c.local_site_name === getSiteName()) ||
          null
      } catch {
        if (seq !== tabSeq) return
        saveptFailed.value = true
        saveptCard.value = null
      } finally {
        if (seq === tabSeq) saveptLoading.value = false
      }
    } else if (name === 'reseed') {
      reseedLoading.value = true
      try {
        // 按候选站 site_id 过滤：展示「可辅到本站」的任务
        const res = await reseedApi.tasks('', siteId)
        if (seq !== tabSeq) return
        const pending = res.tasks.filter((x) => x.status === 'pending').length
        // 后端单次最多返回 500 条，总数以 total 为准；pending 只能统计已加载的部分，
        // 被截断时要说出来，不能让用户以为「就这些」。
        const count = res.total ?? res.tasks.length
        if (count) {
          reseedHint.value = `本站相关辅种候选 ${count} 个（待处理 ${pending}${
            res.truncated ? '，待处理数仅统计最近 500 条' : ''
          }）。`
        } else {
          reseedHint.value = '暂无指向本站的辅种候选。辅种任务以下载器扫描产生，候选站匹配后会显示在此。'
        }
      } catch {
        if (seq !== tabSeq) return
        reseedHint.value = '加载辅种候选失败；可点下方进入全局辅种页。'
      } finally {
        if (seq === tabSeq) reseedLoading.value = false
      }
    }
  }

  return {
    assess,
    assessLoading,
    assessFailed,
    fl,
    flLoading,
    flFailed,
    contract,
    contractCheck,
    contractLoading,
    contractFailed,
    bfTasks,
    bfLoading,
    bfFailed,
    saveptCard,
    saveptLoading,
    saveptFailed,
    reseedHint,
    reseedLoading,
    clearTabState,
    loadTab,
  }
}
