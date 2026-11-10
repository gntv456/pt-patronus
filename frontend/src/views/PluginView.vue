<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { ArrowLeft, Refresh } from '@element-plus/icons-vue'
import { pluginApi, type PluginInfo } from '@/api/plugin'

const route = useRoute()
const router = useRouter()
const plugin = ref<PluginInfo | null>(null)
const html = ref('')
const loading = ref(false)
const frame = ref<HTMLIFrameElement | null>(null)

const pluginId = computed(() => String(route.params.pluginId || ''))
const viewId = computed(() => String(route.params.viewId || ''))
const view = computed(() => (plugin.value?.contributes?.views || []).find((item) => item.id === viewId.value))
const title = computed(() => view.value?.title || plugin.value?.name || 'Plugin View')

function assetPath(path?: string) {
  const clean = (path || 'index.html').replace(/^\/+/, '')
  return clean.startsWith('web/') ? clean.slice(4) : clean
}

function assetBaseURL(path?: string) {
  const clean = assetPath(path)
  const parts = clean.split('/').filter(Boolean)
  if (!parts.length) {
    return `/api/v1/plugins/${encodeURIComponent(pluginId.value)}/assets/`
  }
  const file = parts[parts.length - 1] || ''
  const hasExtension = /\.[A-Za-z0-9]+$/.test(file)
  const dirParts = hasExtension ? parts.slice(0, -1) : parts
  const basePath = dirParts.map((item) => encodeURIComponent(item)).join('/')
  const suffix = basePath ? `${basePath}/` : ''
  return `/api/v1/plugins/${encodeURIComponent(pluginId.value)}/assets/${suffix}`
}

function bridgeScript() {
  // 桥接脚本运行在 sandbox iframe（无 allow-same-origin）里，其 origin 是 "null"——
  // 因此插件侧的 result 监听只能放行 event.origin === 'null'（宿主回帖本就带该 origin）。
  // 不校验 origin 的话，任何被注入到宿主页面的脚本都能伪造 plugin-host-result
  // 应答，污染/劫持插件的桥调用结果。
  return (
    `<script>
(function () {
  if (window.PTPatronus) return;
  var seq = 0;
  var pending = {};
  window.addEventListener('message', function (event) {
    if (event.origin !== 'null') return;
    var data = event.data || {};
    if (data.ptp !== 'plugin-host-result' || !data.id || !pending[data.id]) return;
    var task = pending[data.id];
    delete pending[data.id];
    if (data.ok) task.resolve(data.result || {});
    else task.reject(new Error(data.error || 'Plugin bridge failed'));
  });
  function request(method, params) {
    var id = String(++seq);
    window.parent.postMessage({ ptp: 'plugin-host', id: id, method: method, params: params || {} }, '*');
    return new Promise(function (resolve, reject) {
      pending[id] = { resolve: resolve, reject: reject };
      setTimeout(function () {
        if (!pending[id]) return;
        delete pending[id];
        reject(new Error('Plugin bridge timeout'));
      }, 30000);
    });
  }
  window.PTPatronus = {
    request: request,
    capabilities: function () { return request('capabilities'); },
    config: {
      get: function () { return request('config.get').then(function (result) { return result.config || {}; }); }
    },
    log: function (level, event, message, data) { return request('log.write', { level: level, event: event, message: message, data: data }); },
    notice: function (title, body, level) { return request('notice.send', { title: title, body: body, level: level || 'info' }); },
    publish: function (type, data) { return request('event.publish', { type: type, data: data }); },
    kv: {
      get: function (key) { return request('kv.get', { key: key }); },
      set: function (key, value) { return request('kv.set', { key: key, value: value }); },
      delete: function (key) { return request('kv.delete', { key: key }); }
    },
    sites: {
      list: function () { return request('sites.list').then(function (result) { return result.sites || []; }); }
    },
    action: function (name, input) { return request('action.invoke', { action: name, input: input || {} }); }
  };
})();
</` + `script>`
  )
}

