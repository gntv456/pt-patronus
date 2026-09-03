<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted, computed } from 'vue'
import { ElMessage } from 'element-plus'
import {
  entertainmentApi,
  type GameStats,
  type PlayResult,
  type PrizeDef,
  type GameRecord,
} from '@/api/entertainment'

// 通用「玩一次」富卡（批20）：替换裸按钮兜底——自动拉余额/次数（stats）、
// 最近记录（history，支持则显示）、奖品表（prizes，支持则显示）。
// 覆盖剩余未做专属 UI 的游戏：13city_lottery / wof_wheel / ubits_redpacket /
// hdvideo_chest / hdvideo_scratch / hdvideo_stock 等。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const playing = ref(false)
const result = ref<PlayResult | null>(null)
const loadingInfo = ref(true)

const hasPrizes = computed(() => prizes.value.length > 0)
const hasRecords = computed(() => records.value.length > 0)
const hasStats = computed(
  () =>
    stats.value != null &&
    ((stats.value.balance ?? 0) !== 0 ||
      stats.value.today_spins !== 0 ||
      stats.value.max_daily !== 0 ||
      (stats.value.cost_per_play ?? 0) !== 0),
)

async function loadInfo() {
  loadingInfo.value = true
  const [st, pz, hs] = await Promise.all([
    entertainmentApi.stats(props.siteId, props.game).catch(() => null),
    entertainmentApi.prizes(props.siteId, props.game).catch(() => [] as PrizeDef[]),
    entertainmentApi.history(props.siteId, props.game).catch(() => [] as GameRecord[]),
  ])
  stats.value = st
  prizes.value = pz ?? []
  records.value = (hs ?? []).slice(0, 8)
  loadingInfo.value = false
}

async function play() {
  if (playing.value) return
  playing.value = true
  result.value = null
  try {
    const res = await entertainmentApi.play(props.siteId, props.game)
    result.value = res
    if (res.success)
      ElMessage.success(res.prize ? t('entertainment.prizeWon', { p: res.prize }) : t('entertainment.done'))
    else ElMessage.warning(res.message || t('entertainment.failed'))
    await loadInfo()
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || t('common.requestFailed'))
  } finally {
    playing.value = false
  }
}

onMounted(loadInfo)
</script>

<template>
  <div class="gcard">
    <div class="hero">
      <div class="hero-title">🎮 {{ name }}</div>
      <div v-if="hasStats" class="hero-sub">
        <template v-if="stats?.balance">{{ t('bj.balance') }} <b>{{ stats.balance.toLocaleString() }}</b></template>
        <template v-if="stats?.cost_per_play"> · {{ stats.cost_per_play.toLocaleString() }}/次</template>
        <template v-if="stats?.max_daily"> · 今日 {{ stats.today_spins ?? 0 }}/{{ stats.max_daily }}</template>
      </div>
    </div>

    <section class="glass section play-zone" v-loading="loadingInfo">
      <el-button type="primary" size="large" :loading="playing" @click="play">
        {{ t('entertainment.playOnce') }}
      </el-button>

      <div v-if="result" class="result" :class="result.success ? 'win' : 'lose'">
        <span class="badge">{{ result.success ? t('entertainment.winMark') : t('entertainment.loseMark') }}</span>
        <div class="r-body">
          <span v-if="result.prize" class="r-prize">{{ result.prize }}</span>
          <span v-if="result.amount" class="r-amt">×{{ result.amount }}</span>
          <span v-if="result.balance" class="r-bal">{{ t('bj.balance') }} {{ result.balance.toLocaleString() }}</span>
          <div v-if="result.message" class="r-msg text-muted">{{ result.message }}</div>
        </div>
      </div>
    </section>

    <section v-if="hasPrizes" class="glass section">
      <div class="sec-title">🎁 {{ t('sp.prizes') }}</div>
      <div class="prize-tags">
        <el-tag v-for="p in prizes.slice(0, 16)" :key="p.id" size="small">{{ p.name }}</el-tag>
      </div>
    </section>

    <section v-if="hasRecords" class="glass section">
      <div class="sec-title">📜 {{ t('sp.history') }}</div>
      <div class="rec" v-for="(r, i) in records" :key="i">
        <span class="r-name">{{ r.prize_name || '—' }}</span>
        <span v-if="r.amount" class="r-amt">×{{ r.amount }}</span>
        <span class="r-time text-muted">{{ r.created_at }}</span>
      </div>
    </section>
  </div>
</template>

<style scoped lang="css">
.gcard { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 560px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #475569, #94a3b8); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: .92; font-size: 13px; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.play-zone { display: flex; flex-direction: column; align-items: center; gap: 14px; min-height: 90px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; width: 100%; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
.r-prize { font-weight: 800; margin-right: 8px; }
.r-amt { color: var(--warning, #e6a23c); font-weight: 700; }
.r-bal { color: var(--text-muted); }
.prize-tags { display: flex; flex-wrap: wrap; gap: 6px; }
.rec { display: flex; gap: 10px; align-items: baseline; padding: 5px 0; border-bottom: 1px dashed var(--border-light, #eee); font-size: 13px; }
.rec:last-child { border-bottom: none; }
.r-name { font-weight: 600; }
.r-time { margin-left: auto; font-size: 11px; }
</style>
