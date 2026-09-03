<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { entertainmentApi, type FriendTradeState, type PlayResult } from '@/api/entertainment'

// 好友买卖富组件（批8）：身价/主人/奴隶列表/收益/任务/动态，动作 claim/redeem 走 bet。
const { t } = useI18n()
const props = defineProps<{ siteId: number; game: string; name: string }>()

const state = ref<FriendTradeState | null>(null)
const working = ref(false)
const last = ref<PlayResult | null>(null)

async function refresh() {
  try {
    state.value = await entertainmentApi.friendTradeState(props.siteId, props.game)
  } catch (e: unknown) {
    const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
    ElMessage.error(msg || '状态获取失败')
  }
}

async function act(selection: string) {
  if (working.value) return
  working.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, props.game, { selection, amount: 0 })
    last.value = res
    if (res.success) ElMessage.success(res.message || '操作成功')
    else ElMessage.warning(res.message || '操作失败')
    await refresh()
  } finally {
    working.value = false
  }
}

onMounted(refresh)
</script>

<template>
  <div class="ftgame">
    <div class="hero">
      <div class="hero-title">🤝 {{ name }}</div>
      <div v-if="state" class="hero-sub">
        {{ state.me_name }} · 身价 <b>{{ state.profile_price }}</b>
        <template v-if="state.owner_name"> · 主人 <b>{{ state.owner_name }}</b></template>
        <template v-else> · 自由身</template>
      </div>
    </div>

    <template v-if="state">
      <section class="glass section">
        <div class="grid">
          <div class="cell">
            <div class="k">可领收益</div>
            <div class="v accent">{{ state.claimable_income }}</div>
            <el-button size="small" type="primary" :loading="working" :disabled="state.claimable_income === '0'" @click="act('claim')">
              领取
            </el-button>
          </div>
          <div class="cell">
            <div class="k">累计收益</div>
            <div class="v">{{ state.total_earned || '0' }}</div>
          </div>
          <div class="cell">
            <div class="k">资产总值</div>
            <div class="v">{{ state.asset_total_value || '0' }}</div>
          </div>
          <div class="cell">
            <div class="k">奴隶</div>
            <div class="v">{{ state.owned_count }}<span v-if="state.base_capacity"> / {{ state.base_capacity }}</span></div>
          </div>
          <div v-if="state.owner_name" class="cell">
            <div class="k">赎身价</div>
            <div class="v">{{ state.redeem_price }}</div>
            <el-button size="small" type="warning" :loading="working" :disabled="!state.can_redeem" @click="act('redeem')">
              赎身
            </el-button>
          </div>
          <div v-if="state.next_slot_price" class="cell">
            <div class="k">下一槽位价</div>
            <div class="v">{{ state.next_slot_price }}</div>
          </div>
        </div>
      </section>

      <section v-if="state.assets.length" class="glass section">
        <div class="sec-title">👤 我的奴隶</div>
        <div class="slave" v-for="s in state.assets" :key="s.uid">
          <span class="s-name">{{ s.username }}</span>
          <span class="s-meta text-muted">身价 {{ s.price || '—' }}</span>
          <span class="s-meta text-muted">{{ s.has_work ? `${s.work_label} · ${s.income_amount}` : '今日未安排' }}</span>
        </div>
      </section>

      <section v-if="state.tasks?.length" class="glass section">
        <div class="sec-title">📋 任务</div>
        <div class="task" v-for="task in state.tasks" :key="task.key">
          <span class="t-name">{{ task.label || task.key }}</span>
          <span class="t-prog text-muted">{{ task.progress }}/{{ task.target }}</span>
          <span class="t-reward text-muted">奖励 {{ task.reward_amount }}</span>
          <span class="t-status" :class="task.status">{{ task.status === 'claimed' ? '已领' : task.status === 'completed' ? '可领' : '进行中' }}</span>
        </div>
      </section>

      <section v-if="state.feed?.length" class="glass section">
        <div class="sec-title">📜 动态</div>
        <div class="feed" v-for="(f, i) in state.feed" :key="i">
          <span>{{ f.text }}</span>
          <span class="f-time text-muted">{{ f.created_at }}</span>
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
.ftgame { display: flex; flex-direction: column; gap: var(--sp-4); max-width: 720px; margin: 0 auto; }
.hero { padding: 18px; border-radius: var(--radius-md); background: linear-gradient(135deg, #7c3aed, #db2777); color: #fff; text-align: center; }
.hero-title { font-size: var(--fs-title); font-weight: 800; }
.hero-sub { margin-top: 6px; opacity: 0.92; }
.section { padding: 16px; }
.sec-title { font-weight: 700; margin-bottom: 10px; }
.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(150px, 1fr)); gap: 12px; }
.cell { display: flex; flex-direction: column; gap: 4px; }
.cell .k { font-size: 12px; color: var(--text-muted); }
.cell .v { font-size: 18px; font-weight: 700; }
.cell .v.accent { color: var(--primary, #409eff); }
.slave, .task, .feed { display: flex; gap: 12px; align-items: center; padding: 6px 0; border-bottom: 1px dashed var(--border-light, #eee); font-size: 13px; }
.slave:last-child, .task:last-child, .feed:last-child { border-bottom: none; }
.s-name, .t-name { font-weight: 600; min-width: 90px; }
.t-status { margin-left: auto; }
.t-status.claimed { color: var(--success, #67c23a); }
.t-status.completed { color: var(--warning, #e6a23c); }
.f-time { margin-left: auto; font-size: 12px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }
</style>
