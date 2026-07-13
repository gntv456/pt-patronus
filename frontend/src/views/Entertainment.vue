<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { entertainmentApi, type Game, type PlayResult, type Site } from '@/api/entertainment'
import ScratchCardGame from '@/components/ScratchCardGame.vue'
import JggGameUi from '@/components/JggGameUi.vue'
import MagicScratchGame from '@/components/MagicScratchGame.vue'
import MedalSynthesisGame from '@/components/MedalSynthesisGame.vue'
import LongPTWheelGame from '@/components/LongPTWheelGame.vue'
import LongPTDailyGame from '@/components/LongPTDailyGame.vue'
import LongPTMedalGame from '@/components/LongPTMedalGame.vue'
import MagicFarmGame from '@/components/MagicFarmGame.vue'
import NovaHDScratchGame from '@/components/NovaHDScratchGame.vue'
import NovaHDLuckyGame from '@/components/NovaHDLuckyGame.vue'
import SpinnerGame from '@/components/SpinnerGame.vue'
import BetGame from '@/components/BetGame.vue'
import EventBet from '@/components/EventBet.vue'
import BlackjackGame from '@/components/BlackjackGame.vue'
import PandaFlipGame from '@/components/PandaFlipGame.vue'
import BigSmallGame from '@/components/BigSmallGame.vue'
import EmptyState from '@/components/EmptyState.vue'

const sites = ref<Site[]>([])
const currentSite = ref<number | undefined>()
const games = ref<Game[]>([])
const currentGame = ref<string | undefined>()
const loading = ref(false)
const playing = ref(false)
const result = ref<PlayResult | null>(null)

// 通用（非富 UI）游戏的玩一次
async function play() {
  if (!currentSite.value || !currentGame.value) return
  playing.value = true
  result.value = null
  try {
    const res = await entertainmentApi.play(currentSite.value, currentGame.value)
    result.value = res
    if (res.success) ElMessage.success(res.prize ? `获得 ${res.prize}` : '完成')
    else ElMessage.warning(res.message || '失败')
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '请求失败')
  } finally {
    playing.value = false
  }
}

async function loadSites() {
  try {
    sites.value = await entertainmentApi.sites()
    if (sites.value.length && currentSite.value === undefined) currentSite.value = sites.value[0].id
  } catch {
    ElMessage.error('加载站点失败')
  }
}

async function loadGames() {
  if (!currentSite.value) return
  loading.value = true
  games.value = []
  result.value = null
  try {
    games.value = await entertainmentApi.games(currentSite.value)
    currentGame.value = games.value[0]?.id
  } catch {
    ElMessage.error('加载游戏失败')
  } finally {
    loading.value = false
  }
}

const selectedGame = computed(() => games.value.find((g) => g.id === currentGame.value))
// 走富 UI 的玩法（其余走通用卡）
const richGame = computed(() => currentGame.value)
// 通用抽奖组件覆盖的 game.id（批1：多站转盘/老虎机/抽奖；红包雨 hxpt_redpacket 用 Play 封装触发）
const spinnerIds = ['cbg_wheel', 'cbg_slot', 'ub_wheel', 'luckydraw_wheel', 'march_wheel', 'tangpt_lottery', 'hxpt_redpacket']
// 通用下注组件覆盖的 game.id（批2：骰宝/双色球/Betting/红包；猜大小走专属 BigSmallGame；翻面走 PandaFlipGame）
const betIds = ['cbg_sicbo', 'crab_unionlotto', 'ubits_redpacket']
// 猜大小（批2.5：好学 hxpt_bigsmall + 熊猫 panda_bigsmall，富 UI：走势/排行/记录/倒计时/任务/聊天/红包）
const bigsmallIds = ['hxpt_bigsmall', 'panda_bigsmall']
// 二十一点状态机（批3）
const blackjackIds = ['audiences_blackjack']

