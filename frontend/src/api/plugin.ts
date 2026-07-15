import { http } from './http'

export interface PluginOption {
  label: string
  value: any
}

export interface PluginConfigField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'switch' | 'select' | 'number' | 'password' | 'multiselect'
  required?: boolean
  default?: any
  options?: PluginOption[]
  help?: string
  /** 分组名，用于配置表单分区渲染（订阅/网络/通知/定时 等）；留空归入默认组。 */
  group?: string
  /** 动态选项源，如 'sites'（用户已配置站点）。配合 type='multiselect'：前端据此拉取选项，让用户勾选而非手敲 ID。 */
  options_source?: string
}

export interface PluginScheduleSpec {
  id: string
  label?: string
  action: string
  cron: string
  /** 非空时从前端 config[此键] 读 cron 覆盖；配置为空串则该条调度停用。 */
  cron_config_key?: string
  input?: Record<string, any>
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

export interface PluginContributionSpec {
  commands?: Array<{
    id: string
    label: string
    description?: string
    action?: string
    icon?: string
  }>
  views?: Array<{
    id: string
    title: string
    location?: string
    path?: string
    icon?: string
  }>
  menus?: Array<{
    location: string
    command: string
    group?: string
    when?: string
  }>
}

export interface PluginDependency {
  id: string
  version?: string
  optional?: boolean
  reason?: string
}

export interface PluginDependencyStatus {
  id: string
  version?: string
  optional?: boolean
  reason?: string
  installed: boolean
  enabled: boolean
  installed_version?: string
  satisfied: boolean
  message?: string
}

export interface PluginInstallPlanStep {
  id: string
  name: string
  version: string
  action: 'reuse' | 'install' | 'upgrade'
  installed: boolean
  enabled: boolean
  installed_version?: string
  permissions?: string[]
}

export interface PluginInfo {
  id: string
  name: string
  version: string
  api_version?: string
  app_version?: string
  host_capabilities?: string[]
  optional_host_capabilities?: string[]
  dependencies?: PluginDependency[]
  author: string
  description: string
  homepage?: string
  repository?: string
  license?: string
  keywords?: string[]
  runtime: string
  built_in: boolean
  permissions: string[]
  events: PluginEventSpec[]
  actions: PluginActionSpec[]
  config_schema: PluginConfigField[]
  schedule?: PluginScheduleSpec[]
  contributes?: PluginContributionSpec
  available_optional_host_capabilities?: string[]
  missing_optional_host_capabilities?: string[]
  dependencies_status?: PluginDependencyStatus[]
  can_enable: boolean
  can_enable_reason?: string
  runtime_status?: PluginRuntimeStatus
  enabled: boolean
  installed: boolean
  config: Record<string, any>
  last_error?: string
  last_event_at?: string
  created_at: string
  updated_at: string
}

export interface PluginRuntimeStatus {
  state: string
  managed: boolean
  pid?: number
  base_url?: string
  host_base_url?: string
  restart_count?: number
  started_at?: string
  stopped_at?: string
  last_health_at?: string
  last_restart_at?: string
  next_restart_at?: string
  last_exit?: string
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
  /** 该插件所属市场源（归一化后 URL），安装时回传定位其 archive */
  source?: string
  api_version?: string
  app_version?: string
  host_capabilities?: string[]
  optional_host_capabilities?: string[]
  dependencies?: PluginDependency[]
  author?: string
  description?: string
  homepage?: string
  repository?: string
  license?: string
  keywords?: string[]
  /** 市场展示用分类（如「媒体库」「短剧」「网盘」「工具」），供前端分组/筛选，不进运行时签名 */
  category?: string
  publisher?: {
    id?: string
    name: string
    website?: string
    verified?: boolean
  }
  published_at?: string
  changelog?: string[]
  runtime: string
  archive: string
  sha256: string
  signature?: string
  trust_level?: string
  permissions?: string[]
  events?: PluginEventSpec[]
  actions?: PluginActionSpec[]
  config_schema?: PluginConfigField[]
  contributes?: PluginContributionSpec
  available_optional_host_capabilities?: string[]
  missing_optional_host_capabilities?: string[]
  dependencies_status?: PluginDependencyStatus[]
  dependencies_satisfied: boolean
  install_plan?: PluginInstallPlanStep[]
  installable: boolean
  install_reason?: string
  compatible: boolean
  compatibility_reason?: string
  installed: boolean
  installed_version?: string
  update_available: boolean
}

export interface PluginMarketIndex {
  version: number
  name?: string
  api_version?: string
  app_version?: string
  host_capabilities?: string[]
  plugins: PluginMarketItem[]
}

export interface PluginInstallResult {
  id: string
  name: string
  version: string
  installed_dir: string
  restart_needed: boolean
  items?: PluginInstallPlanStep[]
}

export interface PluginInstallPermissionAccept {
  id: string
  accepted_permissions: string[]
}

export interface PluginRelease {
  id: number
  plugin_id: string
  version: string
  action: string
  source?: string
  archive?: string
  sha256?: string
  snapshot_dir: string
  active: boolean
  created_at: string
}

export interface PluginAssetTicket {
  ticket: string
  expires_at?: string
  expires_in?: number
}

export const pluginApi = {
  list: () => http.get<{ plugins: PluginInfo[] }>('/plugins').then((r) => r.data.plugins ?? []),
  get: (id: string) => http.get<PluginInfo>(`/plugins/${id}`).then((r) => r.data),
  enable: (id: string, acceptedPermissions: string[] = []) =>
    http.post(`/plugins/${id}/enable`, { accepted_permissions: acceptedPermissions }).then((r) => r.data),
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
  getMarketSources: () =>
    http
      .get<{ sources: string[]; official?: string }>('/plugins/market-sources')
      .then((r) => ({ sources: r.data.sources ?? [], official: r.data.official ?? '' })),
  setMarketSources: (sources: string[]) =>
    http
      .put<{ sources: string[]; official?: string }>('/plugins/market-sources', { sources })
      .then((r) => ({ sources: r.data.sources ?? [], official: r.data.official ?? '' })),
  install: (
    source: string,
    id: string,
    acceptedPermissions: string[] = [],
    acceptedPermissionsByPlugin: PluginInstallPermissionAccept[] = [],
  ) =>
    http
      .post<PluginInstallResult>('/plugins/market/install', {
        source,
        id,
        accepted_permissions: acceptedPermissions,
        accepted_permissions_by_plugin: acceptedPermissionsByPlugin,
      })
      .then((r) => r.data),
  releases: (id: string) =>
    http.get<{ releases: PluginRelease[] }>(`/plugins/${id}/releases`).then((r) => r.data.releases ?? []),
  rollback: (id: string, releaseId: number) =>
    http.post<PluginInfo>(`/plugins/${id}/rollback`, { release_id: releaseId }).then((r) => r.data),
  reload: (id: string) => http.post<PluginInfo>(`/plugins/${id}/reload`).then((r) => r.data),
  uninstall: (id: string) => http.delete(`/plugins/${id}`).then((r) => r.data),
  // 插件签名信任配置（管理端）：官方公钥只读 + 用户受信公钥 + 是否允许未签名插件。
  getTrustConfig: () =>
    http
      .get<{ official_key: string; trusted_public_keys: string[]; allow_unsigned: boolean }>(
        '/plugins/trust-config',
      )
      .then((r) => r.data),
  setTrustConfig: (trustedPublicKeys: string[], allowUnsigned: boolean) =>
    http
      .put<{ official_key: string; trusted_public_keys: string[]; allow_unsigned: boolean }>(
        '/plugins/trust-config',
        { trusted_public_keys: trustedPublicKeys, allow_unsigned: allowUnsigned },
      )
      .then((r) => r.data),
  asset: (id: string, path = 'index.html') =>
    http
      .get<string>(`/plugins/${id}/assets/${path.replace(/^\/+/, '')}`, {
        responseType: 'text',
        transformResponse: [(data) => data],
      })
      .then((r) => r.data),
  assetTicket: (id: string) =>
    http.post<PluginAssetTicket>(`/plugins/${id}/assets/ticket`).then((r) => r.data),
  bridge: (id: string, method: string, params: Record<string, any> = {}) =>
    http
      .post<{ ok: boolean; result: Record<string, any> }>(`/plugins/${id}/bridge`, { method, params })
      .then((r) => r.data.result || {}),
}
