import { http } from './http'

export interface PluginOption {
  label: string
  value: any
}

export interface PluginConfigField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'switch' | 'select' | 'number'
  required?: boolean
  default?: any
  options?: PluginOption[]
  help?: string
}

export interface PluginEventSpec {
  type: string
  description?: string
}

export interface PluginActionSpec {
  name: string
  label: string
  description?: string
  input_schema?: PluginConfigField[]
}

export interface PluginInfo {
  id: string
  name: string
  version: string
  author: string
  description: string
  runtime: string
  built_in: boolean
  permissions: string[]
  events: PluginEventSpec[]
  actions: PluginActionSpec[]
  config_schema: PluginConfigField[]
  enabled: boolean
  installed: boolean
  config: Record<string, any>
  last_error?: string
  last_event_at?: string
  created_at: string
  updated_at: string
}

export interface PluginLog {
  id: number
  plugin_id: string
  level: string
  event: string
  message: string
  data?: Record<string, any>
  created_at: string
}

export interface PluginMarketItem {
  id: string
  name: string
  version: string
  author?: string
  description?: string
  runtime: string
  archive: string
  sha256: string
  signature?: string
  permissions?: string[]
  events?: PluginEventSpec[]
  actions?: PluginActionSpec[]
  config_schema?: PluginConfigField[]
  installed: boolean
}

export interface PluginMarketIndex {
  version: number
  name?: string
  plugins: PluginMarketItem[]
}

export interface PluginInstallResult {
  id: string
  name: string
  version: string
  installed_dir: string
  restart_needed: boolean
}

export const pluginApi = {
  list: () => http.get<{ plugins: PluginInfo[] }>('/plugins').then((r) => r.data.plugins ?? []),
  get: (id: string) => http.get<PluginInfo>(`/plugins/${id}`).then((r) => r.data),
  enable: (id: string) => http.post(`/plugins/${id}/enable`).then((r) => r.data),
  disable: (id: string) => http.post(`/plugins/${id}/disable`).then((r) => r.data),
  saveConfig: (id: string, config: Record<string, any>) =>
    http.put<PluginInfo>(`/plugins/${id}/config`, { config }).then((r) => r.data),
  action: (id: string, action: string, input: Record<string, any>) =>
    http.post<{ ok: boolean; output: Record<string, any> }>(`/plugins/${id}/action`, { action, input }).then((r) => r.data),
  logs: (pluginId = '', limit = 100) =>
    http.get<{ logs: PluginLog[] }>('/plugins/logs', { params: { plugin_id: pluginId, limit } }).then((r) => r.data.logs ?? []),
  publishEvent: (type = 'plugin.test', data: Record<string, any> = { source: 'web' }) =>
    http.post('/plugins/events', { type, data }).then((r) => r.data),
  market: (source = '') =>
    http.get<PluginMarketIndex>('/plugins/market', { params: { source } }).then((r) => r.data),
  install: (source: string, id: string) =>
    http.post<PluginInstallResult>('/plugins/market/install', { source, id }).then((r) => r.data),
  reload: (id: string) => http.post<PluginInfo>(`/plugins/${id}/reload`).then((r) => r.data),
  uninstall: (id: string) => http.delete(`/plugins/${id}`).then((r) => r.data),
}