watch(currentSite, loadGames)
loadSites()
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">娱乐</h2>
      <div class="selectors">
        <el-select v-model="currentSite" placeholder="选择站点" style="width: 200px" @change="loadGames">
          <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id">
            <div class="opt-row">
              <span class="opt-name">{{ s.name }}</span>
              <span class="opt-count" :class="{ zero: !s.game_count }">{{ s.game_count ?? 0 }}</span>
            </div>
          </el-option>
        </el-select>
        <el-select v-if="games.length > 1" v-model="currentGame" placeholder="选择玩法" style="width: 160px">
          <el-option v-for="g in games" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
      </div>
    </div>

    <div v-loading="loading">
      <!-- 富 UI 玩法（每个游戏独立组件） -->
      <ScratchCardGame v-if="richGame === 'cspt_scratch' && currentSite" :site-id="currentSite" />
      <JggGameUi v-else-if="richGame === 'jgg' && currentSite" :site-id="currentSite" />
      <MagicScratchGame v-else-if="richGame === 'magic_scratch' && currentSite" :site-id="currentSite" />
      <MedalSynthesisGame v-else-if="richGame === 'medal_synthesis' && currentSite" :site-id="currentSite" />
      <LongPTWheelGame v-else-if="richGame === 'longpt_wheel' && currentSite" :site-id="currentSite" />
      <LongPTDailyGame v-else-if="richGame === 'longpt_daily' && currentSite" :site-id="currentSite" />
      <LongPTMedalGame v-else-if="richGame === 'longpt_medal' && currentSite" :site-id="currentSite" />
      <MagicFarmGame v-else-if="richGame === 'magic_farm' && currentSite" :site-id="currentSite" />
      <NovaHDScratchGame v-else-if="richGame === 'novahd_scratch' && currentSite" :site-id="currentSite" />
      <NovaHDLuckyGame v-else-if="richGame === 'novahd_lucky' && currentSite" :site-id="currentSite" />
      <SpinnerGame v-else-if="currentSite && spinnerIds.includes(richGame as string)" :site-id="currentSite" :game="currentGame as string" :name="selectedGame?.name || ''" />
      <EventBet v-else-if="richGame === 'ourbits_bet' && currentSite" :site-id="currentSite" />
      <PandaFlipGame v-else-if="richGame === 'panda_flip' && currentSite" :site-id="currentSite" />
      <BigSmallGame v-else-if="currentSite && bigsmallIds.includes(richGame as string)" :site-id="currentSite" :game="currentGame as string" :name="selectedGame?.name || ''" />
      <BetGame v-else-if="currentSite && betIds.includes(richGame as string)" :site-id="currentSite" :game="currentGame as string" :name="selectedGame?.name || ''" />
      <BlackjackGame v-else-if="currentSite && blackjackIds.includes(richGame as string)" :site-id="currentSite" :game="currentGame as string" :name="selectedGame?.name || ''" />

      <!-- 通用玩法兜底 -->
      <div v-else-if="selectedGame" class="glass glass-hover simple-card">
        <div class="gname">{{ selectedGame.name }}</div>
        <el-button type="primary" :loading="playing" @click="play">玩一次</el-button>
        <div v-if="result" class="result" :class="result.success ? 'win' : 'lose'">
          <span class="badge">{{ result.success ? '✓ 成功' : '✗ 失败' }}</span>
          <span v-if="result.prize" class="prize">{{ result.prize }}</span>
          <span v-if="result.amount" class="amount">×{{ result.amount }}</span>
          <div v-if="result.message" class="msg text-muted">{{ result.message }}</div>
        </div>
      </div>

      <EmptyState
        v-else-if="!loading"
        icon="🎮"
        :title="currentSite ? '该站点暂无游戏' : '请先选择站点'"
        :description="currentSite ? '该站点尚未开放娱乐玩法，可切换其他站点试试' : '在右上角下拉选择一个 PT 站点，开始每日抽奖、刮刮乐等小游戏'"
      />
    </div>
  </div>
</template>

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.title { margin: 4px 0; font-size: 22px; font-weight: 700; }
.selectors { display: flex; gap: 10px; }
.opt-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; width: 100%; }
.opt-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.opt-count { flex: none; min-width: 20px; padding: 0 6px; border-radius: 999px; font-size: 12px; font-weight: 600; text-align: center; color: var(--pt-primary, #6c5ce7); background: color-mix(in srgb, var(--pt-primary, #6c5ce7) 14%, transparent); }
.opt-count.zero { color: var(--text-muted, #9aa0a6); background: rgba(120, 120, 120, 0.12); font-weight: 500; }
.simple-card { padding: 28px; display: flex; flex-direction: column; align-items: center; gap: 14px; max-width: 360px; margin: 0 auto; }
.gname { font-size: 18px; font-weight: 600; }
.result { width: 100%; padding: 10px; border-radius: var(--radius-md); font-size: 13px; display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
.result.win { background: rgba(46, 204, 113, 0.12); }
.result.lose { background: rgba(231, 76, 60, 0.1); }
.badge { font-weight: 700; }
.prize { color: var(--pt-primary, #6c5ce7); font-weight: 600; }
.amount { color: var(--pt-warn, #f59e0b); font-weight: 600; }
.msg { width: 100%; }
.empty { padding: 50px; text-align: center; }
</style>
