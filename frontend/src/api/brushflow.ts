import { http } from './http'

export interface BrushFlowTask {
  id: number
  name: string
  site_id: number
  downloader_id: number
  enabled: boolean
  keyword: string
  category: string
  min_size_mb: number
  max_size_mb: number
  min_seeders: number
  max_seeders: number
  free_only: boolean
  paused: boolean
  delete_ratio: number
  delete_seed_hours: number
  delete_upload_mb: number
  delete_files: boolean
  max_seeds: number
  save_path: string
  category_dl: string
  total_added: number
  total_deleted: number
  total_upload: number
  last_select_at?: string
  last_sweep_at?: string
  last_error: string
  created_at: string
  updated_at: string
}

export interface BrushFlowSeed {
  id: number
  task_id: number
  downloader_id: number
  source_key: string
  title: string
  size: number
  site_id: number
  added_at: string
  created_at: string
  updated_at: string
}

export const brushflowApi = {
  list: () => http.get<{ tasks: BrushFlowTask[] }>('/brushflow').then((r) => r.data.tasks ?? []),
  get: (id: number) => http.get<BrushFlowTask>(`/brushflow/${id}`).then((r) => r.data),
  create: (t: Partial<BrushFlowTask>) => http.post<BrushFlowTask>('/brushflow', t).then((r) => r.data),
  update: (id: number, t: Partial<BrushFlowTask>) =>
    http.put<BrushFlowTask>(`/brushflow/${id}`, t).then((r) => r.data),
  remove: (id: number) => http.delete(`/brushflow/${id}`).then((r) => r.data),
  select: (id: number) => http.post(`/brushflow/${id}/select`).then((r) => r.data),
  sweep: (id: number) => http.post(`/brushflow/${id}/sweep`).then((r) => r.data),
  seeds: (id: number) =>
    http.get<{ seeds: BrushFlowSeed[] }>(`/brushflow/${id}/seeds`).then((r) => r.data.seeds ?? []),
}
