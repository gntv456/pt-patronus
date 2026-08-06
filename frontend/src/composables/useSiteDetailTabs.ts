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
    if (name === 'assessment') {
      assessLoading.value = true
      contractLoading.value = true
      try {
        const [cards, rules] = await Promise.all([
          assessmentApi.all(false).catch(() => [] as AssessmentCard[]),
          contractApi.list().catch(() => [] as ContractRule[]),
        ])
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
        assessLoading.value = false
        contractLoading.value = false
      }
    } else if (name === 'freeleech') {
      flLoading.value = true
      try {
        const all = await freeleechApi.all(false)
        fl.value = all.find((x) => x.site_id === siteId) || null
      } catch {
        fl.value = null
      } finally {
        flLoading.value = false
      }
    } else if (name === 'brushflow') {
      bfLoading.value = true
      try {
        const tasks = await brushflowApi.list()
        bfTasks.value = tasks.filter((t) => t.site_id === siteId)
      } catch {
        bfTasks.value = []
      } finally {
        bfLoading.value = false
      }
    } else if (name === 'savept') {
      saveptLoading.value = true
      try {
        const result = await saveptApi.get(false)
        const cards = result.cards || []
        saveptCard.value =
          cards.find((c) => c.local_site_id === siteId) ||
          cards.find((c) => c.local_site_name === getSiteName()) ||
          null
      } catch {
        saveptCard.value = null
      } finally {
        saveptLoading.value = false
      }
    } else if (name === 'reseed') {
      reseedLoading.value = true
      try {
        // 按候选站 site_id 过滤：展示「可辅到本站」的任务
        const tasks = await reseedApi.tasks('', siteId)
        const pending = tasks.filter((x) => x.status === 'pending').length
        if (tasks.length) {
          reseedHint.value = `本站相关辅种候选 ${tasks.length} 个（待处理 ${pending}）。`
        } else {
          reseedHint.value = '暂无指向本站的辅种候选。辅种任务以下载器扫描产生，候选站匹配后会显示在此。'
        }
      } catch {
        reseedHint.value = '加载辅种候选失败；可点下方进入全局辅种页。'
      } finally {
        reseedLoading.value = false
      }
    }
  }

  return {
    assess, assessLoading, fl, flLoading, contract, contractCheck, contractLoading,
    bfTasks, bfLoading, saveptCard, saveptLoading, reseedHint, reseedLoading,
    clearTabState, loadTab,
  }
}
