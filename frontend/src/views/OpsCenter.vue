<script setup lang="ts">
import { ref, onMounted } from 'vue'
import GlassIcon from '@/components/GlassIcon.vue'
import OpsMonitor from '@/components/ops/OpsMonitor.vue'
import OpsDocker from '@/components/ops/OpsDocker.vue'
import OpsDatabase from '@/components/ops/OpsDatabase.vue'
import OpsDisk from '@/components/ops/OpsDisk.vue'
import OpsProcess from '@/components/ops/OpsProcess.vue'
import FileManager from '@/views/FileManager.vue'

type Tab = 'monitor' | 'files' | 'docker' | 'database' | 'disk' | 'process'

const activeTab = ref<Tab>('monitor')

const tabs: { key: Tab; label: string; icon: string; from: string; to: string; desc: string }[] = [
  { key: 'monitor', label: '系统监控', icon: 'dashboard', from: '#6c5ce7', to: '#4285f4', desc: 'CPU/内存/磁盘/网络实时指标' },
  { key: 'files', label: '文件管理', icon: 'files', from: '#f7971e', to: '#ffd200', desc: '本地/NAS/云盘统一浏览' },
  { key: 'docker', label: 'Docker', icon: 'server', from: '#0f2027', to: '#2c5364', desc: '远程容器编排与管理' },
  { key: 'database', label: '数据库', icon: 'settings', from: '#11998e', to: '#38ef7d', desc: '受限查询与可视化浏览' },
  { key: 'disk', label: '磁盘管理', icon: 'harddisk', from: '#0984e3', to: '#6c5ce7', desc: '分区/格式化/挂载/卸载' },
  { key: 'process', label: '进程管理', icon: 'activity', from: '#e17055', to: '#fdcb6e', desc: '进程列表/网络连接/结束进程' },
]
</script>

<template>
  <div class="ops-center">
    <header class="ops-header">
      <h1 class="ops-title">运维中心</h1>
      <p class="ops-subtitle">系统监控 · 文件管理 · Docker 容器 · 数据库 · 磁盘管理 · 进程管理 · 六大特权运维功能</p>
    </header>

    <!-- 功能 Tab -->
    <nav class="ops-nav">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="ops-nav-item glass"
        :class="{ active: activeTab === tab.key }"
        @click="activeTab = tab.key"
      >
        <GlassIcon :name="tab.icon" :from="tab.from" :to="tab.to" :size="32" />
        <div class="ops-nav-text">
          <strong>{{ tab.label }}</strong>
          <span>{{ tab.desc }}</span>
        </div>
      </button>
    </nav>

    <!-- 内容区 -->
    <main class="ops-content">
      <KeepAlive>
        <OpsMonitor v-if="activeTab === 'monitor'" />
        <FileManager v-else-if="activeTab === 'files'" :embedded="true" />
        <OpsDocker v-else-if="activeTab === 'docker'" />
        <OpsDatabase v-else-if="activeTab === 'database'" />
        <OpsDisk v-else-if="activeTab === 'disk'" />
        <OpsProcess v-else-if="activeTab === 'process'" />
      </KeepAlive>
    </main>
  </div>
</template>

<style scoped>
.ops-center {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
  max-width: 1400px;
  margin: 0 auto;
  padding: var(--sp-4);
}
.ops-header { text-align: center; }
.ops-title {
  font-size: var(--fs-hero);
  font-weight: 700;
  margin: 0;
  background: var(--grad-primary);
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}
.ops-subtitle { font-size: var(--fs-sm); color: var(--text-muted); margin: var(--sp-1) 0 0; }

.ops-nav {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--sp-3);
}
.ops-nav-item {
  display: flex;
  align-items: center;
  gap: var(--sp-3);
  padding: var(--sp-4);
  border: 0;
  border-radius: var(--radius-md);
  cursor: pointer;
  text-align: left;
  transition: all 0.2s ease;
}
.ops-nav-item:hover {
  transform: translateY(-2px);
  box-shadow: var(--glass-hover-shadow);
}
.ops-nav-item.active {
  outline: 2px solid rgba(108, 92, 231, 0.38);
  background: rgba(108, 92, 231, 0.12);
}
.ops-nav-text { display: flex; flex-direction: column; gap: 2px; }
.ops-nav-text strong { font-size: var(--fs-md); color: var(--text-strong); }
.ops-nav-text span { font-size: var(--fs-xs); color: var(--text-muted); }

.ops-content { min-height: 400px; }

@media (max-width: 760px) {
  .ops-nav { grid-template-columns: 1fr; }
}
</style>
