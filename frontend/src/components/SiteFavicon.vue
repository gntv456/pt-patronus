<script setup lang="ts">
import { computed, ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    name: string
    baseUrl?: string
    size?: number
  }>(),
  { baseUrl: '', size: 28 },
)

const failed = ref(false)
const faviconUrl = computed(() => {
  const raw = props.baseUrl.trim()
  if (!raw) return ''
  try {
    const parsed = new URL(raw)
    if (!['http:', 'https:'].includes(parsed.protocol)) return ''
    return `${raw.replace(/\/$/, '')}/favicon.ico`
  } catch {
    return ''
  }
})
const initial = computed(() => (props.name.trim()[0] || '?').toUpperCase())
const fallbackColor = computed(() => {
  let hash = 0
  for (const ch of props.name) hash = (hash * 31 + ch.charCodeAt(0)) % 360
  return `hsl(${hash}, 46%, 46%)`
})
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(faviconUrl, () => {
  failed.value = false
})
</script>

<template>
  <span class="site-favicon" :style="boxStyle">
    <img
      v-if="faviconUrl && !failed"
      :src="faviconUrl"
      :alt="`${name} Logo`"
      loading="lazy"
      @error="failed = true"
    />
    <span v-else class="site-favicon-fallback" :style="{ backgroundColor: fallbackColor }">
      {{ initial }}
    </span>
  </span>
</template>

<style scoped>
.site-favicon {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid var(--divider);
  border-radius: 6px;
  background: var(--surface-muted);
}
.site-favicon img,
.site-favicon-fallback {
  width: 100%;
  height: 100%;
}
.site-favicon img {
  display: block;
  object-fit: contain;
}
.site-favicon-fallback {
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 12px;
  font-weight: 700;
  line-height: 1;
}
</style>
