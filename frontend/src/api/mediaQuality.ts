// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// 多评分订阅质量配置（存 SystemConfig `media.quality`）
export interface MediaQualityConfig {
  enabled: boolean // 总开关（订阅闸门）
  min_score: number // 最低综合评分（默认 6.0）
  min_votes: number // 最低投票数（统计意义，默认 100）
  min_release_days: number // 上映满 N 天才放行（默认 7）
  sources: string[] // 参与聚合的源：tmdb/douban/imdb/bangumi
}

// 单源评分明细
export interface SourceRating {
  source: string
  score: number
  votes: number
}

// 多源聚合评分 + 闸门判定
export interface RatingSummary {
  title: string
  year: number
  type: string
  aggregated: number // 加权综合评分
  votes: number // 总投票数
  release_date: string
  release_days: number // 距今天数（0=未知）
  sources: SourceRating[]
  passed: boolean
  reason: string // 未通过原因
}

export const sourceOptions = [
  { value: 'tmdb', label: 'TMDB' },
  { value: 'douban', label: '豆瓣' },
  { value: 'imdb', label: 'IMDb(OMDb)' },
  { value: 'bangumi', label: 'Bangumi' },
]

export const sourceLabel = (s: string): string => sourceOptions.find((o) => o.value === s)?.label ?? s

export const mediaQualityApi = {
  getConfig: () => http.get<MediaQualityConfig>('/media/quality').then((r) => r.data),
  setConfig: (cfg: MediaQualityConfig) => http.put('/media/quality', cfg).then((r) => r.data),
  // 多源评分聚合预览
  rating: (title: string, year = 0, type = '') =>
    http.get<RatingSummary>('/media/rating', { params: { title, year, type } }).then((r) => r.data),
}
