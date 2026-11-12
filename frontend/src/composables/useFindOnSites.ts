// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { ref } from 'vue'
import { i18n } from '@/i18n'
import { searchApi, type SiteSearchResult } from '@/api/search'
import type { MediaInfo } from '@/api/recommend'

const t = (k: string) => i18n.global.t(k)

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
        ElMessage.error((e as Error).message || t('opusefindonsites.k1'))
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
