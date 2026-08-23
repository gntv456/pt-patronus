<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import { authApi } from '@/api/auth'

// RtspPlayer — jessibuca(wasm) 播放器封装：动态加载 public/jessibuca/ 资源，连后端 RTSP→WS 代理播放。
const props = defineProps<{ target: MonitorTarget }>()
const containerRef = ref<HTMLDivElement>()
const err = ref('')
type JessibucaPlayer = { destroy: () => void; play: (u: string) => void }
let player: JessibucaPlayer | null = null

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const w = window as unknown as { Jessibuca?: unknown; JessibucaPro?: unknown }
    if (w.Jessibuca || w.JessibucaPro) return resolve()
    const s = document.createElement('script')
    s.src = src
    s.onload = () => resolve()
    s.onerror = () => reject(new Error('jessibuca 资源加载失败，请确认 public/jessibuca/ 已部署'))
    document.head.appendChild(s)
  })
}

async function start() {
  if (!props.target.rtsp_url) {
    err.value = '该摄像头未配置 RTSP 地址'
    return
  }
  try {
    await loadScript('/jessibuca/jessibuca.js')
    const w = window as unknown as { Jessibuca?: new (o: object) => JessibucaPlayer; JessibucaPro?: new (o: object) => JessibucaPlayer }
    const Ctor = w.JessibucaPro || w.Jessibuca
    if (!Ctor) throw new Error('jessibuca 未就绪')
    const p = new Ctor({
      container: containerRef.value,
      videoBuffer: 0.2,
      isResize: true,
      loadingText: '连接摄像头中...',
      decoder: '/jessibuca/decoder.js',
    })
    player = p
    const { ticket } = await authApi.wsTicket()
    p.play(monitorApi.streamURL(props.target.id, ticket))
  } catch (e) {
    err.value = (e as Error).message
  }
}

onMounted(start)
onBeforeUnmount(() => {
  try {
    player?.destroy()
  } catch {
    /* ignore */
  }
  player = null
})
</script>

<template>
  <div class="rtsp-wrap">
    <div ref="containerRef" class="rtsp-player"></div>
    <div v-if="err" class="rtsp-err">{{ err }}</div>
  </div>
</template>

<style scoped>
.rtsp-wrap {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  background: #000;
  border-radius: var(--radius-md);
  overflow: hidden;
}
.rtsp-player {
  width: 100%;
  height: 100%;
}
.rtsp-err {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: var(--fs-body);
  text-align: center;
  padding: 16px;
  background: rgba(0, 0, 0, 0.6);
}
</style>
