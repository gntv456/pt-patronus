import { http } from './http'

export const backupApi = {
  exportUrl: '/api/v1/backup/export',
  importOwn: (data: unknown) => http.post('/backup/import', data).then((r) => r.data),
  importMoviePilot: (data: unknown) => http.post('/backup/import-moviepilot', data).then((r) => r.data),
  // .ptd 是二进制 ZIP：用 FormData 上传（axios 检测到 FormData 自动设 multipart 边界，勿手动设 Content-Type）。
  importPTD: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return http.post('/backup/import-ptd', form).then((r) => r.data)
  },
}
