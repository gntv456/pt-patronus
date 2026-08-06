<script setup lang="ts">
import GlassIcon from '@/components/GlassIcon.vue'

defineProps<{
  activeWorkspace: string
  hostCount: number
  monitorCount: number
  sshTargetCount: number
  credentialCount: number
  configCount: number
  activeAlertCount: number
}>()

const emit = defineEmits<{
  (e: 'workspace', v: string): void
  (e: 'execute'): void
  (e: 'go', name: string): void
  (e: 'deploy'): void
  (e: 'credentials'): void
  (e: 'configs'): void
  (e: 'monitor'): void
}>()
</script>

<template>
  <section class="ops-grid">
    <button
      class="ops-tile glass glass-hover"
      :class="{ active: activeWorkspace === 'asset' }"
      type="button"
      @click="emit('workspace', 'asset')"
    >
      <GlassIcon name="server" from="#0f2027" to="#2c5364" :size="24" />
      <strong>{{ hostCount }}</strong>
      <span>资产管理</span>
    </button>
    <button
      class="ops-tile glass glass-hover"
      :class="{ active: activeWorkspace === 'execute' }"
      type="button"
      @click="emit('execute')"
    >
      <GlassIcon name="server" from="#34495e" to="#16a085" :size="24" />
      <strong>终端</strong>
      <span>在线执行</span>
    </button>
    <button
      class="ops-tile glass glass-hover"
      :class="{ active: activeWorkspace === 'execute' }"
      type="button"
      @click="emit('execute')"
    >
      <GlassIcon name="workflow" from="#11998e" to="#38ef7d" :size="24" />
      <strong>{{ sshTargetCount }}</strong>
      <span>批量执行</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('go', 'files')">
      <GlassIcon name="files" from="#4285f4" to="#6c5ce7" :size="24" />
      <strong>文件</strong>
      <span>上传下载</span>
    </button>
    <button
      class="ops-tile glass glass-hover"
      :class="{ active: activeWorkspace === 'deploy' }"
      type="button"
      @click="emit('deploy')"
    >
      <GlassIcon name="workflow" from="#f7971e" to="#ffd200" :size="24" />
      <strong>发布</strong>
      <span>流程 / 回滚</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('go', 'workflow')">
      <GlassIcon name="history" from="#00b894" to="#0984e3" :size="24" />
      <strong>任务</strong>
      <span>Cron 计划</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('credentials')">
      <GlassIcon name="settings" from="#8e44ad" to="#3498db" :size="24" />
      <strong>{{ credentialCount }}</strong>
      <span>凭据中心</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('configs')">
      <GlassIcon name="files" from="#4285f4" to="#16a085" :size="24" />
      <strong>{{ configCount }}</strong>
      <span>配置中心</span>
    </button>
    <button
      class="ops-tile glass glass-hover"
      :class="{ active: activeWorkspace === 'monitor' }"
      type="button"
      @click="emit('monitor')"
    >
      <GlassIcon name="dashboard" from="#6c5ce7" to="#4285f4" :size="24" />
      <strong>{{ monitorCount }}</strong>
      <span>监控中心</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('go', 'ops-center')">
      <GlassIcon name="dashboard" from="#11998e" to="#38ef7d" :size="24" />
      <strong>运维</strong>
      <span>监控/Docker/DB</span>
    </button>
    <button class="ops-tile glass glass-hover" type="button" @click="emit('go', 'settings')">
      <GlassIcon name="alert" from="#e74c3c" to="#c0392b" :size="24" />
      <strong>{{ activeAlertCount }}</strong>
      <span>报警中心</span>
    </button>
  </section>
</template>

<style scoped src="../../views/nas-shared/opsGrid.css"></style>
