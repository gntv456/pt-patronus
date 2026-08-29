<script setup lang="ts">
// 媒体海报图：豆瓣 doubanio.com 防盗链，浏览器 <img> 直连只回 1x1 占位图（且 img 不带
// JWT，无法过 /media/image 的鉴权）。故豆瓣图改由 axios（带 JWT）请求后端反代取 blob，
// 再喂给 <img>；TMDB/Cover Art 等公网 CDN 直连即可。失败/无图回退占位图标。
import { ref, watch, onUnmounted } from 'vue'
import { http } from '@/api/http'

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
  const u = props.url
  if (!u) {
    src.value = ''
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
