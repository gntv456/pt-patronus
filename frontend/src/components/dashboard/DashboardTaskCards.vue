<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { assessmentApi } from '@/api/assessment'
import { brushflowApi } from '@/api/brushflow'
import { saveptApi } from '@/api/savept'
import { reseedApi } from '@/api/reseed'

const { t } = useI18n()
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
  { key: 'assess', title: t('dt2.assess'), value: '—', hint: t('dt2.assessHint'), tone: 'info', route: 'assessment' },
  {
    key: 'transfer',
    title: t('dt2.transfer'),
    value: '—',
    hint: t('dt2.transferHint'),
    tone: 'info',
    route: 'transfer',
  },
  { key: 'brush', title: t('dt2.brush'), value: '—', hint: t('dt2.brushHint'), tone: 'info', route: 'brushflow' },
  { key: 'health', title: t('dt2.health'), value: '—', hint: t('dt2.healthHint'), tone: 'info', route: 'savept' },
  { key: 'reseed', title: t('dt2.reseed'), value: '—', hint: t('dt2.reseedHint'), tone: 'info', route: 'reseed' },
  { key: 'doctor', title: t('dt2.doctor'), value: '→', hint: t('dt2.doctorHint'), tone: 'info', route: 'doctor' },
])
const loading = ref(false)

function setCard(key: string, patch: Partial<TaskCard>) {
  const i = cards.value.findIndex((c) => c.key === key)
  if (i >= 0) cards.value[i] = { ...cards.value[i], ...patch }
}

