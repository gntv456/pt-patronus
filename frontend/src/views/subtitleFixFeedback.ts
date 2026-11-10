// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 字幕补齐的受理回执 → 界面提示（决策与文案键抽成纯函数，便于单独取证）。
// queued=0 是终态而不是「还没跑完」：chain 在这种情况下不起后台任务，也就不会有
// 「字幕补齐完成」的通知，所以必须当场说清「本轮没有可用条目」，不能留一句「已受理」。
export type SubtitleFixOutcome = { kind: 'success' | 'warning'; key: string; params: { n: number } }

export function subtitleFixOutcome(queued: number, missing: number): SubtitleFixOutcome {
  if (queued > 0) return { kind: 'success', key: 'mediaserver.subtitleFixAccepted', params: { n: queued } }
  return { kind: 'warning', key: 'mediaserver.subtitleFixNone', params: { n: missing } }
}
