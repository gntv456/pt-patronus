import { http } from './http'

// 进站 Webhook：外部 POST /api/v1/webhooks/in/:token → 触发目标工作流/广播事件/通知。

export interface Webhook {
  id: number
  name: string
  /** 仅创建/rotate 响应一次性下发；List 不回传（后端 token 字段已脱敏） */
  token?: string
  target_workflow_id: number // 0=不直接触发
  has_secret?: boolean // 是否设了 HMAC 签名密钥
  enabled: boolean
  trigger_count: number
  last_triggered_at?: string
  created_at: string
  updated_at: string
}

export interface WebhookInput {
  name: string
  target_workflow_id?: number
  secret?: string // 空=不设/保留；非空=设置/更新签名密钥
  enabled?: boolean
}

export const webhookApi = {
  list: () => http.get<{ webhooks: Webhook[] }>('/webhooks').then((r) => r.data.webhooks ?? []),
  // 创建响应 {webhook, token}：token 仅此一次下发（List 永不回明文）。
  create: (data: WebhookInput) =>
    http.post<{ webhook: Webhook; token: string }>('/webhooks', data).then((r) => ({
      ...r.data.webhook,
      token: r.data.token,
    })),
  update: (id: number, data: WebhookInput) =>
    http.put<Webhook>(`/webhooks/${id}`, data).then((r) => r.data),
  rotate: (id: number) => http.post<{ token: string }>(`/webhooks/${id}/rotate`).then((r) => r.data),
  remove: (id: number) => http.delete(`/webhooks/${id}`).then((r) => r.data),
}