async function refresh() {
  loading.value = true
  try {
    // 并行拉数；用 allSettled 区分「成功空数据」与「接口失败」，避免失败时误报平稳/健康
    const { transferApi } = await import('@/api/transfer')
    const [assessR, bfR, saveptR, reseedR, transferR] = await Promise.allSettled([
      assessmentApi.all(false),
      brushflowApi.list(),
      saveptApi.get(false),
      reseedApi.tasks('pending'),
      transferApi.summary(false, 50),
    ])

    if (assessR.status === 'fulfilled') {
      const assess = assessR.value
      const danger = assess.filter((c) => c.status === 'DANGER').length
      const progress = assess.filter((c) => c.status === 'PROGRESS').length
      // UNKNOWN（抓取失败/超时/未登录）同样需要用户处理，不能当平稳
      const unknown = assess.filter((c) => c.status === 'UNKNOWN').length
      const hints: string[] = []
      if (danger) hints.push(t('dt2.assessDanger', { danger, progress }))
      else if (progress) hints.push(t('dt2.assessProgress', { progress }))
      if (unknown) hints.push(t('dt2.assessUnknown', { n: unknown }))
      if (!hints.length) hints.push(t('dt2.assessStable'))
      setCard('assess', {
        value: String(danger + progress + unknown),
        hint: hints.join(' · '),
        tone: danger ? 'danger' : progress || unknown ? 'warn' : 'ok',
      })
    } else {
      setCard('assess', { value: '—', hint: t('dt2.assessLoadFailed'), tone: 'info' })
    }

    if (bfR.status === 'fulfilled') {
      const enabledBf = bfR.value.filter((t) => t.enabled).length
      setCard('brush', {
        value: String(enabledBf),
        hint: enabledBf ? t('dt2.brushN', { n: enabledBf }) : t('dt2.brushNone'),
        tone: enabledBf ? 'warn' : 'ok',
      })
    } else {
      setCard('brush', { value: '—', hint: t('dt2.brushLoadFailed'), tone: 'info' })
    }

    if (saveptR.status === 'fulfilled' && saveptR.value) {
      const critical = saveptR.value.summary?.critical ?? 0
      setCard('health', {
        value: String(critical),
        hint: critical ? t('dt2.healthN', { n: critical }) : t('dt2.healthOk'),
        tone: critical ? 'danger' : 'ok',
      })
    } else {
      setCard('health', { value: '—', hint: t('dt2.healthLoadFailed'), tone: 'info' })
    }

    if (reseedR.status === 'fulfilled') {
      // 候选列表后端单次最多返回 500 条：卡片要报的是真实条数，用 total，缺 total 时退回已加载数。
      const r = reseedR.value
      const n = r.total ?? r.tasks.length
      setCard('reseed', {
        value: String(n),
        hint: n ? t('dt2.reseedN', { n }) : t('dt2.reseedNone'),
        tone: n ? 'warn' : 'ok',
      })
    } else {
      setCard('reseed', { value: '—', hint: t('dt2.reseedLoadFailed'), tone: 'info' })
    }

    // 真实待入库（扫下载器）+ 失败历史
    if (transferR.status === 'fulfilled') {
      const s = transferR.value
      const pending = s.pending || 0
      const failed = s.failed || 0
      const total = pending + failed
      const hints: string[] = []
      if (pending) hints.push(t('dt2.pendingN', { n: pending }))
      if (failed) hints.push(t('dt2.failedN', { n: failed }))
      if (s.scan_partial) hints.push(t('dt2.scanPartial'))
      if (!s.enabled) hints.push(t('dt2.transferDisabled'))
      setCard('transfer', {
        value: total ? String(total) : s.enabled ? '0' : '→',
        hint: hints.length ? hints.join(' · ') : t('dt2.noTodo'),
        tone: failed ? 'danger' : pending ? 'warn' : s.enabled ? 'ok' : 'info',
        query: failed && !pending ? { status: 'failed' } : pending ? { view: 'pending' } : undefined,
      })
    } else {
      setCard('transfer', { value: '—', hint: t('dt2.transferLoadFailed'), tone: 'info' })
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
  <div class="task-cards" :aria-busy="loading">
    <div class="head">
      <h3>{{ t('dt2.title') }}</h3>
      <p>{{ t('dt2.sub') }}</p>
      <button type="button" class="refresh" @click="refresh">{{ t('common.refresh') }}</button>
    </div>
    <el-skeleton v-if="loading" :rows="3" animated class="tc-sk" />
    <div v-else class="grid">
      <button v-for="c in cards" :key="c.key" type="button" class="card" :class="'tone-' + c.tone" @click="open(c)">
        <span class="title">{{ c.title }}</span>
        <span class="value">{{ c.value }}</span>
        <span class="hint">{{ c.hint }}</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.task-cards {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.head {
  display: flex;
  align-items: baseline;
  gap: var(--sp-2);
  flex-wrap: wrap;
}
.head h3 {
  margin: 0;
  font-size: var(--fs-lg);
}
.head p {
  margin: 0;
  color: var(--text-muted);
  font-size: var(--fs-xs);
  flex: 1;
}
.refresh {
  min-height: 24px;
  border: 1px solid var(--color-border);
  background: var(--color-surface-2);
  color: var(--color-text);
  border-radius: var(--radius-control);
  padding: 6px 12px;
  cursor: pointer;
  font-size: var(--fs-xs);
}
.refresh:hover {
  background: var(--pt-primary-bg);
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: var(--sp-2);
}
.card {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
  text-align: left;
  padding: 12px 14px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--pt-primary-bg);
  background: var(--pt-primary-bg);
  color: var(--text-strong);
  cursor: pointer;
  transition:
    background-color var(--dur-base) var(--ease),
    border-color var(--dur-base) var(--ease);
}
.card:hover {
  background: var(--pt-primary-bg);
}
.card .title {
  font-size: var(--fs-xs);
  color: var(--text-muted);
}
.card .value {
  font-size: var(--fs-title);
  font-weight: 700;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}
.tc-sk {
  min-height: 120px;
}
.card .hint {
  font-size: var(--fs-2xs);
  color: var(--text-muted);
}
.tone-danger {
  border-color: var(--pt-danger-bg);
  background: var(--pt-danger-bg);
}
.tone-warn {
  border-color: var(--pt-warn-bg);
  background: var(--pt-warn-bg);
}
.tone-ok {
  border-color: var(--pt-success-bg);
}
.tone-info {
}
</style>
