// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// #294：组内「一键已读」的结果结算抽成纯函数。
//
// useNotices 的 readOne 自己 catch 掉失败并弹提示、**不向上抛**，
// 所以调用方 `await Promise.all(items.map(readOne))` 永远不会 reject——
// 紧跟其后的「整组标已读 + unreadCount=0」于是无条件执行：
// 四条全失败也会显示成「本组已全部已读」。这里按每条的真实结果结算。

export interface GroupReadOutcome {
  /** 成功标记的条数 */
  marked: number
  /** 失败的条数（后端可能各有一条原因，聚合后只出一条组级提示） */
  failed: number
  /** 该组剩余未读数（失败的那些仍然是未读） */
  unreadLeft: number
}

/**
 * items 与本函数的入参按同一顺序一一对应：results[i] = 第 i 条有没有标成功。
 * 只在 true 的那几条上写 read，避免「失败的被顺手标成已读」。
 */
export function applyGroupRead<T extends { read?: boolean }>(
  pending: T[],
  results: boolean[],
  totalUnreadBefore: number,
): GroupReadOutcome {
  let marked = 0
  pending.forEach((n, i) => {
    if (results[i]) {
      n.read = true
      marked++
    }
  })
  const failed = results.length - marked
  return { marked, failed, unreadLeft: Math.max(0, totalUnreadBefore - marked) }
}

/** 组级提示文案：部分失败与全部失败说法不同，但都不许写成「已完成」。 */
export function groupReadNoticeKey(outcome: GroupReadOutcome): 'all' | 'partial' | 'none' {
  if (outcome.failed === 0) return 'all'
  return outcome.marked === 0 ? 'none' : 'partial'
}
