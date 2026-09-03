<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type FishingState, type PlayResult } from '@/api/entertainment'

// 思齐钓鱼富组件（批8）：钓次进度/装备/图鉴网格，抛竿走 play-multi。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<FishingState | null>(null)
const fishing = ref(false)
const last = ref<PlayResult | null>(null)

const rarityOrder: Record<string, number> = { common: 0, uncommon: 1, rare: 2, epic: 3, legendary: 4 }
const codexSorted = computed(() => {
  if (!state.value) return []
  return [...state.value.codex].sort((a, b) =>
    (rarityOrder[a.rarity] ?? 9) - (rarityOrder[b.rarity] ?? 9) || a.code - b.code
  )
})

async function refresh() {
  try {
    state.value = await entertainmentApi.fishingState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function cast(times: number) {
  if (fishing.value) return
  fishing.value = true
  try {
    const res = await entertainmentApi.playMulti(props.siteId, props.game, times)
    last.value = res
    if (res.success) ElMessage.success(res.message || '收竿！')
    else ElMessage.warning(res.message || '抛竿失败')
    await refresh()
  } finally {
    fishing.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="fishgame">
    <div class="hero">
      <div class="hero-title">🎣 {{ name }}</div>
      <div v-if="state" class="hero-sub">
        魔力 <b>{{ state.balance.toLocaleString() }}</b> · 鱼竿 {{ state.selected_rod || '—' }}
        <template v-if="state.selected_lure"> · 鱼饵 {{ state.selected_lure }}</template>
      </div>
    </div>

    <template v-if="state">
      <section class="glass section">
        <div class="sec-title">今日钓次</div>
        <el-progress
          :percentage="state.attempts_limit ? Math.min(100, Math.round((state.attempts_used / state.attempts_limit) * 100)) : 0"
          :format="() => `${state!.attempts_used}/${state!.attempts_limit}`"
        />
        <div class="casts">
          <el-button type="primary" :loading="fishing" :disabled="state.attempts_used >= state.attempts_limit" @click="cast(1)">
            🎣 抛竿一次
          </el-button>
          <el-button :loading="fishing" :disabled="state.attempts_used >= state.attempts_limit" @click="cast(5)">
            连抛 5 次
          </el-button>
          <span class="text-muted caught">今日钓获 {{ state.caught_today }} 尾</span>
        </div>
      </section>

      <section class="glass section">
        <div class="sec-title">🐟 图鉴 {{ state.discovered_count }}/{{ state.total_species }}</div>
        <div class="codex">
          <div
            v-for="f in codexSorted"
            :key="f.code"
            class="fish"
            :class="[f.rarity, { unknown: !f.discovered }]"
            :title="f.discovered ? `${f.name} ×${f.quantity}` : '未解锁'"
          >
            <div class="f-name">{{ f.discovered ? f.name : '???' }}</div>
            <div class="f-meta">{{ f.rarity_label }}</div>
            <div v-if="f.discovered" class="f-qty">×{{ f.quantity }}</div>
          </div>
        </div>
      </section>
    </template>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.fishgame { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 720px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #0ea5e9, #22c55e); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.casts { display: flex; gap: 10px; align-items: center; margin-top: 12px; }
.caught { margin-left: auto; font-size: 13px; }
.codex { display: grid; grid-template-columns: repeat(auto-fill, minmax(96px, 1fr)); gap: 8px; }
.fish { border: 1px solid var(--border-light, #e5e7eb); border-radius: var(--radius-sm, 6px); padding: 8px; text-align: center; font-size: 12px; }
.fish.unknown { opacity: 0.45; filter: grayscale(0.8); }
.fish .f-name { font-weight: 700; }
.fish .f-meta { color: var(--text-muted, #909399); margin-top: 2px; }
.fish .f-qty { margin-top: 2px; }
.fish.rare { border-color: #60a5fa; }
.fish.epic { border-color: #a78bfa; }
.fish.legendary { border-color: #f59e0b; box-shadow: 0 0 6px rgba(245, 158, 11, 0.5); }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
</style>
