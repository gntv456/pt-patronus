<script setup lang="ts">
// 根组件：路由出口。深浅色主题在 main.ts 之前可按 localStorage 切换。
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import en from 'element-plus/es/locale/lang/en'

const { locale } = useI18n()
// element-plus 内置组件文案跟随应用 locale（ElConfigProvider 只需挂根上一次）
const elLocale = computed(() => (locale.value === 'en-US' ? en : zhCn))

const dark = localStorage.getItem('theme') === 'dark'
if (dark) document.documentElement.classList.add('dark')
</script>

<template>
  <el-config-provider :locale="elLocale">
    <router-view />
  </el-config-provider>
</template>
