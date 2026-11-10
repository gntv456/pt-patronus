// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'
import type { MediaInfo } from './recommend'

export const discoverApi = {
  // 跨源关键词搜索媒体：GET /media/discover?keyword=&year=&source=
  search: (keyword: string, year = 0, source = '') =>
    http
      .get<{ results: MediaInfo[] }>('/media/discover', {
        params: { keyword, year, source },
      })
      .then((r) => r.data.results ?? []),
}
