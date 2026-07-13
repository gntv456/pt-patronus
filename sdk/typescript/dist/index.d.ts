export interface HostContext {
    protocol_version?: number;
    plugin_id: string;
    base_url: string;
    token: string;
    permissions?: string[];
    host_capabilities?: string[];
}
export interface ActionRequest<TInput = Record<string, unknown>, TConfig = Record<string, unknown>> {
    action: string;
    input?: TInput;
    config?: TConfig;
    host?: HostContext;
}
export interface EventRequest<TData = Record<string, unknown>, TConfig = Record<string, unknown>> {
    type: string;
    data?: TData;
    config?: TConfig;
    host?: HostContext;
}
export interface SiteRef {
    id: number;
    name: string;
    base_url: string;
    username?: string;
}
export interface KVResult<T> {
    found: boolean;
    value?: T;
}
/** 一个宿主工具的元数据（GET /tools 返回的单条）。 */
export interface HostToolInfo {
    name: string;
    description?: string;
    input_schema?: Record<string, unknown>;
    permission?: string;
    category?: string;
    destructive?: boolean;
}
export interface HostCapabilities {
    api_version: string;
    plugin_id: string;
    permissions: string[];
    host_capabilities: string[];
    endpoints: string[];
}
export type FetchLike = typeof fetch;
export interface HostClientOptions {
    fetch?: FetchLike;
}
export interface PluginViewCapabilities {
    api_version: string;
    plugin_id: string;
    permissions: string[];
    host_capabilities: string[];
    methods: string[];
}
export interface PluginViewMessageTargetLike {
    postMessage(message: unknown, targetOrigin: string): void;
}
export interface PluginViewMessageEventLike {
    data?: unknown;
    source?: unknown;
}
export interface PluginViewWindowLike {
    parent?: PluginViewMessageTargetLike | null;
    addEventListener(type: 'message', listener: (event: PluginViewMessageEventLike) => void): void;
    removeEventListener(type: 'message', listener: (event: PluginViewMessageEventLike) => void): void;
}
export interface PluginViewClientOptions {
    source?: PluginViewWindowLike;
    target?: PluginViewMessageTargetLike;
    timeoutMs?: number;
    targetOrigin?: string;
    requestType?: string;
    responseType?: string;
}
export declare class PTPHostClient {
    readonly context: HostContext;
    private readonly fetchImpl;
    constructor(context: HostContext, options?: HostClientOptions);
    static fromEnv(env?: Record<string, string | undefined>): PTPHostClient | null;
    static fromPayload(payload: {
        host?: HostContext;
    }, options?: HostClientOptions): PTPHostClient | null;
    hasPermission(permission: string): boolean;
    hasCapability(capability: string): boolean;
    capabilities(): Promise<HostCapabilities>;
    config<T = Record<string, unknown>>(): Promise<T>;
    setRuntimeConfig(key: string, value: unknown): Promise<void>;
    log(level: string, event: string, message: string, data?: Record<string, unknown>): Promise<void>;
    notice(title: string, body: string, level?: string): Promise<void>;
    publish(type: string, data?: Record<string, unknown>): Promise<void>;
    kvGet<T = unknown>(key: string): Promise<KVResult<T>>;
    kvSet(key: string, value: unknown): Promise<void>;
    kvDelete(key: string): Promise<void>;
    sites(): Promise<SiteRef[]>;
    siteCookie(id: number): Promise<string>;
    /** Search the host media chain (TMDB/豆瓣/寸光集). Requires the `media:search` permission. */
    mediaSearch(keyword: string, year?: number): Promise<Record<string, unknown>[]>;
    /** List host tools this plugin is permitted to call (GET /tools, self-describing). */
    tools(): Promise<HostToolInfo[]>;
    /** Invoke a host tool by name (POST /tools/{name}). Permission gated by manifest declarations. */
    call<T = unknown>(name: string, input?: Record<string, unknown>): Promise<T>;
    torrentsList(downloaderId: number): Promise<unknown>;
    torrentsAdd(downloaderId: number, url: string, category?: string): Promise<unknown>;
    mediaserverList(): Promise<unknown>;
    mediaserverLibraryScan(serverId: number, libraryId?: string): Promise<unknown>;
    subscribeList(): Promise<unknown>;
    /** Multi-channel notification (Bark/TG/QQ/WeChat/Webhook). Perm: notify:send. Differs from notice() (站内). */
    sendNotification(title: string, body: string, level?: string): Promise<unknown>;
    dashboardOverview(): Promise<unknown>;
    private request;
}
export declare class PTPPluginViewClient {
    private readonly source;
    private readonly target;
    private readonly timeoutMs;
    private readonly targetOrigin;
    private readonly requestType;
    private readonly responseType;
    private readonly pending;
    private sequence;
    private capabilitiesCache;
    constructor(options?: PluginViewClientOptions);
    static fromWindow(options?: PluginViewClientOptions): PTPPluginViewClient | null;
    dispose(): void;
    hasPermission(permission: string): boolean;
    hasCapability(capability: string): boolean;
    capabilities(): Promise<PluginViewCapabilities>;
    config<T = Record<string, unknown>>(): Promise<T>;
    log(level: string, event: string, message: string, data?: Record<string, unknown>): Promise<void>;
    notice(title: string, body: string, level?: string): Promise<void>;
    publish(type: string, data?: Record<string, unknown>): Promise<void>;
    kvGet<T = unknown>(key: string): Promise<KVResult<T>>;
    kvSet(key: string, value: unknown): Promise<void>;
    kvDelete(key: string): Promise<void>;
    sites(): Promise<SiteRef[]>;
    /** Invoke a declared plugin action from the view (e.g. a dashboard fetching live data). */
    action(name: string, input?: Record<string, unknown>): Promise<unknown>;
    /** List host tools via the view bridge (tool.list). Same source as GET /tools. */
    tools(): Promise<HostToolInfo[]>;
    /** Invoke a host tool via the view bridge (tool.call). Result unwrapped from {result:...}. */
    call<T = unknown>(name: string, input?: Record<string, unknown>): Promise<T>;
    request<T = Record<string, unknown>>(method: string, params?: Record<string, unknown>): Promise<T>;
    private readonly handleMessage;
}
