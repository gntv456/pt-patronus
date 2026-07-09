# PTPatronus Plugin SDK

TypeScript client for the PTPatronus external plugin Host API.

External plugins receive host access in two ways:

- Command plugins receive `PTP_HOST_URL` and `PTP_HOST_TOKEN` environment variables.
- Managed or remote HTTP plugins receive the same data in the `host` field of `/action` and `/event` payloads.
- Command plugins also receive `PTP_HOST_CAPABILITIES` as a comma-separated list.
- Plugin views running inside the sandboxed iframe can talk to the host through `postMessage`.

```ts
import { PTPHostClient, type ActionRequest } from '@ptpatronus/plugin-sdk'

export async function handleAction(payload: ActionRequest) {
  const host = PTPHostClient.fromPayload(payload) ?? PTPHostClient.fromEnv()
  if (!host?.hasCapability('host.api.kv.write')) {
    throw new Error('This plugin requires host KV write support')
  }
  await host?.log('info', 'demo.action', 'action started', { action: payload.action })

  const count = await host?.kvGet<number>('count')
  const next = (count?.value ?? 0) + 1
  await host?.kvSet('count', next)

  return { ok: true, output: { next } }
}
```

Supported Host API capabilities:

- `GET /capabilities`
- `log:write`
- `notice:write`
- `event:publish`
- `kv:read`, `kv:write`, `kv:delete`
- `site:read`, `site:cookie`
- `config:read`, `config:write`
- `media:search` — `host.mediaSearch(keyword, year)` reuses the host TMDB/豆瓣/寸光集 metadata chain

Use `host:*` or `*` only for trusted internal plugins.

## Plugin view bridge

Plugin views can import `PTPPluginViewClient` instead of writing `postMessage` plumbing by hand:

```ts
import { PTPPluginViewClient } from '@ptpatronus/plugin-sdk'

const host = PTPPluginViewClient.fromWindow()
if (!host) throw new Error('Plugin view host bridge is unavailable')

const caps = await host.capabilities()
if (host.hasPermission('kv:write')) {
  const current = await host.kvGet<number>('view-count')
  await host.kvSet('view-count', (current.value ?? 0) + 1)
}
```

Available plugin-view bridge helpers:

- `capabilities()`
- `config()`
- `log()`
- `notice()`
- `publish()`
- `kvGet()`, `kvSet()`, `kvDelete()`
- `sites()`
- `action(name, input?)` — invoke a declared plugin action (e.g. a dashboard fetching live data)
- `request(method, params)`

Call `dispose()` if your plugin view creates a long-lived client and later tears it down.

---

Full developer guide (scaffold/validate/test/package/publish, manifest schema, async-worker pattern,
market flow): **[DEVGUIDE.md](../DEVGUIDE.md)** — the public, host-source-free reference.
