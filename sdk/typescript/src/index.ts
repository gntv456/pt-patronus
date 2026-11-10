// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

export interface HostContext {
  protocol_version?: number
  plugin_id: string
  base_url: string
  token: string
  permissions?: string[]
  host_capabilities?: string[]
}

export interface ActionRequest<TInput = Record<string, unknown>, TConfig = Record<string, unknown>> {
  action: string
  input?: TInput
  config?: TConfig
  host?: HostContext
}

export interface EventRequest<TData = Record<string, unknown>, TConfig = Record<string, unknown>> {
  type: string
  data?: TData
  config?: TConfig
  host?: HostContext
}

export interface SiteRef {
  id: number
  name: string
  base_url: string
  username?: string
}

export interface KVResult<T> {
  found: boolean
  value?: T
}

/** 一个宿主工具的元数据（GET /tools 返回的单条）。 */
export interface HostToolInfo {
  name: string
  description?: string
  input_schema?: Record<string, unknown>
  permission?: string
  category?: string
  destructive?: boolean
}

export interface HostCapabilities {
  api_version: string
  plugin_id: string
  permissions: string[]
  host_capabilities: string[]
  endpoints: string[]
}

export type FetchLike = typeof fetch

export interface HostClientOptions {
  fetch?: FetchLike
}

export interface PluginViewCapabilities {
  api_version: string
  plugin_id: string
  permissions: string[]
  host_capabilities: string[]
  methods: string[]
}

export interface PluginViewMessageTargetLike {
  postMessage(message: unknown, targetOrigin: string): void
}

export interface PluginViewMessageEventLike {
  data?: unknown
  source?: unknown
}

export interface PluginViewWindowLike {
  parent?: PluginViewMessageTargetLike | null
  addEventListener(type: 'message', listener: (event: PluginViewMessageEventLike) => void): void
  removeEventListener(type: 'message', listener: (event: PluginViewMessageEventLike) => void): void
}

export interface PluginViewClientOptions {
  source?: PluginViewWindowLike
  target?: PluginViewMessageTargetLike
  timeoutMs?: number
  targetOrigin?: string
  requestType?: string
  responseType?: string
}

export class PTPHostClient {
  readonly context: HostContext
  private readonly fetchImpl: FetchLike

  constructor(context: HostContext, options: HostClientOptions = {}) {
    if (!context.base_url) throw new Error('host base_url is required')
    if (!context.token) throw new Error('host token is required')
    this.context = {
      ...context,
      base_url: context.base_url.replace(/\/+$/, ''),
    }
    this.fetchImpl = options.fetch ?? fetch
  }

  static fromEnv(env: Record<string, string | undefined> = envLike()): PTPHostClient | null {
    const baseURL = env.PTP_HOST_URL
    const token = env.PTP_HOST_TOKEN
    if (!baseURL || !token) return null
    return new PTPHostClient({
      protocol_version: Number(env.PTP_HOST_PROTOCOL || '1'),
      plugin_id: env.PTP_PLUGIN_ID || '',
      base_url: baseURL,
      token,
      permissions: splitEnvList(env.PTP_HOST_PERMISSIONS),
      host_capabilities: splitEnvList(env.PTP_HOST_CAPABILITIES),
    })
  }

  static fromPayload(payload: { host?: HostContext }, options: HostClientOptions = {}): PTPHostClient | null {
    if (!payload.host) return null
    return new PTPHostClient(payload.host, options)
  }

  hasPermission(permission: string): boolean {
    return hasScopedPermission(this.context.permissions ?? [], permission)
  }

  hasCapability(capability: string): boolean {
    return hasNamedCapability(this.context.host_capabilities ?? [], capability)
  }

  capabilities(): Promise<HostCapabilities> {
    return this.request<HostCapabilities>('GET', '/capabilities')
  }

  config<T = Record<string, unknown>>(): Promise<T> {
    return this.request<{ config: T }>('GET', '/config').then((r) => r.config)
  }

  setRuntimeConfig(key: string, value: unknown): Promise<void> {
    return this.request('POST', '/runtime/config', { key, value }).then(() => undefined)
  }

  log(level: string, event: string, message: string, data?: Record<string, unknown>): Promise<void> {
    return this.request('POST', '/log', { level, event, message, data }).then(() => undefined)
  }

  notice(title: string, body: string, level = 'info'): Promise<void> {
    return this.request('POST', '/notice', { title, body, level }).then(() => undefined)
  }

  publish(type: string, data?: Record<string, unknown>): Promise<void> {
    return this.request('POST', '/events', { type, data }).then(() => undefined)
  }

  kvGet<T = unknown>(key: string): Promise<KVResult<T>> {
    return this.request<KVResult<T>>('GET', `/kv/${encodeURIComponent(key)}`)
  }

