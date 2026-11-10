<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

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

async function act(selection: string, extra?: Record<string, string>) {
  if (working.value) return
  working.value = true
  try {
    const res = await entertainmentApi.bet(props.siteId, props.game, { selection, amount: 0, extra })
    last.value = res
    if (res.success) ElMessage.success(res.message || '操作成功')
    else ElMessage.warning(res.message || '操作失败')
    await refresh()
  } finally {
    working.value = false
  }
}

const expandedSlave = ref<string | null>(null)
function toggleSlave(uid: string) {
  expandedSlave.value = expandedSlave.value === uid ? null : uid
}

async function batch(kind: 'work' | 'interact') {
  const opts = state.value?.assets?.[0]?.[kind === 'work' ? 'work_options' : 'interaction_options'] ?? []
  if (!opts.length) {
    ElMessage.info('暂无可用选项')
    return
  }
  // 取第一个已解锁选项做一键
  const opt = opts.find((o) => o.unlocked)
  if (!opt) {
    ElMessage.info('所有选项均未解锁')
    return
  }
  const key = kind === 'work' ? 'work_key' : 'interaction_key'
  await act(kind === 'work' ? 'batch_work' : 'batch_interact', { [key]: opt.key })
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
        <div class="sec-title sec-flex">
          <span>👤 我的奴隶</span>
          <span class="batch-btns">
            <el-button size="small" :disabled="working" @click="batch('work')">一键工作</el-button>
            <el-button size="small" :disabled="working" @click="batch('interact')">一键互动</el-button>
          </span>
        </div>
        <div v-for="s in state.assets" :key="s.uid" class="slave-card">
          <div class="slave slave-row" @click="toggleSlave(s.uid)">
            <span class="s-name">{{ expandedSlave === s.uid ? '▾' : '▸' }} {{ s.username }}</span>
            <span class="s-meta text-muted">身价 {{ s.price || '—' }}</span>
            <span class="s-meta text-muted">{{ s.has_work ? `${s.work_label} · ${s.income_amount}` : '今日未安排' }}</span>
            <el-button size="small" type="danger" plain :disabled="working" @click.stop="act('release', { target_uid: s.uid })">释放</el-button>
          </div>
          <div v-if="expandedSlave === s.uid" class="slave-detail">
            <div v-if="s.work_options?.length" class="opt-group">
              <div class="opt-label">安排工作</div>
              <button
                v-for="o in s.work_options"
                :key="o.key"
                class="opt"
                :disabled="working || !o.unlocked"
                :title="o.description + (o.unlocked ? '' : `（${o.unlock_text || '未解锁'}）`)"
                @click="act('work', { target_uid: s.uid, work_key: o.key })"
              >
                <b>{{ o.label }}</b>
                <span class="text-muted">{{ o.unlocked ? (o.income || o.description || '') : (o.unlock_text || '未解锁') }}</span>
              </button>
            </div>
            <div v-if="s.interaction_options?.length" class="opt-group">
              <div class="opt-label">互动</div>
              <button
                v-for="o in s.interaction_options"
                :key="o.key"
                class="opt"
                :disabled="working || !o.unlocked"
                :title="o.description"
                @click="act('interact', { target_uid: s.uid, interaction_key: o.key })"
              >
                <b>{{ o.label }}</b>
                <span class="text-muted">{{ o.unlocked ? (o.description || '') : (o.unlock_text || '未解锁') }}</span>
              </button>
            </div>
            <div v-if="!s.work_options?.length && !s.interaction_options?.length" class="text-muted no-opts">
              该奴隶暂无可培养选项
            </div>
          </div>
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
.task, .feed { display: flex; gap: 12px; align-items: center; padding: 6px 0; border-bottom: 1px dashed var(--border-light, #eee); font-size: 13px; }
.task:last-child, .feed:last-child { border-bottom: none; }
.slave { display: flex; gap: 12px; align-items: center; font-size: 13px; }
.s-name, .t-name { font-weight: 600; min-width: 90px; }
.t-status { margin-left: auto; }
.t-status.claimed { color: var(--success, #67c23a); }
.t-status.completed { color: var(--warning, #e6a23c); }
.f-time { margin-left: auto; font-size: 12px; }
.result { display: flex; gap: 10px; padding: 12px 16px; border-radius: var(--radius-md); align-items: flex-start; }
.result.win { background: var(--success-bg, #f0f9eb); }
.result.lose { background: var(--danger-bg, #fef0f0); }
.badge { font-weight: 800; }

.sec-flex { display: flex; align-items: center; justify-content: space-between; }
.batch-btns { display: flex; gap: 6px; }
.slave-card { border: 1px solid var(--border-light, #eee); border-radius: 8px; margin-bottom: 8px; overflow: hidden; }
.slave-row { cursor: pointer; padding: 8px 10px; }
.slave-row:hover { background: var(--bg-hover, #f5f7fa); }
.slave { display: flex; gap: 12px; align-items: center; }
.slave :last-child { margin-left: auto; }
.slave-detail { padding: 10px 12px; border-top: 1px dashed var(--border-light, #eee); background: var(--bg-card, #fafafa); }
.opt-group { margin-bottom: 10px; }
.opt-group:last-child { margin-bottom: 0; }
.opt-label { font-size: 12px; font-weight: 700; color: var(--text-muted); margin-bottom: 6px; }
.opt { display: flex; flex-direction: column; gap: 2px; width: 100%; text-align: left; padding: 7px 10px; margin-bottom: 4px; border: 1px solid var(--border-light, #e5e7eb); border-radius: 6px; background: #fff; cursor: pointer; font-size: 13px; }
.opt:hover:not(:disabled) { border-color: var(--primary, #409eff); }
.opt:disabled { opacity: .55; cursor: not-allowed; }
.opt span { font-size: 11px; }
.no-opts { font-size: 12px; padding: 4px 0; }
</style>
