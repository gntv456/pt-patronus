<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 媒体海报图：豆瓣 doubanio.com 防盗链，浏览器 <img> 直连只回 1x1 占位图（且 img 不带
// JWT，无法过 /media/image 的鉴权）。故豆瓣图改由 axios（带 JWT）请求后端反代取 blob，
// 再喂给 <img>；TMDB/Cover Art 等公网 CDN 直连即可。失败/无图回退占位图标。
import { ref, watch, onUnmounted } from 'vue'
import { http } from '@/api/http'
// #405 图片 src 同样要过协议白名单：这个组件的 url 来自豆瓣/TMDB 等第三方响应
// （含 CelebrityDialog 的 avatar），外部可控。javascript:/data: 一律拒掉。
import { safeUrl } from '@/utils/safeUrl'

const props = defineProps<{ url: string; kind?: string }>()

const src = ref('')
const failed = ref(false)
let blobUrl = ''

async function resolve() {
  failed.value = false
  if (blobUrl) {
    URL.revokeObjectURL(blobUrl)
    blobUrl = ''
  }
  // #405 协议白名单在此收口：safeUrl 放过 http(s)/mailto/相对路径，其余一律拒。
  const u = safeUrl(props.url)
  if (!u) {
    src.value = ''
    failed.value = !!props.url   // 有 url 但被拒 ⇒ 显示占位，而不是静默空白
    return
  }
  // 公网 CDN 直连（不防盗链、无需鉴权）
  if (!/doubanio\.com/.test(u)) {
    src.value = u
    return
  }
  try {
    const blob: Blob = await http.get('/media/image', { params: { url: u }, responseType: 'blob' }).then((r) => r.data)
    blobUrl = URL.createObjectURL(blob)
    src.value = blobUrl
  } catch {
    src.value = ''
    failed.value = true
  }
}

watch(() => props.url, resolve, { immediate: true })
onUnmounted(() => {
  if (blobUrl) URL.revokeObjectURL(blobUrl)
})
</script>

<template>
  <img v-if="src && !failed" :src="src" alt="" loading="lazy" @error="failed = true" />
  <div v-else class="no-poster">{{ kind === 'music' ? '🎵' : kind === 'person' ? '👤' : '🎬' }}</div>
</template>

<style scoped>
img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}
.no-poster {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 40px;
}
</style>
