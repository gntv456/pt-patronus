<script setup lang="ts">
// SiteFavicon — site logo/avatar with multi-source fallback.
// Tries base/favicon.ico → origin icons → public resolvers → letter block.
import { computed, ref, watch } from 'vue'
import { faviconCandidates } from '@/utils/siteFavicon'

const props = withDefaults(
  defineProps<{
    name?: string
    baseUrl?: string
    size?: number
  }>(),
  { name: '', baseUrl: '', size: 28 },
)

const tryIndex = ref(0)
const candidates = computed(() => faviconCandidates(props.baseUrl))
const currentUrl = computed(() => candidates.value[tryIndex.value] || '')
const showImage = computed(() => Boolean(currentUrl.value))
const initial = computed(() => (props.name.trim()[0] || '?').toUpperCase())
const fallbackColor = computed(() => {
  let hash = 0
  for (const ch of props.name) hash = (hash * 31 + ch.charCodeAt(0)) % 360
  return `hsl(${hash}, 46%, 46%)`
})
const boxStyle = computed(() => ({ width: `${props.size}px`, height: `${props.size}px` }))

watch(
  () => props.baseUrl,
  () => {
    tryIndex.value = 0
  },
)

function onImgError() {
  // Advance to next candidate; when exhausted, currentUrl becomes '' and letter shows.
  if (tryIndex.value < candidates.value.length) {
    tryIndex.value += 1
  }
}
</script>

<template>
  <span class="site-favicon" :style="boxStyle">
    <img
      v-if="showImage"
      :key="currentUrl"
      :src="currentUrl"
      :alt="`${name} Logo`"
      loading="lazy"
      decoding="async"
      referrerpolicy="no-referrer"
      @error="onImgError"
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
