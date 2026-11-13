// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 统一全站反馈：toast / 破坏性操作确认 / 撤销。
// 收敛散落的 ElMessage / ElMessageBox 调用，保证「确认 + 可撤销 + 不抢焦点」一致体验。
import { h } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'

export type FeedbackType = 'success' | 'warning' | 'error' | 'info'

export interface ConfirmOptions {
  title: string
  /** 必须写明对象名称与后果，不使用泛化文案 */
  detail: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
}

export function useFeedback() {
  /** 简短反馈：3–5s 自动消失、可手动关闭、不抢焦点 */
  function toast(
    message: string,
    type: FeedbackType = 'success',
    duration = 4000,
  ) {
    return ElMessage({ message, type, duration, showClose: true })
  }

  /** 破坏性操作确认：明确对象与后果，区分「取消」与「关闭」 */
  async function confirm(opts: ConfirmOptions): Promise<boolean> {
    try {
      await ElMessageBox.confirm(opts.detail, opts.title, {
        type: opts.danger === false ? 'info' : 'warning',
        confirmButtonText: opts.confirmText ?? '确认',
        cancelButtonText: opts.cancelText ?? '取消',
        distinguishCancelAndClose: true,
        confirmButtonClass: opts.danger === false ? '' : 'el-button--danger',
      })
      return true
    } catch {
      return false
    }
  }

  /** 可撤销反馈：8s 内点「撤销」执行回调；用于删除、清空、导入覆盖等 */
  function undo(message: string, onUndo: () => void, duration = 8000) {
    let handler: ReturnType<typeof ElMessage> | undefined
    const button = h(
      'button',
      {
        type: 'button',
        class: 'pt-undo-btn',
        onClick: () => {
          onUndo()
          handler?.close()
        },
      },
      '撤销',
    )
    handler = ElMessage({
      message: h('span', [message, ' ', button]),
      type: 'success',
      duration,
      showClose: true,
    })
    return handler
  }

  /** 失败反馈：必须给出可恢复的下一步（skill: error-recovery） */
  function error(message: string, recoveryHint = '请重试或检查配置') {
    return ElMessage({
      message: `${message}（${recoveryHint}）`,
      type: 'error',
      duration: 6000,
      showClose: true,
    })
  }

  return { toast, confirm, undo, error }
}
