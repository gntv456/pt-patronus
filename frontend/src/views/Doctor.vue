<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { systemApi, type DoctorReport } from '@/api/system'

const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const report = ref<DoctorReport | null>(null)
const loading = ref(false)
const fixing = ref(false)

const sevColor: Record<string, string> = { info: '#52c41a', warn: '#faad14', error: '#ff4d4f' }
const statusType: Record<string, 'success' | 'info' | 'warning' | 'danger'> = {
  ok: 'success', skipped: 'info', degraded: 'warning', failed: 'danger', fixed: 'success',
}

async function run(fix: boolean) {
  if (fix) fixing.value = true
  else loading.value = true
  try {
    report.value = fix ? await systemApi.fixDoctor() : await systemApi.runDoctor()
  } catch {
    ElMessage.error('诊断失败')
  } finally {
    loading.value = false
    fixing.value = false
  }
}

onMounted(() => run(false))
</script>

<template>
  <div class="doctor-wrap" :class="{ embedded: props.embedded }">
    <div class="doctor-head">
      <div>
        <h2 v-if="!props.embedded">诊断</h2>
        <h3 v-else class="title embedded-title">诊断</h3>
        <p v-if="report" class="text-muted">
          状态：
          <el-tag :type="report.status === 'healthy' ? 'success' : report.status === 'degraded' ? 'warning' : 'danger'">
            {{ report.status }}
          </el-tag>
          · 健康评分 <b>{{ report.score ?? '-' }}</b>/100
          · 共 {{ report.summary.total }} 项（info {{ report.summary.info }} / warn {{ report.summary.warn }} / error {{ report.summary.error }}）
        </p>
      </div>
      <div class="doctor-actions">
        <el-button :loading="loading" @click="run(false)">重新诊断</el-button>
        <el-button type="primary" :loading="fixing" @click="run(true)">尝试修复</el-button>
      </div>
    </div>

    <el-alert
      v-if="report?.environment?.safe_mode"
      type="warning"
      :closable="false"
      title="SAFE_MODE 已启用"
      description="插件 / 调度 / 监控未激活，仅核心 HTTP + 诊断可达。修复后取消 SAFE_MODE 重启恢复。"
    />

    <div v-loading="loading" class="findings">
      <div v-for="f in report?.findings ?? []" :key="f.id" class="card glass finding">
        <div class="f-head">
          <span class="dot" :style="{ background: sevColor[f.severity] }" />
          <span class="f-title">{{ f.title }}</span>
          <el-tag size="small" :type="statusType[f.status] ?? 'info'">{{ f.status }}</el-tag>
          <span class="f-id text-muted">{{ f.id }}</span>
        </div>
        <div v-if="f.detail" class="f-detail">{{ f.detail }}</div>
        <div v-if="f.recommendation" class="f-rec">建议：{{ f.recommendation }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.doctor-wrap { display: flex; flex-direction: column; gap: var(--sp-3); }
.doctor-head { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-3); flex-wrap: wrap; }
.doctor-actions { display: flex; gap: var(--sp-2); }
.findings { display: flex; flex-direction: column; gap: var(--sp-2); }
.finding { padding: 12px 14px; }
.f-head { display: flex; align-items: center; gap: var(--sp-2); }
.dot { width: 10px; height: 10px; border-radius: 50%; display: inline-block; }
.f-title { font-weight: 600; }
.f-id { margin-left: auto; font-size: var(--fs-xs); }
.f-detail { margin-top: 6px; font-size: var(--fs-body); }
.f-rec { margin-top: 4px; font-size: var(--fs-xs); opacity: 0.7; }
.doctor-wrap.embedded { padding: 0; }
.doctor-wrap.embedded .embedded-title { font-size: var(--fs-lg); font-weight: 600; margin: 0; }

/* 诊断结果 staggered 入场 */
.findings > * { animation: pt-fade-in-up var(--dur-base) var(--ease-out-smooth) both; }
.findings > *:nth-child(1) { animation-delay: 0ms; }
.findings > *:nth-child(2) { animation-delay: 60ms; }
.findings > *:nth-child(3) { animation-delay: 120ms; }
.findings > *:nth-child(4) { animation-delay: 180ms; }
.findings > *:nth-child(5) { animation-delay: 240ms; }
.findings > *:nth-child(n+6) { animation-delay: 300ms; }
.finding { transition: transform var(--dur-fast) var(--ease-out-smooth); }
.finding:hover { transform: translateX(2px); }
</style>
