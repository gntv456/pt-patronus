<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { entertainmentApi, type GameStats, type PrizeDef, type GameRecord, type PlayResult } from '@/api/entertainment'

const props = defineProps<{ siteId: number }>()

const GAME = 'longpt_daily'

const stats = ref<GameStats | null>(null)
const prizes = ref<PrizeDef[]>([])
const records = ref<GameRecord[]>([])
const joining = ref(false)
const joined = ref<PlayResult | null>(null)

// 每日奖品图标按名称关键词推断（奖品名来自站点 desc 字段，如"333333魔力"）
function emoji(name: string) {
  if (/魔力|火花/.test(name)) return '🪙'
  if (/上传/.test(name)) return '📤'
  if (/VIP|会员/.test(name)) return '👑'
  if (/邀请/.test(name)) return '✉️'
  if (/勋章/.test(name)) return '🎖️'
  if (/彩虹/.test(name)) return '🌈'
  return '🎁'
}

async function refreshStats() {
  stats.value = await entertainmentApi.stats(props.siteId, GAME).catch(() => stats.value)
}
async function refreshHistory() {
  records.value = await entertainmentApi.history(props.siteId, GAME).catch(() => [])
}

async function join() {
  if (joining.value) return
  joining.value = true
  try {
    const res = await entertainmentApi.play(props.siteId, GAME)
    joined.value = res
    if (res.success) ElMessage.success(res.message || '参与成功，等待开奖')
    else ElMessage.warning(res.message || '参与失败')
    await Promise.all([refreshStats(), refreshHistory()])
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    joining.value = false
  }
}

onMounted(async () => {
  prizes.value = await entertainmentApi.prizes(props.siteId, GAME).catch(() => [] as PrizeDef[])
  await Promise.all([refreshStats(), refreshHistory()])
})
</script>

<template>
  <div class="lpt-daily">
    <div class="hero">
      <div class="hero-title">🎊 每日抽奖</div>
      <div class="hero-sub">每人每天仅限参与一次，{{ stats?.open_time ? `${stats.open_time} 统一开奖` : '当日开奖' }}</div>
    </div>

    <div class="board">
      <!-- 今日奖品 -->
      <div class="prizes">
        <div class="section-title">今日奖品</div>
        <div class="prize-grid">
          <div v-for="p in prizes" :key="p.id" class="prize-card">
            <span class="p-emoji">{{ emoji(p.name) }}</span>
            <span class="p-name">{{ p.name }}</span>
          </div>
          <div v-if="!prizes.length" class="text-muted empty-tip">暂无奖品配置</div>
        </div>
      </div>

      <!-- 立即参与 -->
      <div class="join">
        <div class="section-title">立即参与</div>
        <div class="join-circle-wrap">
          <button class="join-circle" :disabled="joining" @click="join">
            <span v-if="joining" class="spinner" />
            <template v-else>
              <span class="jc-icon">🎁</span>
              <span class="jc-text">{{ joined && joined.success ? '已参与' : '立即参与' }}</span>
            </template>
          </button>
        </div>
        <div class="join-meta">
          <div v-if="stats?.participants !== undefined" class="meta-row">
            <span>👥</span><span>已参与 {{ stats.participants }} 人</span>
          </div>
          <div v-if="stats?.start_time" class="meta-row">
            <span>🕒</span><span>参与时间 {{ stats.start_time }}-{{ stats.end_time }}</span>
          </div>
          <div v-if="stats?.open_time" class="meta-row">
            <span>🔔</span><span>开奖时间 {{ stats.open_time }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 参与提示 -->
    <div v-if="joined" class="result" :class="joined.success ? 'win' : 'lose'">
      <span class="badge">{{ joined.success ? '✓ 已参与' : '✗ 失败' }}</span>
      <span v-if="joined.message" class="r-msg">{{ joined.message }}</span>
    </div>

    <!-- 获奖记录 -->
    <div v-if="records.length" class="glass records">
      <div class="rec-title">获奖记录</div>
      <div v-for="(r, i) in records.slice(0, 10)" :key="i" class="rec-row">
        <span class="rec-prize" :class="{ muted: r.prize_name === '未中奖' }">{{ r.prize_name }}</span>
        <span v-if="r.amount" class="rec-amt">×{{ r.amount }}</span>
        <span class="rec-time text-muted">{{ r.created_at }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.lpt-daily { display: flex; flex-direction: column; gap: 16px; max-width: 640px; margin: 0 auto; }
.hero { padding: 16px; border-radius: 14px; background: linear-gradient(135deg, #1e3a8a, #3b5bdb); text-align: center; }
.hero-title { font-size: 22px; font-weight: 800; color: #fff; letter-spacing: 2px; }
.hero-sub { margin-top: 4px; font-size: 12px; color: rgba(255, 255, 255, 0.8); }

.board { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
@media (max-width: 520px) { .board { grid-template-columns: 1fr; } }
.section-title { font-weight: 700; margin-bottom: 10px; font-size: 15px; }

.prize-grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px; }
@media (max-width: 520px) { .prize-grid { grid-template-columns: repeat(3, 1fr); } }
.prize-card { display: flex; flex-direction: column; align-items: center; gap: 4px; padding: 12px 6px; border-radius: 12px; background: var(--glass-bg, rgba(255,255,255,0.06)); border: 1px solid var(--border-soft, rgba(255,255,255,0.08)); }
.p-emoji { font-size: 26px; }
.p-name { font-size: 12px; font-weight: 600; text-align: center; }
.empty-tip { grid-column: 1 / -1; text-align: center; padding: 20px; font-size: 13px; }

.join { display: flex; flex-direction: column; align-items: center; }
.join-circle-wrap { padding: 8px 0 14px; }
.join-circle { width: 120px; height: 120px; border-radius: 50%; border: 4px solid #fff; cursor: pointer;
  background: linear-gradient(135deg, #22c55e, #16a34a); color: #fff;
  display: flex; flex-direction: column; align-items: center; justify-content: center;
  box-shadow: 0 6px 18px rgba(34, 197, 94, 0.45); transition: transform 0.15s; }
.join-circle:hover:not(:disabled) { transform: scale(1.04); }
.join-circle:disabled { cursor: wait; opacity: 0.85; }
.jc-icon { font-size: 30px; }
.jc-text { font-size: 14px; font-weight: 700; margin-top: 4px; }
.spinner { width: 28px; height: 28px; border: 3px solid rgba(255,255,255,0.4); border-top-color: #fff; border-radius: 50%; animation: sp 0.8s linear infinite; }
@keyframes sp { to { transform: rotate(360deg); } }

.join-meta { width: 100%; display: flex; flex-direction: column; gap: 6px; }
.meta-row { display: flex; align-items: center; gap: 8px; padding: 6px 12px; border-radius: 10px; background: rgba(30, 58, 138, 0.12); font-size: 12px; }

.result { padding: 12px; border-radius: 12px; display: flex; gap: 10px; align-items: center; }
.result.win { background: rgba(34, 197, 94, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-weight: 700; }
.r-msg { font-size: 13px; }

.records { padding: 14px; border-radius: 12px; }
.rec-title { font-weight: 700; margin-bottom: 8px; }
.rec-row { display: flex; gap: 10px; align-items: center; padding: 5px 0; font-size: 13px; border-bottom: 1px solid var(--border-soft, rgba(255,255,255,0.05)); }
.rec-prize { font-weight: 600; }
.rec-prize.muted { color: var(--text-muted, #999); font-weight: 400; }
.rec-amt { color: var(--pt-warn, #f59e0b); }
.rec-time { margin-left: auto; font-size: 11px; }
</style>
