<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { assessmentApi } from '@/api/assessment'
import { brushflowApi } from '@/api/brushflow'
import { saveptApi } from '@/api/savept'
import { reseedApi } from '@/api/reseed'

const router = useRouter()

interface TaskCard {
  key: string
  title: string
  value: string
  hint: string
  tone: 'danger' | 'warn' | 'ok' | 'info'
  route: string
  query?: Record<string, string>
}

const cards = ref<TaskCard[]>([
  { key: 'assess', title: '保级风险', value: '—', hint: '考核 DANGER/PROGRESS', tone: 'info', route: 'assessment' },
  { key: 'transfer', title: '待整理', value: '—', hint: '下载完成待入库', tone: 'info', route: 'transfer' },
  { key: 'brush', title: '刷流中', value: '—', hint: '启用中的刷流任务', tone: 'info', route: 'brushflow' },
  { key: 'health', title: '站点健康', value: '—', hint: '监护室 critical', tone: 'info', route: 'savept' },
  { key: 'reseed', title: '辅种建议', value: '—', hint: '待处理候选', tone: 'info', route: 'reseed' },
  { key: 'doctor', title: '健康自检', value: '→', hint: '打开诊断页', tone: 'info', route: 'doctor' },
])
const loading = ref(false)

function setCard(key: string, patch: Partial<TaskCard>) {
  const i = cards.value.findIndex((c) => c.key === key)
  if (i >= 0) cards.value[i] = { ...cards.value[i], ...patch }
}

async function refresh() {
  loading.value = true
  try {
    const [assess, bf, savept, reseedTasks] = await Promise.all([
      assessmentApi.all(false).catch(() => []),
      brushflowApi.list().catch(() => []),
      saveptApi.get(false).catch(() => null),
      reseedApi.tasks('pending').catch(() => []),
    ])

    const danger = assess.filter((c) => c.status === 'DANGER').length
    const progress = assess.filter((c) => c.status === 'PROGRESS').length
    setCard('assess', {
      value: String(danger + progress),
      hint: danger ? `${danger} 站危急 · ${progress} 进行中` : progress ? `${progress} 站进行中` : '考核状态平稳',
      tone: danger ? 'danger' : progress ? 'warn' : 'ok',
    })

    const enabledBf = bf.filter((t) => t.enabled).length
    setCard('brush', {
      value: String(enabledBf),
      hint: enabledBf ? `${enabledBf} 个任务启用` : '暂无启用任务',
      tone: enabledBf ? 'warn' : 'ok',
    })

    const critical = savept?.summary?.critical ?? 0
    setCard('health', {
      value: String(critical),
      hint: critical ? `${critical} 站异常` : '监护站健康',
      tone: critical ? 'danger' : 'ok',
    })

    setCard('reseed', {
      value: String(reseedTasks.length),
      hint: reseedTasks.length ? `${reseedTasks.length} 待处理候选` : '无待处理辅种',
      tone: reseedTasks.length ? 'warn' : 'ok',
    })

    // 待整理：无独立队列 API，用失败/最近历史作提示入口
    try {
      const { transferApi } = await import('@/api/transfer')
      const hist = await transferApi.history('failed', 50)
      const n = hist.length
      setCard('transfer', {
        value: n ? String(n) : '→',
        hint: n ? `${n} 条失败记录` : '打开整理页查看',
        tone: n ? 'warn' : 'info',
      })
    } catch {
      setCard('transfer', { value: '→', hint: '打开整理页查看', tone: 'info' })
    }
  } finally {
    loading.value = false
  }
}

function open(c: TaskCard) {
  router.push({ name: c.route, query: c.query })
}

defineExpose({ refresh })
onMounted(() => {
  void refresh()
})
</script>

<template>
  <div class="task-cards" v-loading="loading">
    <div class="head">
      <h3>任务卡片</h3>
      <p>跨站总览入口 · 点卡片下钻</p>
      <button type="button" class="refresh" @click="refresh">刷新</button>
    </div>
    <div class="grid">
      <button
        v-for="c in cards"
        :key="c.key"
        type="button"
        class="card"
        :class="'tone-' + c.tone"
        @click="open(c)"
      >
        <span class="title">{{ c.title }}</span>
        <span class="value">{{ c.value }}</span>
        <span class="hint">{{ c.hint }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.task-cards { display: flex; flex-direction: column; gap: 12px; }
.head { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; }
.head h3 { margin: 0; font-size: 15px; }
.head p { margin: 0; color: var(--text-muted); font-size: 12px; flex: 1; }
.refresh {
  border: none; background: rgba(108, 92, 231, 0.1); color: var(--text-strong);
  border-radius: 999px; padding: 4px 10px; cursor: pointer; font-size: 12px;
}
.refresh:hover { background: rgba(108, 92, 231, 0.18); }
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 10px;
}
.card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  text-align: left;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid rgba(108, 92, 231, 0.12);
  background: rgba(108, 92, 231, 0.05);
  color: var(--text-strong);
  cursor: pointer;
}
.card:hover { background: rgba(108, 92, 231, 0.12); }
.card .title { font-size: 12px; color: var(--text-muted); }
.card .value { font-size: 22px; font-weight: 700; line-height: 1.2; }
.card .hint { font-size: 11px; color: var(--text-muted); }
.tone-danger { border-color: rgba(239, 68, 68, 0.35); background: rgba(239, 68, 68, 0.08); }
.tone-warn { border-color: rgba(245, 158, 11, 0.35); background: rgba(245, 158, 11, 0.08); }
.tone-ok { border-color: rgba(34, 197, 94, 0.28); }
.tone-info { }
</style>
