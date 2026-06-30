<script setup lang="ts">
import { useRoute } from 'vue-router'
const route = useRoute()
</script>

<template>
  <div class="placeholder glass">
    <div class="ph-icon">🚧</div>
    <h2>{{ String(route.name) }}</h2>
    <p class="text-muted">该模块在后续阶段接入（站点 / 下载器 / 搜索 / 娱乐 / 文件 …）。</p>
  </div>
</template>

<style scoped>
.placeholder {
  padding: 60px 32px;
  text-align: center;
}
.ph-icon {
  font-size: 48px;
}
h2 {
  text-transform: capitalize;
  margin: 12px 0 6px;
}
</style>
