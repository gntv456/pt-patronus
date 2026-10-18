// SSO 失败码友好化：/auth/oidc/start 与 /auth/oidc/callback 都由浏览器整页导航进入，
// 后端失败时把错误码放进 fragment 回跳登录页（否则用户看到的是浏览器渲染的一屏 JSON）。
// 这里把码翻成人话，并给出「下一步去哪儿」——沿用 utils/siteError.ts 的做法（文案单一来源、
// 不铺 i18n 键），因为这些句子只在登录页一处出现。
const OIDC_FAIL_TEXT: Record<string, string> = {
  oidc_not_configured: 'SSO 未在这台服务上装配，请管理员到「设置 - OIDC/SSO」配置',
  oidc_disabled: 'SSO 未启用或 issuer、client_id 没填全，请管理员到「设置 - OIDC/SSO」补齐',
  oidc_provider_unreachable: '连不上 SSO 身份提供方：issuer 地址可能填错、被 SSRF 策略拦下或对方服务不可用',
  oidc_state_mismatch: 'SSO 回调校验失败（登录请求过期或浏览器没带上 state cookie），请重新点 SSO 登录',
  oidc_login_failed: 'SSO 登录未完成：与身份提供方交换令牌失败，请核对 client_id / client_secret 与 issuer',
  auth_user_rejected: '这个实例只允许一个账号，当前 SSO 身份没有被接受',
  auth_token_failed: 'SSO 身份已确认，但本机登录凭据签发失败，请查看服务日志',
}

// oidcFailText 把 fragment 里的错误码翻成用户可读的一句话；未知码保留出来，
// 便于界面与日志对上同一件事，而不是统一成「登录失败」让人猜。
export function oidcFailText(code?: string | null): string {
  if (!code) return 'SSO 登录失败'
  const hit = OIDC_FAIL_TEXT[code]
  if (hit) return hit
  return `SSO 登录失败（${code}）`
}
