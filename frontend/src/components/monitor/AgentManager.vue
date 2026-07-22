<script setup lang="ts">
import { ref, watch } from 'vue'
import { agentApi, type Agent } from '@/api/monitor'
import { showAgentSecret } from '@/utils/agent-secret'
import { ElMessage, ElMessageBox } from 'element-plus'

// AgentManager — 管理已有 agent（重置 secret / 删除）。
// 创建新 agent 统一走「+ 添加设备」选「Agent 上报」采集方式（见 NasServer.onSubmit）。
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [] }>()

const agents = ref<Agent[]>([])

async function load() {
  try {
    agents.value = await agentApi.list()
  } catch {
    /* ignore */
  }
}
watch(visible, (v) => {
  if (v) load()
})

async function remove(a: Agent) {
  try {
    await ElMessageBox.confirm(`删除 agent「${a.name}」？关联设备与历史数据一并删除。`, '删除 Agent', {
      type: 'warning',
    })
    await agentApi.remove(a.id)
    ElMessage.success('已删除')
    await load()
    emit('changed')
  } catch {
    /* 取消 */
  }
}

async function rotate(a: Agent) {
  try {
    await ElMessageBox.confirm(
      `重置「${a.name}」的 secret？旧 secret 立即失效，需用新 secret 重启 agent。`,
      '重置 Secret',
      { type: 'warning' },
    )
    const r = await agentApi.rotate(a.id)
    showAgentSecret(a, r.plain_secret)
    await load()
  } catch {
    /* 取消 */
  }
}

function fmtTime(s: string | null): string {
  return s ? new Date(s).toLocaleString() : '-'
}
</script>

<template>
  <el-dialog v-model="visible" title="Agent 管理" width="760px">
    <div class="am-bar">
      <span class="am-hint">添加新 agent 请用顶部「+ 添加设备」选「Agent 上报」采集方式。这里只管理已有 agent。</span>
    </div>
    <el-table :data="agents" empty-text="暂无 Agent（用「+ 添加设备」添加）" size="small">
      <el-table-column prop="name" label="名称" />
      <el-table-column label="主机名" width="130">
        <template #default="{ row }">{{ row.hostname || '-' }}</template>
      </el-table-column>
      <el-table-column label="IP" width="130">
        <template #default="{ row }">{{ row.ip || '-' }}</template>
      </el-table-column>
      <el-table-column label="状态" width="80">
        <template #default="{ row }">
          <span :class="['am-dot', row.online ? 'on' : 'off']">●</span>
          {{ row.online ? '在线' : '离线' }}
        </template>
      </el-table-column>
      <el-table-column label="最后上报" width="150">
        <template #default="{ row }">{{ fmtTime(row.last_seen_at) }}</template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" text @click="rotate(row as Agent)">重置 Secret</el-button>
          <el-button size="small" text type="danger" @click="remove(row as Agent)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>
  </el-dialog>
</template>

<style scoped>
.am-bar {
  margin-bottom: 12px;
}
.am-hint {
  font-size: 12px;
  color: var(--text-muted);
}
.am-dot.on {
  color: #11998e;
}
.am-dot.off {
  color: #e74c3c;
}
</style>
