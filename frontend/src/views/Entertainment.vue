<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { entertainmentApi, type Game, type Site } from '@/api/entertainment'
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
import GobangGame from '@/components/GobangGame.vue'
import FriendTradeGame from '@/components/FriendTradeGame.vue'
import FishingGame from '@/components/FishingGame.vue'
import City13WheelGame from '@/components/City13WheelGame.vue'
import City13DiceGame from '@/components/City13DiceGame.vue'
import CbgWheelGame from '@/components/CbgWheelGame.vue'
import CbgSlotGame from '@/components/CbgSlotGame.vue'
import LottoPickerGame from '@/components/LottoPickerGame.vue'
import TangptLotteryGame from '@/components/TangptLotteryGame.vue'
import HiloGame from '@/components/HiloGame.vue'
import SimpleGameCard from '@/components/SimpleGameCard.vue'
import EmptyState from '@/components/EmptyState.vue'

const { t } = useI18n()
const sites = ref<Site[]>([])
const currentSite = ref<number | undefined>()
const games = ref<Game[]>([])
const currentGame = ref<string | undefined>()
const loading = ref(false)

async function loadSites() {
  try {
    sites.value = await entertainmentApi.sites()
    if (sites.value.length && currentSite.value === undefined) {
      // 总览页直达：?site=<id>&game=<id>
      const qs = new URLSearchParams(window.location.search)
      const qSite = Number(qs.get('site')) || undefined
      const qGame = qs.get('game') || undefined
      if (qSite && sites.value.some((s) => s.id === qSite)) {
        currentSite.value = qSite
        pendingGame.value = qGame
      } else {
        currentSite.value = sites.value[0].id
      }
    }
  } catch {
    ElMessage.error(t('entertainment.loadSitesFailed'))
  }
}
const pendingGame = ref<string | undefined>()

async function loadGames() {
  if (!currentSite.value) return
  loading.value = true
  games.value = []
  try {
    games.value = await entertainmentApi.games(currentSite.value)
    currentGame.value =
      (pendingGame.value && games.value.some((g) => g.id === pendingGame.value) ? pendingGame.value : undefined) ??
      games.value[0]?.id
    pendingGame.value = undefined
  } catch {
    ElMessage.error(t('entertainment.loadGamesFailed'))
  } finally {
    loading.value = false
  }
}

const selectedGame = computed(() => games.value.find((g) => g.id === currentGame.value))
// 走富 UI 的玩法（其余走通用卡）
const richGame = computed(() => currentGame.value)
// 通用抽奖组件覆盖的 game.id（批1：多站转盘/老虎机/抽奖；红包雨 hxpt_redpacket 用 Play 封装触发）
const spinnerIds = [
  'ub_wheel',
  'luckydraw_wheel',
  'march_wheel',
  'hxpt_redpacket',
  'hhan_lucky_wheel',
]
// 通用下注组件覆盖的 game.id（批2：骰宝/双色球/Betting/红包；猜大小走专属 BigSmallGame；翻面走 PandaFlipGame）
// 批6：麒麟双色球 hdkylin_lotto（选号 6红+1蓝）+ 蟹黄堡快三 crab_fastthree（角色三选一，Amount=批量注数）
const betIds = ['cbg_sicbo', 'ubits_redpacket', 'crab_fastthree', 'tangpt_slot', 'hdvideo_slots', 'hdvideo_bigsmall']
// 猜大小（批2.5：好学 hxpt_bigsmall + 熊猫 panda_bigsmall + 躺平 tangpt_bigsmall，富 UI：走势/排行/记录/倒计时/任务/聊天/红包）
const bigsmallIds = ['hxpt_bigsmall', 'panda_bigsmall', 'tangpt_bigsmall']
// 二十一点状态机（批3；批7 补 CHDBits 复用 nxp + OpenCD 对抗制变体）
const blackjackIds = ['audiences_blackjack', 'nxp_blackjack', 'opencd_blackjack', 'hdvideo_blackjack']

watch(currentSite, loadGames)
loadSites()
</script>

