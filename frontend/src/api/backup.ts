import { http } from './http'

export const backupApi = {
  exportUrl: '/api/v1/backup/export',
  // password 非空时走口令加密信封（X-Backup-Password 头，不在 URL 泄漏）。
  importOwn: (data: unknown, password = '') =>
    http
      .post('/backup/import', data, password ? { headers: { 'X-Backup-Password': password } } : undefined)
      .then((r) => r.data),
  importMoviePilot: (data: unknown) => http.post('/backup/import-moviepilot', data).then((r) => r.data),
  // .ptd 是二进制 ZIP：用 FormData 上传（axios 检测到 FormData 自动设 multipart 边界，勿手动设 Content-Type）。
  importPTD: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return http.post('/backup/import-ptd', form).then((r) => r.data)
  },
}
