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
    request<T = Record<string, unknown>>(method: string, params?: Record<string, unknown>): Promise<T>;
    private readonly handleMessage;
}
