<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { onMounted, toRef } from 'vue'
import { useMedalSynthesis, fmtNum as fmt } from '@/composables/useMedalSynthesis'

const { t } = useI18n()
const props = defineProps<{ siteId: number }>()

const {
  targets,
  stats,
  selectedId,
  drawing,
  synthesizing,
  lastDraw,
  showHistory,
  history,
  historyLoading,
  selected,
  progressPct,
  loadAll,
  draw,
  synthesize,
  toggleHistory,
} = useMedalSynthesis(toRef(props, 'siteId'))

onMounted(loadAll)
</script>

<template>
  <div class="card-synth">
    <!-- 顶部栏 -->
    <div class="topbar">
      <span class="title">{{ t('msg2.title') }}</span>
      <span class="balance">{{ t('msg2.balance', { v: stats ? fmt(stats.balance) : '--' }) }}</span>
    </div>

    <div v-if="!targets.length" class="empty text-muted">
      {{ t('msg2.noTargetsPre') }}<code>entertainment raw response</code>{{ t('msg2.noTargetsPost') }}
    </div>

    <template v-else>
      <!-- 目标选择 -->
      <div v-if="targets.length > 1" class="target-tabs">
        <button
          v-for="t in targets"
          :key="t.id"
          class="tab"
          :class="{ active: t.id === selectedId }"
          @click="selectedId = t.id"
        >
          {{ t.name }}
        </button>
      </div>

      <div v-if="selected" class="panel">
        <!-- 左：目标卡 -->
        <div class="target-card">
          <img
            v-if="selected.image"
            :src="selected.image"
            :alt="selected.name"
            class="t-img"
            @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')"
          />
          <div v-else class="t-img placeholder"><SptIcon name="medal" :size="40" aria-hidden="true" /></div>
          <div class="t-name">{{ selected.name }}</div>
          <div class="t-progress">
            <div class="bar"><div class="fill" :style="{ width: progressPct + '%' }" /></div>
            <span class="pct">{{ selected.got }}/{{ selected.need }} · {{ progressPct }}%</span>
          </div>
          <div v-if="selected.pity" class="t-pity">{{ t('msg2.pity') }} {{ selected.pity }}</div>
        </div>

        <!-- 右：碎片网格 + 操作 -->
        <div class="right">
          <div class="frag-grid">
            <div
              v-for="(f, i) in selected.fragments"
              :key="i"
              class="frag"
              :class="{ collected: f.current >= f.required && f.required > 0, rare: f.is_rare }"
            >
              <img
                v-if="f.image"
                :src="f.image"
                :alt="f.name || t('msg2.fragAlt')"
                class="frag-img"
                @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')"
              />
              <div v-else class="frag-img placeholder"><SptIcon name="package" :size="24" aria-hidden="true" /></div>
              <div class="frag-name">{{ f.name || t('msg2.frag') }}</div>
              <div class="frag-count">
                <span class="cur">{{ f.current }}</span
                >/<span class="req">{{ f.required }}</span>
              </div>
            </div>
            <div v-if="!selected?.fragments?.length" class="text-muted frag-empty">{{ t('msg2.noFrags') }}</div>
          </div>

          <!-- 操作按钮 -->
          <div class="ops">
            <button class="op draw1" :disabled="!!drawing" @click="draw(1)">
              {{ drawing === 1 ? '…' : t('msg2.draw1') }}<small>{{ fmt(selected.draw_price) }}</small>
            </button>
            <button class="op draw10" :disabled="!!drawing" @click="draw(10)">
              {{ drawing === 10 ? '…' : t('msg2.draw10') }}<small>{{ fmt(selected.ten_price) }}</small>
            </button>
            <button class="op draw100" :disabled="!!drawing" @click="draw(100)">
              {{ drawing === 100 ? '…' : t('msg2.draw100') }}<small>{{ fmt(selected.hundred_price) }}</small>
            </button>
            <button class="op synth" :disabled="synthesizing || !selected.can_synthesize" @click="synthesize">
              {{ synthesizing ? '…' : t('msg2.synth') }}
            </button>
            <button class="op hist" :class="{ active: showHistory }" @click="toggleHistory">
              {{ t('msg2.history') }}
            </button>
          </div>

          <!-- 上次抽取结果 -->
          <div v-if="lastDraw" class="last-draw">
            <span class="ld-label">{{ t('msg2.drawResult', { n: lastDraw.count }) }}</span>
            <span v-for="(f, i) in lastDraw.frags" :key="i" class="ld-frag">{{ f.name }}×{{ f.count }}</span>
            <span v-if="!lastDraw.frags.length" class="text-muted">{{ lastDraw.msg || t('bg.done') }}</span>
          </div>

          <!-- 抽取记录 -->
          <div v-if="showHistory" class="history" v-loading="historyLoading">
            <div class="hd-label">{{ t('msg2.histN', { n: history.length }) }}</div>
            <div v-if="!history.length" class="text-muted hist-empty">{{ t('msg2.noHistory') }}</div>
            <div v-else class="hist-list">
              <div v-for="(r, i) in history" :key="i" class="hist-row">
                <span class="h-frag">{{ r.prize_name || '—' }}</span>
                <span class="h-syn text-muted">{{ r.prize_type }}</span>
                <span v-if="r.cost > 0" class="h-cost">-{{ fmt(r.cost) }}</span>
                <span class="h-time text-muted">{{ r.created_at }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped src="./medal-synthesis.css"></style>