  kvSet(key: string, value: unknown): Promise<void> {
    return this.request('PUT', `/kv/${encodeURIComponent(key)}`, { value }).then(() => undefined)
  }

  kvDelete(key: string): Promise<void> {
    return this.request('DELETE', `/kv/${encodeURIComponent(key)}`).then(() => undefined)
  }

  sites(): Promise<SiteRef[]> {
    return this.request<{ sites: SiteRef[] }>('GET', '/sites').then((r) => r.sites ?? [])
  }

  siteCookie(id: number): Promise<string> {
    return this.request<{ cookie: string }>('GET', `/sites/${id}/cookie`).then((r) => r.cookie)
  }

  /** Search the host media chain (TMDB/豆瓣/寸光集). Requires the `media:search` permission. */
  mediaSearch(keyword: string, year = 0): Promise<Record<string, unknown>[]> {
    return this.request<{ results: Record<string, unknown>[] }>('POST', '/media/search', { keyword, year }).then((r) => r.results ?? [])
  }

  /** List host tools this plugin is permitted to call (GET /tools, self-describing). */
  tools(): Promise<HostToolInfo[]> {
    return this.request<{ tools: HostToolInfo[] }>('GET', '/tools').then((r) => r.tools ?? [])
  }

  /** Invoke a host tool by name (POST /tools/{name}). Permission gated by manifest declarations. */
  call<T = unknown>(name: string, input?: Record<string, unknown>): Promise<T> {
    return this.request<T>('POST', `/tools/${encodeURIComponent(name)}`, { input: input ?? {} })
  }

  // ---- 核心域便捷方法（薄封装，等价于 call(name, {...})；未列出的工具请用 call）----
  torrentsList(downloaderId: number): Promise<unknown> { return this.call('torrents_list', { downloader_id: downloaderId }) }
  torrentsAdd(downloaderId: number, url: string, category = ''): Promise<unknown> { return this.call('torrents_add', { downloader_id: downloaderId, url, category }) }
  mediaserverList(): Promise<unknown> { return this.call('mediaserver_list') }
  mediaserverLibraryScan(serverId: number, libraryId = ''): Promise<unknown> { return this.call('mediaserver_library_scan', { server_id: serverId, library_id: libraryId }) }
  subscribeList(): Promise<unknown> { return this.call('subscribe_list') }
  /** Multi-channel notification (Bark/TG/QQ/WeChat/Webhook). Perm: notify:send. Differs from notice() (站内). */
  sendNotification(title: string, body: string, level = 'info'): Promise<unknown> { return this.call('send_notification', { title, body, level }) }
  dashboardOverview(): Promise<unknown> { return this.call('dashboard_overview') }

  private async request<T = Record<string, unknown>>(method: string, path: string, body?: unknown): Promise<T> {
    const response = await this.fetchImpl(`${this.context.base_url}${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${this.context.token}`,
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await response.text()
    const data = text ? JSON.parse(text) : {}
    if (!response.ok) {
      const message = typeof data?.error === 'string' ? data.error : `Host API ${method} ${path} failed`
      throw new Error(message)
    }
    return data as T
  }
}

export class PTPPluginViewClient {
  private readonly source: PluginViewWindowLike
  private readonly target: PluginViewMessageTargetLike
  private readonly timeoutMs: number
  private readonly targetOrigin: string
  private readonly requestType: string
  private readonly responseType: string
  private readonly pending = new Map<string, {
    resolve: (value: any) => void
    reject: (error: Error) => void
    timer: ReturnType<typeof setTimeout>
  }>()
  private sequence = 0
  private capabilitiesCache: PluginViewCapabilities | null = null

  constructor(options: PluginViewClientOptions = {}) {
    const source = options.source ?? windowLike()
    const target = options.target ?? source?.parent ?? null
    if (!source) throw new Error('plugin view source window is required')
    if (!target) throw new Error('plugin view target window is required')
    this.source = source
    this.target = target
    this.timeoutMs = Math.max(250, options.timeoutMs ?? 30_000)
    this.targetOrigin = options.targetOrigin ?? '*'
    this.requestType = options.requestType ?? 'plugin-host'
    this.responseType = options.responseType ?? 'plugin-host-result'
    this.source.addEventListener('message', this.handleMessage)
  }

  static fromWindow(options: PluginViewClientOptions = {}): PTPPluginViewClient | null {
    const source = options.source ?? windowLike()
    const target = options.target ?? source?.parent ?? null
    if (!source || !target) return null
    return new PTPPluginViewClient({ ...options, source, target })
  }

  dispose(): void {
    this.source.removeEventListener('message', this.handleMessage)
    for (const [id, entry] of this.pending) {
      clearTimeout(entry.timer)
      entry.reject(new Error('Plugin view client disposed'))
      this.pending.delete(id)
    }
  }

