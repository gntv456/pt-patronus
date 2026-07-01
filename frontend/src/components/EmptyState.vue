<script setup lang="ts">
// 统一空态：图标 + 主文案 + 副文案 + 引导操作（插槽）。
// 替代各页干瘪的「暂无数据」纯文字，给用户清晰的「为什么空 + 下一步做什么」。
withDefaults(
  defineProps<{
    icon?: string
    title?: string
    description?: string
  }>(),
  { icon: '📭', title: '暂无数据', description: '' },
)
</script>

<template>
  <div class="empty-state">
    <div class="es-icon">{{ icon }}</div>
    <div v-if="title" class="es-title">{{ title }}</div>
    <div v-if="description" class="es-desc">{{ description }}</div>
    <div v-if="$slots.default" class="es-actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-2);
  padding: var(--sp-10) var(--sp-4);
  text-align: center;
}
.es-icon {
  font-size: 40px;
  line-height: 1;
  opacity: 0.7;
}
.es-title {
  font-size: var(--fs-md);
  font-weight: 600;
  color: var(--text-strong);
}
.es-desc {
  font-size: var(--fs-sm);
  color: var(--text-muted);
  max-width: 340px;
  line-height: 1.5;
}
.es-actions {
  margin-top: var(--sp-2);
  display: flex;
  gap: var(--sp-2);
}
</style>
