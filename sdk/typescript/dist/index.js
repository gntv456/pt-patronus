export class PTPHostClient {
    context;
    fetchImpl;
    constructor(context, options = {}) {
        if (!context.base_url)
            throw new Error('host base_url is required');
        if (!context.token)
            throw new Error('host token is required');
        this.context = {
            ...context,
            base_url: context.base_url.replace(/\/+$/, ''),
        };
        this.fetchImpl = options.fetch ?? fetch;
    }
    static fromEnv(env = envLike()) {
        const baseURL = env.PTP_HOST_URL;
        const token = env.PTP_HOST_TOKEN;
        if (!baseURL || !token)
            return null;
        return new PTPHostClient({
            protocol_version: Number(env.PTP_HOST_PROTOCOL || '1'),
            plugin_id: env.PTP_PLUGIN_ID || '',
            base_url: baseURL,
            token,
            host_capabilities: splitEnvList(env.PTP_HOST_CAPABILITIES),
        });
    }
    static fromPayload(payload, options = {}) {
        if (!payload.host)
            return null;
        return new PTPHostClient(payload.host, options);
    }
    hasPermission(permission) {
        return hasScopedPermission(this.context.permissions ?? [], permission);
    }
    hasCapability(capability) {
        return hasNamedCapability(this.context.host_capabilities ?? [], capability);
    }
    capabilities() {
        return this.request('GET', '/capabilities');
    }
    config() {
        return this.request('GET', '/config').then((r) => r.config);
    }
    setRuntimeConfig(key, value) {
        return this.request('POST', '/runtime/config', { key, value }).then(() => undefined);
    }
    log(level, event, message, data) {
        return this.request('POST', '/log', { level, event, message, data }).then(() => undefined);
    }
    notice(title, body, level = 'info') {
        return this.request('POST', '/notice', { title, body, level }).then(() => undefined);
    }
    publish(type, data) {
        return this.request('POST', '/events', { type, data }).then(() => undefined);
    }
    kvGet(key) {
        return this.request('GET', `/kv/${encodeURIComponent(key)}`);
    }
    kvSet(key, value) {
        return this.request('PUT', `/kv/${encodeURIComponent(key)}`, { value }).then(() => undefined);
    }
    kvDelete(key) {
        return this.request('DELETE', `/kv/${encodeURIComponent(key)}`).then(() => undefined);
    }
    sites() {
        return this.request('GET', '/sites').then((r) => r.sites ?? []);
    }
    siteCookie(id) {
        return this.request('GET', `/sites/${id}/cookie`).then((r) => r.cookie);
    }
    async request(method, path, body) {
        const response = await this.fetchImpl(`${this.context.base_url}${path}`, {
            method,
            headers: {
                Authorization: `Bearer ${this.context.token}`,
                ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
            },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const text = await response.text();
        const data = text ? JSON.parse(text) : {};
        if (!response.ok) {
            const message = typeof data?.error === 'string' ? data.error : `Host API ${method} ${path} failed`;
            throw new Error(message);
        }
        return data;
    }
}
export class PTPPluginViewClient {
    source;
    target;
    timeoutMs;
    targetOrigin;
    requestType;
    responseType;
    pending = new Map();
    sequence = 0;
    capabilitiesCache = null;
    constructor(options = {}) {
        const source = options.source ?? windowLike();
        const target = options.target ?? source?.parent ?? null;
        if (!source)
            throw new Error('plugin view source window is required');
        if (!target)
            throw new Error('plugin view target window is required');
        this.source = source;
        this.target = target;
        this.timeoutMs = Math.max(250, options.timeoutMs ?? 30_000);
        this.targetOrigin = options.targetOrigin ?? '*';
        this.requestType = options.requestType ?? 'plugin-host';
        this.responseType = options.responseType ?? 'plugin-host-result';
        this.source.addEventListener('message', this.handleMessage);
    }
    static fromWindow(options = {}) {
        const source = options.source ?? windowLike();
        const target = options.target ?? source?.parent ?? null;
        if (!source || !target)
            return null;
        return new PTPPluginViewClient({ ...options, source, target });
    }
    dispose() {
        this.source.removeEventListener('message', this.handleMessage);
        for (const [id, entry] of this.pending) {
            clearTimeout(entry.timer);
            entry.reject(new Error('Plugin view client disposed'));
            this.pending.delete(id);
        }
    }
    hasPermission(permission) {
        return hasScopedPermission(this.capabilitiesCache?.permissions ?? [], permission);
    }
    hasCapability(capability) {
        return hasNamedCapability(this.capabilitiesCache?.host_capabilities ?? [], capability);
    }
    async capabilities() {
        const data = await this.request('capabilities');
        this.capabilitiesCache = data;
        return data;
    }
    config() {
        return this.request('config.get').then((r) => r.config);
    }
    log(level, event, message, data) {
        return this.request('log.write', { level, event, message, data }).then(() => undefined);
    }
    notice(title, body, level = 'info') {
        return this.request('notice.send', { title, body, level }).then(() => undefined);
    }
    publish(type, data) {
        return this.request('event.publish', { type, data }).then(() => undefined);
    }
    kvGet(key) {
        return this.request('kv.get', { key });
    }
    kvSet(key, value) {
        return this.request('kv.set', { key, value }).then(() => undefined);
    }
    kvDelete(key) {
        return this.request('kv.delete', { key }).then(() => undefined);
    }
    sites() {
        return this.request('sites.list').then((r) => r.sites ?? []);
    }
    request(method, params) {
        const id = String(++this.sequence);
        this.target.postMessage({
            ptp: this.requestType,
            id,
            method,
            params: params ?? {},
        }, this.targetOrigin);
        return new Promise((resolve, reject) => {
            const timer = setTimeout(() => {
                this.pending.delete(id);
                reject(new Error(`Plugin view bridge timeout for ${method}`));
            }, this.timeoutMs);
            this.pending.set(id, { resolve, reject, timer });
        });
    }
    handleMessage = (event) => {
        const data = event.data;
        if (!data || typeof data !== 'object')
            return;
        const packet = data;
        if (packet.ptp !== this.responseType || !packet.id)
            return;
        if (event.source && event.source !== this.target)
            return;
        const entry = this.pending.get(packet.id);
        if (!entry)
            return;
        clearTimeout(entry.timer);
        this.pending.delete(packet.id);
        if (packet.ok) {
            entry.resolve(packet.result);
            return;
        }
        entry.reject(new Error(typeof packet.error === 'string' ? packet.error : 'Plugin bridge failed'));
    };
}
function envLike() {
    const maybeProcess = globalThis;
    return maybeProcess.process?.env ?? {};
}
function windowLike() {
    const maybeWindow = globalThis;
    if (typeof maybeWindow.addEventListener !== 'function' || typeof maybeWindow.removeEventListener !== 'function') {
        return null;
    }
    return maybeWindow;
}
function splitEnvList(value) {
    return (value || '')
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
}
function hasScopedPermission(permissions, permission) {
    return permissions.some((item) => {
        if (item === '*' || item === 'host:*' || item === 'plugin:*' || item === permission)
            return true;
        if (item.endsWith(':*'))
            return permission.startsWith(item.slice(0, -1));
        return false;
    });
}
function hasNamedCapability(capabilities, capability) {
    return capabilities.includes(capability);
}
