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
  }

  async function loadTab(name: string) {
    const siteId = getSiteId()
    if (!siteId) return
    const seq = ++tabSeq
    if (name === 'assessment') {
      assessLoading.value = true
      contractLoading.value = true
      try {
        const [cards, rules] = await Promise.all([
          assessmentApi.all(false).catch(() => [] as AssessmentCard[]),
          contractApi.list().catch(() => [] as ContractRule[]),
        ])
        if (seq !== tabSeq) return
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
        saveptCard.value = null
      } finally {
        if (seq === tabSeq) saveptLoading.value = false
      }
    } else if (name === 'reseed') {
      reseedLoading.value = true
      try {
        // 按候选站 site_id 过滤：展示「可辅到本站」的任务
        const tasks = await reseedApi.tasks('', siteId)
        if (seq !== tabSeq) return
        const pending = tasks.filter((x) => x.status === 'pending').length
        if (tasks.length) {
          reseedHint.value = `本站相关辅种候选 ${tasks.length} 个（待处理 ${pending}）。`
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
    fl,
    flLoading,
    contract,
    contractCheck,
    contractLoading,
    bfTasks,
    bfLoading,
    saveptCard,
    saveptLoading,
    reseedHint,
    reseedLoading,
    clearTabState,
    loadTab,
  }
}
