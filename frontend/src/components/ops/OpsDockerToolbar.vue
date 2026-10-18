<script setup lang="ts">
// Docker 运维页顶部工具栏：本机/SSH 模式切换、远程主机选择、刷新、socket 提示。
// 从 OpsDocker.vue 迁出。
import { useI18n } from 'vue-i18n'

const { t } = useI18n()

defineProps<{
  mode: 'local' | 'remote'
  localAvailable: boolean
  // 后端探测给的具体原因；没给就退回泛化提示（不是所有失败都有话说）。
  localHint?: string
  selectedTarget: number
  sshTargets: Array<{ id: number; name: string }>
  loading: boolean
  error: string
}>()

const emit = defineEmits<{
  (e: 'update:selectedTarget', v: number): void
  (e: 'switchMode', m: 'local' | 'remote'): void
  (e: 'refresh'): void
}>()
</script>

<template>
  <!-- 主机选择 + 刷新 -->
  <div class="docker-toolbar">
    <div class="mode-switch">
      <button
        class="mode-btn"
        :class="{ active: mode === 'local' }"
        :disabled="!localAvailable"
        @click="emit('switchMode', 'local')"
      >
        {{ t('opsdocker.localMode') }}
      </button>
      <button class="mode-btn" :class="{ active: mode === 'remote' }" @click="emit('switchMode', 'remote')">
        {{ t('opsdocker.sshRemote') }}
      </button>
    </div>
    <el-select
      v-if="mode === 'remote'"
      :model-value="selectedTarget"
      @update:model-value="emit('update:selectedTarget', $event)"
      :placeholder="t('opsdocker.pickHost')"
      @change="emit('refresh')"
      style="width: 200px"
    >
      <el-option v-for="t in sshTargets" :key="t.id" :label="t.name" :value="t.id" />
    </el-select>
    <el-button :loading="loading" @click="emit('refresh')">{{ t('common.refresh') }}</el-button>
    <span v-if="!localAvailable && mode === 'local'" class="docker-hint">
      {{ localHint || t('opsdocker.socketHint') }}
    </span>
    <span v-if="error" class="docker-error">{{ error }}</span>
  </div>
</template>
