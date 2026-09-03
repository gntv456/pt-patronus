<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type PlayResult, type TangptLotteryState } from '@/api/entertainment'

// 躺平抽奖富组件（批14）：站点真实奖品扇区（含概率/颜色）+ 概率榜 + 连抽。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<TangptLotteryState | null>(null)
const spinning = ref(false)
const angle = ref(0)
const last = ref<PlayResult | null>(null)
const count = ref(1)

const slice = computed(() => 360 / (state.value?.items.length || 12))
const wheelStyle = computed(() => {
  const items = state.value?.items ?? []
  if (!items.length) return {}
  const parts = items.map(
    (it, i) => `${it.color || '#64748b'} ${(360 / items.length) * i}deg ${(360 / items.length) * (i + 1)}deg`,
  )
  return {
    background: `conic-gradient(from -90deg, ${parts.join(',')})`,
    transform: `rotate(${angle.value}deg)`,
    transition: spinning.value ? 'transform 3s cubic-bezier(0.15, 0.85, 0.25, 1)' : 'none',
  }
})
const sorted = computed(() =>
  [...(state.value?.items ?? [])].sort((a, b) => b.probability - a.probability),
)

async function refresh() {
  try {
    state.value = await entertainmentApi.tangptLotteryState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function draw() {
  if (spinning.value) return
  spinning.value = true
  last.value = null
  try {
    const res =
      count.value > 1
        ? await entertainmentApi.playMulti(props.siteId, props.game, count.value)
        : await entertainmentApi.play(props.siteId, props.game)
    angle.value += 360 * 5 + Math.floor(Math.random() * 360)
    await new Promise((r) => setTimeout(r, 3100))
    last.value = res
    if (res.success) {
      if (res.prizes?.length) {
        const groups = new Map<string, number>()
        for (const p of res.prizes) {
          const key = p.prize_name || '未知'
          groups.set(key, (groups.get(key) ?? 0) + 1)
        }
        ElMessage.success([...groups.entries()].map(([n, c]) => `${n} ×${c}`).join('，'))
      } else {
        ElMessage.success(res.prize || res.message || '中奖！')
      }
    } else {
      ElMessage.warning(res.message || '抽奖失败')
    }
  } finally {
    spinning.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="tpl">
    <div class="hero">
      <div class="hero-title">🎰 {{ name }}</div>
      <div v-if="state" class="hero-sub">
        {{ state.cost_per_draw.toLocaleString() }} 魔力/抽 · 今日剩余 {{ state.daily_remaining }} · 单次上限 {{ state.max_draw_count }}
      </div>
    </div>

    <section class="glass section wheel-zone">
      <div class="wheel-wrap">
        <div class="wheel" :style="wheelStyle">
          <div
            v-for="(it, i) in state?.items ?? []"
            :key="it.id"
            class="slice-label"
            :style="{ transform: `rotate(${(i + 0.5) * slice}deg)` }"
          >
            {{ it.name }}{{ it.amount ? ` ${it.amount >= 1000 ? it.amount.toLocaleString() : it.amount}` : '' }}
          </div>
        </div>
        <div class="hub">🎲</div>
        <div class="pointer">▼</div>
      </div>
      <div class="ops">
        <el-input-number v-model="count" :min="1" :max="state?.max_draw_count || 100" />
        <el-button type="primary" size="large" :loading="spinning" :disabled="!state?.enabled" @click="draw">
          抽 {{ count }} 次（{{ ((state?.cost_per_draw ?? 0) * count).toLocaleString() }} 魔力）
        </el-button>
      </div>
    </section>

    <section v-if="sorted.length" class="glass section">
      <div class="sec-title">📊 奖品与概率</div>
      <div class="prob-table">
        <div v-for="it in sorted" :key="it.id" class="prob-row">
          <span class="dot" :style="{ background: it.color || '#64748b' }" />
          <span class="p-name">{{ it.name }}{{ it.amount ? ` ×${it.amount.toLocaleString()}` : '' }}</span>
          <div class="p-bar">
            <div class="p-fill" :style="{ width: `${Math.min(100, it.probability * 3)}%`, background: it.color || '#64748b' }" />
          </div>
          <span class="p-val">{{ it.probability }}%</span>
        </div>
      </div>
    </section>

    <div v-if="last" class="result" :class="last.success ? 'win' : 'lose'">
      <span class="badge">{{ last.success ? '✓' : '✗' }}</span>
      <div class="r-body">
        <span v-if="last.prize" class="r-prize">{{ last.prize }}</span>
        <span v-if="last.balance" class="r-bal">魔力 {{ last.balance.toLocaleString() }}</span>
        <div v-if="last.message" class="r-msg text-muted">{{ last.message }}</div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="css">
.tpl { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 620px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #0f766e, #14b8a6); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; font-size: 13px; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.wheel-zone { display: flex; flex-direction: column; align-items: center; gap: 18px; }
.wheel-wrap { position: relative; width: 280px; height: 280px; }
.wheel { position: absolute; inset: 0; border-radius: 50%; border: 6px solid #fff; box-shadow: 0 8px 24px rgba(0,0,0,.25); overflow: hidden; }
.slice-label { position: absolute; left: 50%; top: 10px; transform-origin: 0 128px; font-size: 11px; font-weight: 700; color: #fff; text-shadow: 0 1px 2px rgba(0,0,0,.6); white-space: nowrap; }
.hub { position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); width: 56px; height: 56px; border-radius: 50%; background: #fff; display: flex; align-items: center; justify-content: center; font-size: 26px; box-shadow: 0 2px 8px rgba(0,0,0,.3); z-index: 2; }
.pointer { position: absolute; top: -12px; left: 50%; transform: translateX(-50%); font-size: 24px; color: #dc2626; z-index: 3; text-shadow: 0 1px 2px #fff; }
.ops { display: flex; gap: 12px; align-items: center; }
.prob-table { display: flex; flex-direction: column; gap: 8px; }
.prob-row { display: flex; align-items: center; gap: 10px; font-size: 13px; }
.dot { width: 10px; height: 10px; border-radius: 50%; flex: none; }
.p-name { min-width: 130px; font-weight: 600; }
.p-bar { flex: 1; height: 8px; border-radius: 4px; background: var(--border-light, #e5e7eb); overflow: hidden; }
.p-fill { height: 100%; border-radius: 4px; }
.p-val { min-width: 52px; text-align: right; color: var(--text-muted); }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
</style>
