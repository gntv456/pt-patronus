import { http } from './http'

export interface ContractRule {
  id: number
  site_id: number
  enabled: boolean
  min_ratio: number
  min_seeding: number
  min_upload_gb: number
  official_only: boolean // true=只统计官种
  min_seeding_size_gb: number // 做种总体积下限(GiB)，0=不检
  period: string // '' | 'weekly' | 'monthly'（告警去重窗口）
  start_time?: string // 生效起始(RFC3339)，留空=立即
  last_checked_at?: string
  last_status: string // ok | breach | error | ''
  last_detail: string
  created_at: string
  updated_at: string
}

export interface ContractCheckResult {
  rule_id: number
  site_id: number
  site_name: string
  status: 'ok' | 'breach' | 'error'
  detail?: string
  ratio: number
  seeding: number
  upload_gb: number
  seeding_official?: number // 官种做种数（拉到明细时）
  seeding_size_gb: number // 做种总体积(GiB)
  unsupported?: string // 不支持官种识别时的降级提示
  breaches?: string[]
}

export const contractApi = {
  list: () => http.get<{ rules: ContractRule[] }>('/contract/rules').then((r) => r.data.rules ?? []),
  upsert: (r: Partial<ContractRule>) => http.post<ContractRule>('/contract/rules', r).then((r) => r.data),
  remove: (id: number) => http.delete(`/contract/rules/${id}`).then((r) => r.data),
  check: (id: number) => http.post<ContractCheckResult>(`/contract/rules/${id}/check`).then((r) => r.data),
}
