<script setup lang="ts">
// 统一骨架占位：固定高度避免 CLS（skill: content-jumping / progressive-loading）。
withDefaults(
  defineProps<{
    height?: string
    width?: string
    radius?: string
    label?: string
  }>(),
  { height: '36px', width: '100%', radius: 'var(--radius-control)' },
)
</script>

<template>
  <div
    class="pt-skeleton"
    role="status"
    aria-busy="true"
    :aria-label="label"
    :style="{ height, width, borderRadius: radius }"
  />
</template>