function injectIntoHead(source: string, tag: string) {
  const baseRe = /<head[^>]*>/i
  if (baseRe.test(source)) {
    return source.replace(baseRe, (match) => `${match}\n${tag}`)
  }
  return `${tag}\n${source}`
}

function injectBridge(source: string, baseURL: string) {
  const base = `<base href="${baseURL}">`
  const script = bridgeScript()
  const withBase = /<base\b/i.test(source) ? source.replace(/<base\b[^>]*>/i, base) : injectIntoHead(source, base)
  return injectIntoHead(withBase, script)
}

async function handleBridgeMessage(event: MessageEvent) {
  // 双重校验（收权加固）：
  //  1. event.source 必须是插件 iframe 的 contentWindow（原有校验）
  //  2. event.origin 必须是 'null'——sandbox 无 allow-same-origin 的 srcdoc iframe
  //     的 origin 恒为 "null"；任何其它 origin（宿主页面被注入的脚本伪造成别的窗口）
  //     一律拒绝，防止第三方页面/窗口伪造 plugin-host 调用桥 API。
  if (!frame.value?.contentWindow || event.source !== frame.value.contentWindow) return
  if (event.origin !== 'null') return
  const data = event.data || {}
  if (data.ptp !== 'plugin-host' || !data.id) return
  const target = event.source as Window
  try {
    const result = await pluginApi.bridge(pluginId.value, String(data.method || ''), data.params || {})
    target.postMessage({ ptp: 'plugin-host-result', id: data.id, ok: true, result }, event.origin)
  } catch (e: unknown) {
    target.postMessage(
      { ptp: 'plugin-host-result', id: data.id, ok: false, error: (e as Error).message || 'Bridge failed' },
      event.origin,
    )
  }
}

async function load() {
  loading.value = true
  try {
    plugin.value = await pluginApi.get(pluginId.value)
    if (!view.value) {
      html.value = ''
      ElMessage.warning('Plugin view not found')
      return
    }
    const [, source] = await Promise.all([
      pluginApi.assetTicket(pluginId.value),
      pluginApi.asset(pluginId.value, assetPath(view.value.path)),
    ])
    html.value = injectBridge(source, assetBaseURL(view.value.path))
  } catch (e: unknown) {
    html.value = ''
    ElMessage.error((e as Error).message || 'Load plugin view failed')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  window.addEventListener('message', handleBridgeMessage)
  load()
})
onBeforeUnmount(() => window.removeEventListener('message', handleBridgeMessage))
</script>

<template>
  <div class="page">
    <div class="head">
      <div class="title-block">
        <el-button :icon="ArrowLeft" circle @click="router.push({ name: 'plugins' })" />
        <div>
          <h2 class="title">{{ title }}</h2>
          <div class="meta">{{ pluginId }} / {{ viewId }}</div>
        </div>
      </div>
      <el-button :icon="Refresh" :loading="loading" @click="load">Refresh</el-button>
    </div>

    <div v-loading="loading" class="frame-shell">
      <iframe
        v-if="html"
        :key="`${pluginId}:${viewId}:${html.length}`"
        ref="frame"
        class="plugin-frame"
        :srcdoc="html"
        sandbox="allow-scripts allow-forms allow-popups allow-modals"
        referrerpolicy="no-referrer"
      />
      <el-empty v-else description="No view" />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  min-height: calc(100vh - 128px);
  flex-direction: column;
  gap: var(--sp-3);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
}
.title-block {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: var(--sp-3);
}
.title {
  margin: 0;
  font-size: var(--fs-title);
}
.meta {
  margin-top: 3px;
  color: var(--text-muted);
  font-size: var(--fs-sm);
}
.frame-shell {
  min-height: 560px;
  flex: 1;
  overflow: hidden;
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  background: #fff;
}
.plugin-frame {
  display: block;
  width: 100%;
  height: 100%;
  min-height: 560px;
  border: 0;
  background: #fff;
}
@media (max-width: 720px) {
  .head {
    align-items: stretch;
    flex-direction: column;
  }
  .title {
    font-size: var(--fs-xl);
  }
  .frame-shell,
  .plugin-frame {
    min-height: 70vh;
  }
}
</style>
