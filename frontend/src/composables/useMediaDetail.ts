import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { mediaApi, type MediaInfo, type CreditPerson, type CelebrityProfile } from '@/api/media'
import { subscribeApi } from '@/api/subscribe'

export function useMediaDetail() {
  const route = useRoute()
  const router = useRouter()
  const detail = ref<MediaInfo | null>(null)
  const loading = ref(false)
  const error = ref('')
  const subscribing = ref(false)
  const moreKind = ref<'cast' | 'recommendations' | null>(null)

  const CAST_PREVIEW_LIMIT = 12
  const RECOMMENDATIONS_PREVIEW_LIMIT = 8
  const visibleCast = computed(() => (detail.value?.cast || []).slice(0, CAST_PREVIEW_LIMIT))
  const visibleRecommendations = computed(() =>
    (detail.value?.recommendations || []).slice(0, RECOMMENDATIONS_PREVIEW_LIMIT),
  )
  const moreVisible = computed({
    get: () => moreKind.value !== null,
    set: (visible: boolean) => {
      if (!visible) moreKind.value = null
    },
  })
  const moreTitle = computed(() => (moreKind.value === 'cast' ? '演员阵容' : '推荐'))

  // 演员简介弹窗
  const celVisible = ref(false)
  const celLoading = ref(false)
  const celProfile = ref<CelebrityProfile | null>(null)

  async function load() {
    const source = route.query.source as string
    const id = route.query.id as string
    if (!source || !id) {
      error.value = '缺少 source/id'
      return
    }
    moreKind.value = null
    loading.value = true
    error.value = ''
    try {
      detail.value = await mediaApi.detail(source, id)
    } catch (e: unknown) {
      error.value = (e as Error).message || '加载失败'
    } finally {
      loading.value = false
    }
  }

  // 一键用标题跨站搜种子（跳种子搜索 tab，预填关键词）
  function searchResources() {
    if (!detail.value) return
    router.push({ path: '/search', query: { keyword: detail.value.title, mode: 'torrent' } })
  }

  // 订阅：创建后 cron 自动搜索+下载（命中推送到默认下载器 + 通知）。同源同 id 已订阅则幂等。
  async function subscribe() {
    if (!detail.value) return
    subscribing.value = true
    try {
      await subscribeApi.create({
        source: detail.value.source,
        source_id: detail.value.source_id,
        title: detail.value.title,
        year: detail.value.year,
        type: detail.value.type,
        category: detail.value.type === 'tv' ? 'tv' : 'movie',
        poster_url: detail.value.poster_url,
      })
      ElMessage.success('已订阅，将自动搜索并下载到默认下载器')
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '订阅失败')
    } finally {
      subscribing.value = false
    }
  }

  // 点推荐项 → 换源/id 重载本页
  function openRec(m: MediaInfo) {
    router.push({ path: '/media-detail', query: { source: m.source, id: m.source_id } })
  }

  function openMore(kind: 'cast' | 'recommendations') {
    moreKind.value = kind
  }

  // 点演员头像 → 拉演员简介弹窗（豆瓣 /elessar/subject）
  async function openCelebrity(p: CreditPerson) {
    if (!p.id || !detail.value) return
    celLoading.value = true
    celProfile.value = null
    celVisible.value = true
    try {
      celProfile.value = await mediaApi.celebrity(detail.value.source, p.id)
    } catch (e: unknown) {
      ElMessage.error((e as Error).message || '加载演员简介失败')
      celVisible.value = false
    } finally {
      celLoading.value = false
    }
  }

  watch(() => [route.query.source, route.query.id], load, { immediate: true })

  return {
    route,
    router,
    detail,
    loading,
    error,
    subscribing,
    moreKind,
    CAST_PREVIEW_LIMIT,
    RECOMMENDATIONS_PREVIEW_LIMIT,
    visibleCast,
    visibleRecommendations,
    moreVisible,
    moreTitle,
    celVisible,
    celLoading,
    celProfile,
    load,
    searchResources,
    subscribe,
    openRec,
    openMore,
    openCelebrity,
  }
}