<template>
  <div class="page">
    <div class="head">
      <h2 class="title">{{ t('entertainment.title') }}</h2>
      <div class="selectors">
        <el-select
          v-model="currentSite"
          :placeholder="t('entertainment.selectSite')"
          style="width: 200px"
          popper-class="ent-site-dropdown"
          @change="loadGames"
        >
          <el-option v-for="s in sites" :key="s.id" :label="s.name" :value="s.id">
            <div class="opt-row">
              <span class="opt-name">{{ s.name }}</span>
              <span class="opt-count" :class="{ zero: !s.game_count }">{{ s.game_count ?? 0 }}</span>
            </div>
          </el-option>
        </el-select>
        <el-select
          v-if="games.length > 1"
          v-model="currentGame"
          :placeholder="t('entertainment.selectGame')"
          style="width: 160px"
        >
          <el-option v-for="g in games" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
      </div>
    </div>

    <div v-loading="loading" :key="`${currentSite}-${currentGame}`" class="game-stage">
      <!-- 富 UI 玩法（每个游戏独立组件） -->
      <ScratchCardGame v-if="richGame === 'cspt_scratch' && currentSite" :site-id="currentSite" />
      <JggGameUi v-else-if="richGame === 'jgg' && currentSite" :site-id="currentSite" />
      <MagicScratchGame v-else-if="richGame === 'magic_scratch' && currentSite" :site-id="currentSite" />
      <MedalSynthesisGame v-else-if="richGame === 'medal_synthesis' && currentSite" :site-id="currentSite" />
      <LongPTWheelGame v-else-if="richGame === 'longpt_wheel' && currentSite" :site-id="currentSite" />
      <LongPTDailyGame v-else-if="richGame === 'longpt_daily' && currentSite" :site-id="currentSite" />
      <LongPTMedalGame v-else-if="richGame === 'longpt_medal' && currentSite" :site-id="currentSite" />
      <MagicFarmGame
        v-else-if="['magic_farm', 'siqi_plant', 'dubhe_stardust'].includes(richGame as string) && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
      />
      <NovaHDScratchGame v-else-if="richGame === 'novahd_scratch' && currentSite" :site-id="currentSite" />
      <NovaHDLuckyGame v-else-if="richGame === 'novahd_lucky' && currentSite" :site-id="currentSite" />
      <SpinnerGame
        v-else-if="currentSite && spinnerIds.includes(richGame as string)"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <EventBet v-else-if="richGame === 'ourbits_bet' && currentSite" :site-id="currentSite" />
      <PandaFlipGame v-else-if="richGame === 'panda_flip' && currentSite" :site-id="currentSite" />
      <BigSmallGame
        v-else-if="currentSite && bigsmallIds.includes(richGame as string)"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <BetGame
        v-else-if="currentSite && betIds.includes(richGame as string)"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <BlackjackGame
        v-else-if="currentSite && blackjackIds.includes(richGame as string)"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <GobangGame
        v-else-if="richGame === 'pting_gobang' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <FriendTradeGame
        v-else-if="richGame === 'panda_friendtrade' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <FishingGame
        v-else-if="richGame === 'siqi_fishing' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <City13WheelGame
        v-else-if="richGame === '13city_wheel' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <City13DiceGame
        v-else-if="richGame === '13city_dice' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <CbgWheelGame
        v-else-if="richGame === 'cbg_wheel' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <CbgSlotGame
        v-else-if="richGame === 'cbg_slot' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <HiloGame
        v-else-if="richGame === 'hdvideo_hilo' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <TangptLotteryGame
        v-else-if="richGame === 'tangpt_lottery' && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />
      <LottoPickerGame
        v-else-if="['zmpt_lotto', 'crab_unionlotto'].includes(richGame as string) && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame?.name || ''"
      />

      <!-- 通用玩法兜底（批20：SimpleGameCard 富卡——stats/prizes/history 自动加载） -->
      <SimpleGameCard
        v-else-if="selectedGame && currentSite"
        :site-id="currentSite"
        :game="currentGame as string"
        :name="selectedGame.name"
      />

      <EmptyState
        v-else-if="!loading"
        icon="🎮"
        :title="currentSite ? t('entertainment.emptySiteTitle') : t('entertainment.emptyTitle')"
        :description="currentSite ? t('entertainment.emptySiteDesc') : t('entertainment.emptyDesc')"
      />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: var(--sp-4);
}
.head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-3);
  flex-wrap: wrap;
}
.title {
  margin: 4px 0;
  font-size: var(--fs-title);
  font-weight: 700;
}
.selectors {
  display: flex;
  gap: var(--sp-2);
}
.opt-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--sp-4);
  width: 100%;
}
.opt-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.opt-count {
  flex: none;
  min-width: 20px;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  font-size: var(--fs-xs);
  font-weight: 600;
  text-align: center;
  color: var(--pt-primary, #6c5ce7);
  background: color-mix(in srgb, var(--pt-primary, #6c5ce7) 14%, transparent);
}
.opt-count.zero {
  color: var(--text-muted, #9aa0a6);
  background: rgba(120, 120, 120, 0.12);
  font-weight: 500;
}
.simple-card {
  padding: 28px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--sp-3);
  max-width: 360px;
  margin: 0 auto;
}
.gname {
  font-size: var(--fs-xl);
  font-weight: 600;
}
.result {
  width: 100%;
  padding: 10px;
  border-radius: var(--radius-md);
  font-size: var(--fs-body);
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-2);
  align-items: center;
}
.result.win {
  background: var(--pt-success-bg);
}
.result.lose {
  background: var(--pt-danger-bg);
}
.badge {
  font-weight: 700;
}
.prize {
  color: var(--pt-primary, #6c5ce7);
  font-weight: 600;
}
.amount {
  color: var(--pt-warn, #f59e0b);
  font-weight: 600;
}
.msg {
  width: 100%;
}
.empty {
  padding: 50px;
  text-align: center;
}
</style>

<style>
/* 娱乐页站点筛选下拉：加高面板以显示更多站点（el-select 默认 max-height 仅约 274px，只够 7-8 项）。
   popper-class 作用在 teleport 到 body 的 dropdown 根元素，必须用非 scoped 全局样式才能命中。 */
.ent-site-dropdown.el-select-dropdown {
  max-height: 60vh;
}
.ent-site-dropdown .el-select-dropdown__wrap,
.ent-site-dropdown .el-scrollbar__wrap {
  max-height: 60vh;
}

/* 游戏切换：水润淡入 */
.game-stage > * {
  animation: pt-scale-in var(--dur-slow) var(--ease-out-spring) both;
}
.result {
  animation: pt-fade-in-up var(--dur-base) var(--ease-out-smooth) both;
}
</style>
