<script setup lang="ts">
// 根组件：路由出口。深浅色主题在 main.ts 之前可按 localStorage 切换。
const dark = localStorage.getItem('theme') === 'dark'
if (dark) document.documentElement.classList.add('dark')
</script>

<template>
  <router-view />
</template>
