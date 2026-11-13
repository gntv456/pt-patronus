<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, watch } from 'vue'
import { agentApi, type Agent } from '@/api/monitor'
import { showAgentSecret } from '@/utils/agent-secret'
import { ElMessage, ElMessageBox } from 'element-plus'

// AgentManager — 管理已有 agent（重置 secret / 删除）。
// 创建新 agent 统一走「+ 添加设备」选「Agent 上报」采集方式（见 NasServer.onSubmit）。
const { t } = useI18n()
const visible = defineModel<boolean>('visible', { default: false })
const emit = defineEmits<{ changed: [] }>()

const agents = ref<Agent[]>([])
const latestVersion = ref('')
const upgrading = ref<number | null>(null)

async function load() {
  try {
    const [list, lv] = await Promise.all([agentApi.list(), agentApi.latestAgentVersion().catch(() => null)])
    agents.value = list
    if (lv) latestVersion.value = lv.version
  } catch (e: unknown) {
    // 从前是 catch { /* ignore */ }：列表没读到就继续显示上一次/空列表，
    // 和「这台主机真没有 agent」长得一样（面板那句 empty 会被人当成事实）。
    ElMessage.error((e as Error).message || t('am.loadFailed'))
  }
}

async function upgradeAgent(a: Agent) {
  try {
    await ElMessageBox.confirm(t('am.upgradeConfirm', { name: a.name }), t('am.upgradeTitle'), { type: 'info' })
  } catch {
    return
  }
  upgrading.value = a.id
  try {
    const r = await agentApi.upgrade(a.id)
    if (!r.ok) {
      ElMessage.error(r.error || t('am.upgradeFailed'))
      return
    }
    ElMessage.success(t('am.upgradeSent'))
    setTimeout(() => load(), 12000)
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || t('am.upgradeFailed'))
  } finally {
    upgrading.value = null
  }
}

function needUpgrade(a: Agent): boolean {
  return !!latestVersion.value && !!a.version && a.version !== latestVersion.value && !!a.online
}
watch(visible, (v) => {
  if (v) load()
})

async function remove(a: Agent) {
  try {
    await ElMessageBox.confirm(t('am.removeConfirm', { name: a.name }), t('am.removeTitle'), {
      type: 'warning',
    })
  } catch {
    return // 用户取消
  }
  try {
    await agentApi.remove(a.id)
    ElMessage.success(t('common.deleted'))
    await load()
    emit('changed')
  } catch (e: unknown) {
    // 从前删除与「取消」共用一个 catch { /* 取消 */ }：删失败（后端 4xx 有中文原因）
    // 在界面上和「用户自己取消了」完全一样——点了没反应（#206/#268 那族）。
    ElMessage.error((e as Error).message || t('am.removeFailed'))
  }
}

async function rotate(a: Agent) {
  try {
    await ElMessageBox.confirm(t('am.rotateConfirm', { name: a.name }), t('am.rotateTitle'), { type: 'warning' })
  } catch {
    return // 用户取消
  }
  try {
    const r = await agentApi.rotate(a.id)
    showAgentSecret(a, r.plain_secret)
    await load()
  } catch (e: unknown) {
    // 重置 secret 是有副作用的写操作：旧 secret 是否已失效必须说得出，
    // 静默成「取消」会让人以为没动过，继续拿旧 secret 重启 agent。
    ElMessage.error((e as Error).message || t('am.rotateFailed'))
  }
}

function fmtTime(s: string | null): string {
  return s ? new Date(s).toLocaleString() : '-'
}
</script>

<template>
  <el-dialog v-model="visible" :title="t('am.title')" width="760px">
    <div class="am-bar">
      <span class="am-hint">{{ t('am.hint') }}</span>
      <span v-if="latestVersion" class="am-latest">{{ t('am.latest') }}{{ latestVersion }}</span>
    </div>
    <el-table :data="agents" :empty-text="t('am.empty')" size="small">
      <el-table-column prop="name" :label="t('common.name')" />
      <el-table-column :label="t('am.hostname')" width="130">
        <template #default="{ row }">{{ row.hostname || '-' }}</template>
      </el-table-column>
      <el-table-column label="IP" width="130">
        <template #default="{ row }">{{ row.ip || '-' }}</template>
      </el-table-column>
      <el-table-column :label="t('am.version')" width="90">
        <template #default="{ row }">
          <span>{{ row.version || '-' }}</span>
          <el-tag v-if="needUpgrade(row as Agent)" size="small" type="warning" style="margin-left: 4px">{{
            t('am.canUpgrade')
          }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column :label="t('am.status')" width="80">
        <template #default="{ row }">
          <span :class="['am-dot', row.online ? 'on' : 'off']">●</span>
          {{ row.online ? t('dc.online') : t('dc.offline') }}
        </template>
      </el-table-column>
      <el-table-column :label="t('am.lastSeen')" width="150">
        <template #default="{ row }">{{ fmtTime(row.last_seen_at) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.action')" width="140">
        <template #default="{ row }">
          <el-button size="small" text @click="rotate(row as Agent)">{{ t('am.rotateTitle') }}</el-button>
          <el-button
            v-if="row.online"
            size="small"
            text
            type="success"
            :loading="upgrading === row.id"
            @click="upgradeAgent(row as Agent)"
            >{{ t('am.upgradeBtn') }}</el-button
          >
          <el-button size="small" text type="danger" @click="remove(row as Agent)">{{ t('common.delete') }}</el-button>
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
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.am-dot.on {
  color: var(--pt-success-fg);
}
.am-latest {
  font-size: var(--fs-xs);
  color: var(--pt-primary);
  margin-left: 12px;
}
.am-dot.off {
  color: var(--pt-danger);
}
</style>
