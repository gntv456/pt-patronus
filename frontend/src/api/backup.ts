// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// importOutcomeKind 判断一次导入到底有没有内容：0 条不是「成功」。
// MoviePilot / .ptd 两条外部格式路径从前无条件弹绿色「导入完成：站点 0」，
// 而选错文件（拿自有备份去点 MP 入口）正是返回 0 条的常见原因。
export function importOutcomeKind(sites: unknown, downloaders?: unknown): 'done' | 'none' {
  const s = Number(sites ?? 0)
  const d = Number(downloaders ?? 0)
  return s + d > 0 ? 'done' : 'none'
}

// IMPORT_TIMEOUT = 0：备份导入不给客户端超时。
// 三条导入的服务端实现（internal/backup/backup.go 的 Import / ImportMoviePilot、ptd.go 的 ImportPTD）
// 收下的 ctx 参数根本没往下传，站点落库走 siteOp.CreateMany 的单事务
//（注释自证：「单事务批量：N 次 fsync → 1 次」）——所以浏览器断开既不会中止导入，
// 只会让 15s 全局超时把已经完成的导入显示成「请求超时，请稍后重试」，
// 用户接着再点一次 = 重复导入尝试，而真实账面（导入了几个/跳过几个）永远读不到。
const IMPORT_TIMEOUT = 0

// importFailureNotice 把后端 failures 折成「账面总数 + 这一条要展示几条」。
// 提示只列前 3 条是长度约束，但总数必须一起说：12 条失败只列 3 条而不报总数，
// 用户在页面上看到的就是「3 条没导入」，以为剩下的都成功了（同 #62/#77/#170 那一族）。
export const FAILURE_SHOWN_LIMIT = 3

export function importFailureNotice(failures: unknown): { total: number; shown: string[] } {
  const list = Array.isArray(failures) ? failures.map((x) => String(x)) : []
  return { total: list.length, shown: list.slice(0, FAILURE_SHOWN_LIMIT) }
}

export const backupApi = {
  exportUrl: '/api/v1/backup/export',
  // password 非空时走口令加密信封（X-Backup-Password 头，不在 URL 泄漏）。
  importOwn: (data: unknown, password = '') =>
    http
      .post('/backup/import', data, {
        headers: password ? { 'X-Backup-Password': password } : {},
        timeout: IMPORT_TIMEOUT,
      })
      .then((r) => r.data),
  importMoviePilot: (data: unknown) =>
    http.post('/backup/import-moviepilot', data, { timeout: IMPORT_TIMEOUT }).then((r) => r.data),
  // .ptd 是二进制 ZIP：用 FormData 上传（axios 检测到 FormData 自动设 multipart 边界，勿手动设 Content-Type）。
  importPTD: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return http.post('/backup/import-ptd', form, { timeout: IMPORT_TIMEOUT }).then((r) => r.data)
  },
}
