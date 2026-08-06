import { ref } from 'vue'
import { searchApi, type SiteSearchResult } from '@/api/search'
import type { MediaInfo } from '@/api/recommend'

export function useFindOnSites() {
  // find-on-sites 弹窗
  const findVisible = ref(false)
  const findLoading = ref(false)
  const findTitle = ref('')
  const findPoster = ref('')
  const findResults = ref<SiteSearchResult[]>([])

  // Abort in-flight stream when reopening or closing the dialog.
  let findAbort: AbortController | null = null

  async function findOnSites(m: MediaInfo) {
    findAbort?.abort()
    const ctrl = new AbortController()
    findAbort = ctrl

    findVisible.value = true
    findLoading.value = true
    findTitle.value = m.title
    findPoster.value = m.poster_url || ''
    findResults.value = []

    try {
      // Stream per-site results so the dialog fills as soon as the first site returns
      // instead of waiting for the slowest site (batch /search can take tens of seconds).
      await searchApi.allStream(
        m.title,
        m.type || '',
        [],
        (r) => {
          if (findAbort !== ctrl) return
          findResults.value = [...findResults.value, { ...r, torrents: r.torrents ?? [] }]
        },
        ctrl.signal,
      )
    } catch (e: unknown) {
      if ((e as Error).name !== 'AbortError') {
        ElMessage.error((e as Error).message || '站点查找失败')
      }
    } finally {
      if (findAbort === ctrl) {
        findLoading.value = false
        findAbort = null
      }
    }
  }

  function closeFindDialog() {
    findVisible.value = false
    findAbort?.abort()
    findAbort = null
    findLoading.value = false
  }

  function abortFind() {
    findAbort?.abort()
  }

  return { findVisible, findLoading, findTitle, findPoster, findResults, findOnSites, closeFindDialog, abortFind }
}
