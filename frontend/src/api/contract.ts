import { http } from './http'

export interface ContractRule {
  id: number
  site_id: number
  enabled: boolean
  min_ratio: number
  min_seeding: number
  min_upload_gb: number
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
  breaches?: string[]
}

export const contractApi = {
  list: () => http.get<{ rules: ContractRule[] }>('/contract/rules').then((r) => r.data.rules ?? []),
  upsert: (r: Partial<ContractRule>) => http.post<ContractRule>('/contract/rules', r).then((r) => r.data),
  remove: (id: number) => http.delete(`/contract/rules/${id}`).then((r) => r.data),
  check: (id: number) =>
    http.post<ContractCheckResult>(`/contract/rules/${id}/check`).then((r) => r.data),
}
