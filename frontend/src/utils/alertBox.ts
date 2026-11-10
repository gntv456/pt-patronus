// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// ElMessageBox.alert 的安全封装。
//
// 为什么存在：alert 返回 promise，用户点「知道了/确认」resolve，但按 ESC / 点 X / 点遮罩
// 关闭时 **reject('cancel')**——Element Plus 的取消语义。未 catch 的 reject 会冒泡到
// window.unhandledrejection，被 main.ts 的全局兜底拦下，弹「页面出了点问题：Error: cancel」
// 全屏错误卡（用户实报：站点未联通弹窗关掉后出现）。alert 是纯通知，取消即关闭，
// 吞掉 cancel 是唯一正确语义。
//
// 全库 ElMessageBox.alert 一律走本封装；需要区分确认/取消的用 confirm 并自行 catch。
import { ElMessageBox } from 'element-plus'
import type { ElMessageBoxOptions } from 'element-plus'

/** alertBox：ElMessageBox.alert + 吞 cancel（通知型弹窗无取消语义）。 */
export function alertBox(message: string, title?: string, options?: ElMessageBoxOptions): void {
  ElMessageBox.alert(message, title, options).catch(() => {
    /* 用户关闭（ESC/X/遮罩）= reject('cancel')，纯通知无需处理 */
  })
}
