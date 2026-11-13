<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import SptIcon from '@/components/SptIcon.vue'
import { useFeedback } from '@/composables/useFeedback'
import { systemApi, type DoctorReport } from '@/api/system'

const { t } = useI18n()
const props = withDefaults(defineProps<{ embedded?: boolean }>(), { embedded: false })

const report = ref<DoctorReport | null>(null)
const loading = ref(false)
const fixing = ref(false)
const errorSummary = ref('')
const errorBox = ref<HTMLElement | null>(null)
const { confirm, undo, error: feedbackError } = useFeedback()

const sevColor: Record<string, string> = {
  info: 'var(--pt-success-fg)',
  warn: 'var(--pt-warn-fg)',
  error: 'var(--pt-danger-fg)',
}
// 状态不只靠颜色：不同严重级别用不同图标 + aria-label。
const sevIcon: Record<string, string> = { info: 'check-circle', warn: 'alert-triangle', error: 'alert-octagon' }
const statusType: Record<string, 'success' | 'info' | 'warning' | 'danger'> = {
  ok: 'success',
  skipped: 'info',
  degraded: 'warning',
  failed: 'danger',
  fixed: 'success',
}

async function run(fix: boolean) {
  errorSummary.value = ''
  if (fix) {
    // 破坏性操作：确认 + 列出将处理的项（复用现有文案与 findings 标题，不新增 i18n key）。
    const items = report.value?.findings?.map((f) => f.title) ?? []
    const ok = await confirm({
      title: t('doctor.fix'),
      detail: items.length ? `${t('doctor.fix')}：${items.join('、')}` : t('doctor.fix'),
      danger: true,
    })
    if (!ok) return
  }
  const snapshot = report.value
  if (fix) fixing.value = true
  else loading.value = true
  try {
    report.value = fix ? await systemApi.fixDoctor() : await systemApi.runDoctor()
    if (fix) {
      // 修复前快照 + 8s 撤销：恢复修复前报告并重新诊断。
      undo(t('doctor.fix'), () => {
        report.value = snapshot
        void run(false)
      })
    }
  } catch (e: unknown) {
    errorSummary.value = (e as Error).message || t('doctor.runFailed')
    feedbackError(errorSummary.value)
    void nextTick(() => errorBox.value?.focus())
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
        <h2 v-if="!props.embedded">{{ t('doctor.title') }}</h2>
        <h3 v-else class="title embedded-title">{{ t('doctor.title') }}</h3>
        <p v-if="report" class="text-muted">
          {{ t('doctor.statusLabel') }}
          <el-tag :type="report.status === 'healthy' ? 'success' : report.status === 'degraded' ? 'warning' : 'danger'">
            {{ report.status }}
          </el-tag>
          · {{ t('doctor.score') }} <b>{{ report.score ?? '-' }}</b
          >/100 ·
          {{
            t('doctor.itemsCount', {
              n: report.summary.total,
              info: report.summary.info,
              warn: report.summary.warn,
              error: report.summary.error,
            })
          }}
        </p>
      </div>
      <div class="doctor-actions">
        <el-button :loading="loading" @click="run(false)">{{ t('doctor.rerun') }}</el-button>
        <el-button type="primary" :loading="fixing" @click="run(true)">{{ t('doctor.fix') }}</el-button>
      </div>
    </div>

    <!-- 可聚焦的错误摘要：失败时不仅弹 toast，页内也给出可重试入口并移入焦点 -->
    <div v-if="errorSummary" ref="errorBox" class="doctor-error" role="alert" tabindex="-1">
      <SptIcon name="alert-circle" :size="16" aria-hidden="true" />
      <span>{{ errorSummary }}</span>
      <el-button size="small" @click="run(false)">{{ t('common.retry') }}</el-button>
    </div>

    <el-alert
      v-if="report?.environment?.safe_mode"
      type="warning"
      :closable="false"
      :title="t('doctor.safeModeTitle')"
      :description="t('doctor.safeModeDesc')"
    />

    <div v-if="loading" class="findings" aria-busy="true">
      <el-skeleton :rows="5" animated class="doctor-skeleton" />
    </div>
    <div v-else-if="!report?.findings?.length" class="doctor-empty solid-card">
      <SptIcon name="check-circle" :size="28" aria-hidden="true" />
      <span>{{ t('common.noData') }}</span>
      <el-button size="small" @click="run(false)">{{ t('doctor.rerun') }}</el-button>
    </div>
    <div v-else class="findings">
      <div v-for="f in report?.findings ?? []" :key="f.id" class="card solid-card finding">
        <div class="f-head">
          <SptIcon
            :name="sevIcon[f.severity] ?? 'info'"
            :size="16"
            class="f-sev"
            :style="{ color: sevColor[f.severity] }"
            :aria-label="f.severity"
          />
          <span class="f-title">{{ f.title }}</span>
          <el-tag size="small" :type="statusType[f.status] ?? 'info'">{{ f.status }}</el-tag>
          <span class="f-id text-muted">{{ f.id }}</span>
        </div>
        <div v-if="f.detail" class="f-detail">{{ f.detail }}</div>
        <div v-if="f.recommendation" class="f-rec">{{ t('doctor.recommendation') }}{{ f.recommendation }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.doctor-wrap {
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.doctor-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: var(--sp-3);
  flex-wrap: wrap;
}
.doctor-actions {
  display: flex;
  gap: var(--sp-2);
}
.findings {
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.finding {
  padding: 12px 14px;
}
.f-head {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
}
.f-sev {
  flex: 0 0 auto;
}
.doctor-error {
  display: flex;
  align-items: center;
  gap: var(--sp-2);
  padding: 10px 12px;
  border: 1px solid var(--color-danger);
  border-radius: var(--radius-control);
  background: var(--color-danger-soft);
  color: var(--pt-danger-fg);
  font-size: var(--fs-sm);
}
.doctor-error:focus-visible {
  outline: 2px solid var(--color-focus);
  outline-offset: 2px;
}
.doctor-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--sp-2);
  padding: var(--space-8);
  color: var(--color-text-muted);
  font-size: var(--fs-sm);
}
.doctor-skeleton {
  padding: 12px;
}
.f-title {
  font-weight: 600;
}
.f-id {
  margin-left: auto;
  font-size: var(--fs-xs);
}
.f-detail {
  margin-top: 6px;
  font-size: var(--fs-body);
}
.f-rec {
  margin-top: 4px;
  font-size: var(--fs-xs);
  color: var(--color-text-muted);
}
.doctor-wrap.embedded {
  padding: 0;
}
.doctor-wrap.embedded .embedded-title {
  font-size: var(--fs-lg);
  font-weight: 600;
  margin: 0;
}
@media (max-width: 640px) {
  .f-id,
  .f-rec {
    font-size: var(--fs-lg);
  }
}

/* 装饰性 staggered 入场与 hover 位移已移除（去 AI 感） */
</style>