  hasPermission(permission: string): boolean {
    return hasScopedPermission(this.capabilitiesCache?.permissions ?? [], permission)
  }

  hasCapability(capability: string): boolean {
    return hasNamedCapability(this.capabilitiesCache?.host_capabilities ?? [], capability)
  }

  async capabilities(): Promise<PluginViewCapabilities> {
    const data = await this.request<PluginViewCapabilities>('capabilities')
    this.capabilitiesCache = data
    return data
  }

  config<T = Record<string, unknown>>(): Promise<T> {
    return this.request<{ config: T }>('config.get').then((r) => r.config)
  }

  log(level: string, event: string, message: string, data?: Record<string, unknown>): Promise<void> {
    return this.request('log.write', { level, event, message, data }).then(() => undefined)
  }

  notice(title: string, body: string, level = 'info'): Promise<void> {
    return this.request('notice.send', { title, body, level }).then(() => undefined)
  }

  publish(type: string, data?: Record<string, unknown>): Promise<void> {
    return this.request('event.publish', { type, data }).then(() => undefined)
  }

  kvGet<T = unknown>(key: string): Promise<KVResult<T>> {
    return this.request<KVResult<T>>('kv.get', { key })
  }

  kvSet(key: string, value: unknown): Promise<void> {
    return this.request('kv.set', { key, value }).then(() => undefined)
  }

  kvDelete(key: string): Promise<void> {
    return this.request('kv.delete', { key }).then(() => undefined)
  }

  sites(): Promise<SiteRef[]> {
    return this.request<{ sites: SiteRef[] }>('sites.list').then((r) => r.sites ?? [])
  }

  /** Invoke a declared plugin action from the view (e.g. a dashboard fetching live data). */
  action(name: string, input?: Record<string, unknown>): Promise<unknown> {
    return this.request('action.invoke', { action: name, input: input ?? {} })
  }

  /** List host tools via the view bridge (tool.list). Same source as GET /tools. */
  tools(): Promise<HostToolInfo[]> {
    return this.request<{ tools: HostToolInfo[] }>('tool.list').then((r) => r.tools ?? [])
  }

  /** Invoke a host tool via the view bridge (tool.call). Result unwrapped from {result:...}. */
  call<T = unknown>(name: string, input?: Record<string, unknown>): Promise<T> {
    return this.request<{ result: T }>('tool.call', { name, input: input ?? {} }).then((r) => r.result)
  }

  request<T = Record<string, unknown>>(method: string, params?: Record<string, unknown>): Promise<T> {
    const id = String(++this.sequence)
    this.target.postMessage({
      ptp: this.requestType,
      id,
      method,
      params: params ?? {},
    }, this.targetOrigin)
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        reject(new Error(`Plugin view bridge timeout for ${method}`))
      }, this.timeoutMs)
      this.pending.set(id, { resolve, reject, timer })
    })
  }

  private readonly handleMessage = (event: PluginViewMessageEventLike): void => {
    const data = event.data
    if (!data || typeof data !== 'object') return
    const packet = data as { ptp?: string; id?: string; ok?: boolean; result?: unknown; error?: unknown }
    if (packet.ptp !== this.responseType || !packet.id) return
    if (event.source && event.source !== this.target) return
    const entry = this.pending.get(packet.id)
    if (!entry) return
    clearTimeout(entry.timer)
    this.pending.delete(packet.id)
    if (packet.ok) {
      entry.resolve(packet.result)
      return
    }
    entry.reject(new Error(typeof packet.error === 'string' ? packet.error : 'Plugin bridge failed'))
  }
}

function envLike(): Record<string, string | undefined> {
  const maybeProcess = globalThis as typeof globalThis & {
    process?: { env?: Record<string, string | undefined> }
  }
  return maybeProcess.process?.env ?? {}
}

function windowLike(): PluginViewWindowLike | null {
  const maybeWindow = globalThis as typeof globalThis & Partial<PluginViewWindowLike>
  if (typeof maybeWindow.addEventListener !== 'function' || typeof maybeWindow.removeEventListener !== 'function') {
    return null
  }
  return maybeWindow as PluginViewWindowLike
}

function splitEnvList(value?: string): string[] {
  return (value || '')
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function hasScopedPermission(permissions: string[], permission: string): boolean {
  return permissions.some((item) => {
    if (item === '*' || item === 'host:*' || item === 'plugin:*' || item === permission) return true
    if (item.endsWith(':*')) return permission.startsWith(item.slice(0, -1))
    return false
  })
}

function hasNamedCapability(capabilities: string[], capability: string): boolean {
  return capabilities.includes(capability)
}
