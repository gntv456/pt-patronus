<script setup lang="ts">
import { onMounted, toRef } from 'vue'
import { useMedalSynthesis, fmtNum as fmt } from '@/composables/useMedalSynthesis'

const props = defineProps<{ siteId: number }>()

const {
  targets, stats, selectedId, drawing, synthesizing, lastDraw,
  showHistory, history, historyLoading,
  selected, progressPct,
  loadAll, draw, synthesize, toggleHistory,
} = useMedalSynthesis(toRef(props, 'siteId'))

onMounted(loadAll)
</script>

<template>
  <div class="card-synth">
    <!-- 顶部栏 -->
    <div class="topbar">
      <span class="title">卡牌合成</span>
      <span class="balance">当前火花：{{ stats ? fmt(stats.balance) : '--' }}</span>
    </div>

    <div v-if="!targets.length" class="empty text-muted">
      未解析到合成目标（可能未登录或页面结构变化，见后端日志 <code>entertainment raw response</code>）
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
          <img v-if="selected.image" :src="selected.image" :alt="selected.name" class="t-img" @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')" />
          <div v-else class="t-img placeholder">🏅</div>
          <div class="t-name">{{ selected.name }}</div>
          <div class="t-progress">
            <div class="bar"><div class="fill" :style="{ width: progressPct + '%' }" /></div>
            <span class="pct">{{ selected.got }}/{{ selected.need }} · {{ progressPct }}%</span>
          </div>
          <div v-if="selected.pity" class="t-pity">保底 {{ selected.pity }}</div>
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
              <img v-if="f.image" :src="f.image" class="frag-img" @error="(e) => ((e.target as HTMLImageElement).style.display = 'none')" />
              <div v-else class="frag-img placeholder">💠</div>
              <div class="frag-name">{{ f.name || '碎片' }}</div>
              <div class="frag-count">
                <span class="cur">{{ f.current }}</span>/<span class="req">{{ f.required }}</span>
              </div>
            </div>
            <div v-if="!(selected?.fragments?.length)" class="text-muted frag-empty">无碎片数据</div>
          </div>

          <!-- 操作按钮 -->
          <div class="ops">
            <button class="op draw1" :disabled="!!drawing" @click="draw(1)">
              {{ drawing === 1 ? '…' : '单抽' }}<small>{{ fmt(selected.draw_price) }}</small>
            </button>
            <button class="op draw10" :disabled="!!drawing" @click="draw(10)">
              {{ drawing === 10 ? '…' : '十连' }}<small>{{ fmt(selected.ten_price) }}</small>
            </button>
            <button class="op draw100" :disabled="!!drawing" @click="draw(100)">
              {{ drawing === 100 ? '…' : '百连' }}<small>{{ fmt(selected.hundred_price) }}</small>
            </button>
            <button
              class="op synth"
              :disabled="synthesizing || !selected.can_synthesize"
              @click="synthesize"
            >
              {{ synthesizing ? '…' : '合成' }}
            </button>
            <button class="op hist" :class="{ active: showHistory }" @click="toggleHistory">记录</button>
          </div>

          <!-- 上次抽取结果 -->
          <div v-if="lastDraw" class="last-draw">
            <span class="ld-label">{{ lastDraw.count }} 连抽结果：</span>
            <span v-for="(f, i) in lastDraw.frags" :key="i" class="ld-frag">{{ f.name }}×{{ f.count }}</span>
            <span v-if="!lastDraw.frags.length" class="text-muted">{{ lastDraw.msg || '完成' }}</span>
          </div>

          <!-- 抽取记录 -->
          <div v-if="showHistory" class="history" v-loading="historyLoading">
            <div class="hd-label">抽取记录（{{ history.length }}）</div>
            <div v-if="!history.length" class="text-muted hist-empty">暂无记录</div>
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
