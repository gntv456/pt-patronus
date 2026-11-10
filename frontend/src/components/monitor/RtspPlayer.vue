<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { monitorApi, type MonitorTarget } from '@/api/monitor'
import { authApi } from '@/api/auth'
import { assetsReady, assetUrl } from '@/utils/jessibucaAssets'

// RtspPlayer — jessibuca(wasm) 播放器封装：动态加载 public/jessibuca/ 资源，连后端 RTSP→WS 代理播放。
const { t } = useI18n()
const props = defineProps<{ target: MonitorTarget }>()
const containerRef = ref<HTMLDivElement>()
const err = ref('')
// 资源尚未就位（首次使用，后端正在按需下载）：这不是故障，不该走重连退避。
// 直接继续发起播放请求即可——后端会等下载完成再响应，这里只是先给个「准备中」提示。
const assetsPending = ref(false)
type JessibucaPlayer = {
  destroy: () => void
  play: (u: string) => void
  // 事件 API 因捆绑版本而异：仅在存在时注册（断流重连），缺失则退化为手动刷新。
  on?: (ev: string, cb: (msg?: unknown) => void) => void
}
let player: JessibucaPlayer | null = null
// 断流重连：error/timeout 事件触发有界退避重连（每次重建播放器并取新 ticket，
// 旧 ticket 单次有效）。无成功回调可依据，故每个挂载实例最多重连 5 次，超限提示刷新。
let stopped = false
let reconnects = 0
let reconnectTimer: number | undefined
const maxReconnects = 5

function scheduleReconnect() {
  if (stopped) return
  if (reconnects >= maxReconnects) {
    err.value = t('rp.giveUp')
    return
  }
  reconnects++
  err.value = t('rp.reconnecting')
  window.clearTimeout(reconnectTimer)
  reconnectTimer = window.setTimeout(
    () => {
      if (stopped) return
      try {
        player?.destroy()
      } catch {
        /* ignore */
      }
      player = null
      start()
    },
    Math.min(2000 * reconnects, 10000),
  )
}

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const w = window as unknown as { Jessibuca?: unknown; JessibucaPro?: unknown }
    if (w.Jessibuca || w.JessibucaPro) return resolve()
    const s = document.createElement('script')
    s.src = src
    s.onload = () => resolve()
    s.onerror = () => reject(new Error(t('rp.loadFailed')))
    document.head.appendChild(s)
  })
}

async function start() {
  if (!props.target.rtsp_url) {
    err.value = t('rp.noRtsp')
    return
  }
  // 先探一次只为显示状态，不阻断：资源不在镜像里，由后端按需下载，
  // 首次播放请求会阻塞到下载完成为止。探测失败也照常发起播放。
  assetsPending.value = !(await assetsReady())
  err.value = assetsPending.value ? t('rp.assetsPending') : ''
  try {
    await loadScript(assetUrl('jessibuca.js'))
    const w = window as unknown as {
      Jessibuca?: new (o: object) => JessibucaPlayer
      JessibucaPro?: new (o: object) => JessibucaPlayer
    }
    const Ctor = w.JessibucaPro || w.Jessibuca
    if (!Ctor) throw new Error(t('rp.notReady'))
    const p = new Ctor({
      container: containerRef.value,
      videoBuffer: 0.2,
      isResize: true,
      loadingText: t('rp.connecting'),
      decoder: assetUrl('decoder.js'),
      timeout: 15,
    })
    player = p
    if (typeof p.on === 'function') {
      p.on('error', () => scheduleReconnect())
      p.on('timeout', () => scheduleReconnect())
    }
    const { ticket } = await authApi.wsTicket()
    p.play(monitorApi.streamURL(props.target.id, ticket))
  } catch (e) {
    err.value = (e as Error).message
    scheduleReconnect()
  }
}

onMounted(start)
onBeforeUnmount(() => {
  stopped = true
  window.clearTimeout(reconnectTimer)
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
    <div v-if="err" class="rtsp-err">
      <template v-if="assetsPending">
        <p class="rtsp-err-main">{{ t('rp.assetsPending') }}</p>
        <p class="rtsp-err-note">{{ t('rp.assetsPendingHint') }}</p>
      </template>
      <template v-else>{{ err }}</template>
    </div>
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
.rtsp-err-main {
  font-weight: 600;
  margin-bottom: 6px;
}
.rtsp-err-note {
  font-size: var(--fs-xs);
  opacity: 0.85;
  margin: 4px 0;
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
