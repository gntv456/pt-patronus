// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// PWA Service Worker：仅做「安装到桌面」+ 基础离线壳（cache-first 静态资源，API 永不缓存）。
// 版本号变更时 bump CACHE 以触发旧缓存清理。对标 MoviePilot WebPush/PWA 体验。
const CACHE = 'ptpatronus-pwa-v1'

self.addEventListener('install', (event) => {
  // 不 precache 全量 chunk（hash 名单构建期未知），首访问运行时按需缓存。
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))),
    ).then(() => self.clients.claim()),
  )
})

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url)
  if (event.request.method !== 'GET') return
  // API 与 WS/SSE 一律直连，绝不缓存。
  if (url.pathname.startsWith('/api/')) return
  // 同源静态资源 cache-first；未命中回网络并写入缓存（生产 hashed chunk 稳定可缓存）。
  event.respondWith(
    caches.open(CACHE).then(async (cache) => {
      const hit = await cache.match(event.request)
      if (hit) return hit
      try {
        const resp = await fetch(event.request)
        if (resp.ok && url.origin === self.location.origin) {
          cache.put(event.request, resp.clone())
        }
        return resp
      } catch {
        return hit || Response.error()
      }
    }),
  )
})
