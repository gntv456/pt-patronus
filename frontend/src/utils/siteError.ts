// 站点抓取错误友好化：把后端原始英文错误映射为用户可理解的中文，
// 避免 "context deadline exceeded" 等技术黑话直接外显给普通用户。
// 原始文本仍由调用方保留在 title 属性供高级用户悬停查看。
// 提取自 views/Messages.vue，供 Messages / Announcements 等多页共用。
export function friendlySiteError(raw?: string): string {
  if (!raw) return ''
  const e = raw.toLowerCase()
  if (
    e.includes('cookie') ||
    e.includes('unauthorized') ||
    e.includes('401') ||
    e.includes('login') ||
    e.includes('auth')
  )
    return '站点登录已失效，请在「站点」页重新配置 Cookie'
  if (e.includes('deadline') || e.includes('timeout') || e.includes('timed out')) return '站点响应超时，请稍后重试'
  if (e.includes('404') || e.includes('not found') || e.includes('no such')) return '站点页面结构变更，待校准'
  if (
    e.includes('502') ||
    e.includes('503') ||
    e.includes('504') ||
    e.includes('unavailable') ||
    e.includes('server error')
  )
    return '站点暂时不可用'
  return '该站点获取失败'
}
