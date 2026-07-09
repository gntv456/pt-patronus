import { http } from './http'

// 通道测试结果（POST /notification/test 返回）。
export interface NotifySendResult {
  channel: string
  ok: boolean
  error?: string
}

// 各通道配置（GET /notification/config 回显；secret 脱敏 + *_configured 标记）。
export interface NotifyConfig {
  bark: { enabled: boolean; server: string; key: string; key_configured: boolean }
  telegram: { enabled: boolean; chat_id: string; api_url: string; token: string; token_configured: boolean }
  qq: { enabled: boolean; app_id: string; openid: string; group_openid: string; app_secret: string; app_secret_configured: boolean }
  wechat: {
    enabled: boolean; account_id: string; target: string; base_url: string
    bot_token: string; bot_token_configured: boolean
  }
  webhook: { enabled: boolean; url: string }
  dingtalk: { enabled: boolean; url: string }
  feishu: { enabled: boolean; url: string }
  email: {
    enabled: boolean; host: string; port: number; username: string
    password: string; password_configured: boolean; from: string; to: string
  }
}

// 提交体：整份配置；秘密字段空串=保留旧值（后端 merge）。
export interface NotifyConfigInput {
  bark: { enabled: boolean; server: string; key: string }
  telegram: { enabled: boolean; token: string; chat_id: string; api_url: string }
  qq: { enabled: boolean; app_id: string; app_secret: string; openid: string; group_openid: string }
  wechat: { enabled: boolean; bot_token: string; account_id: string; target: string; base_url: string }
  webhook: { enabled: boolean; url: string }
  dingtalk: { enabled: boolean; url: string }
  feishu: { enabled: boolean; url: string }
  email: {
    enabled: boolean; host: string; port: number; username: string
    password: string; from: string; to: string
  }
}

// 微信扫码状态（GET /notification/wechat/qrcode/status 返回）。
export interface WechatQRStatus {
  status: string // waiting | scanned | loggedin
  logged_in: boolean
  account_id?: string
  message?: string
}

export const notificationApi = {
  channels: () => http.get<{ channels: string[] }>('/notification/channels').then((r) => r.data.channels),
  test: (title: string, body = '') =>
    http
      .post<{ channels: string[]; results: NotifySendResult[] }>('/notification/test', { title, body })
      .then((r) => r.data),
  getConfig: () => http.get<NotifyConfig>('/notification/config').then((r) => r.data),
  setConfig: (cfg: NotifyConfigInput) => http.put<{ ok: boolean }>('/notification/config', cfg).then((r) => r.data),
  // 微信 ClawBot 扫码登录（admin）
  wechatQrcode: () =>
    http.post<{ qrcode: string; qrcode_url: string }>('/notification/wechat/qrcode').then((r) => r.data),
  wechatQrcodeStatus: (qrcode: string) =>
    http
      .get<WechatQRStatus>('/notification/wechat/qrcode/status', { params: { qrcode } })
      .then((r) => r.data),
}
