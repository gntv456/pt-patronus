<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// PT监护室站点详情弹窗：点击卡片右下角「详情」按钮打开。
// 七个区块：站点LOGO/站点网址/开站时间/站点特色/站点人数/站点种子数/站点介绍。
// 数据 = savept.icu 卡片 + ptseek 元数据（后端按域名合并进 detail）。
import { useI18n } from 'vue-i18n'
import type { SaveptSiteCard as SiteCard } from '@/api/savept'
import { getStatusMeta } from '@/utils/savept'
import { safeUrl } from '@/utils/safeUrl'
import SptIcon from '@/components/SptIcon.vue'

const { t } = useI18n()
// 服务端 open/close_time 缺失时下发的占位值，判断时须比对原始值而非本地化文案
const unknownSentinel = '未知'
defineProps<{ card: SiteCard | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()

function isKnown(v?: string) {
  return !!v && v !== unknownSentinel
}
function fmtNum(n?: number) {
  return typeof n === 'number' && n > 0 ? n.toLocaleString() : t('ssc.unknown')
}
// 站点形式：PT 站点 / 社区论坛 / Usenet 等（ptseek service_type）
function serviceTypeLabel(card: SiteCard) {
  const st = card.detail?.service_type
  if (!st || st === 'pt_tracker') return t('ssc.stPT')
  if (st === 'community_forum') return t('ssc.stForum')
  if (st.includes('usenet')) return 'Usenet'
  return st
}
// 开站日期展示：优先 savept.icu 精确日期，其次 ptseek founded 年份
function openDateText(card: SiteCard) {
  if (isKnown(card.open_time)) return card.open_time
  if (card.detail?.founded) return String(card.detail.founded)
  return t('ssc.unknown')
}
</script>

<template>
  <Teleport to="body">
    <Transition name="spt-dialog-fade">
      <div v-if="card" class="spt-sd-overlay" @click.self="emit('close')">
        <div class="spt-sd" role="dialog" aria-modal="true" :aria-label="t('ssc.dTitle')">
          <button class="spt-sd__close" :aria-label="t('common.close')" @click="emit('close')">
            <SptIcon name="x" :size="16" />
          </button>

          <!-- ① 站点LOGO + 名称/别名 + 状态 -->
          <div class="spt-sd__head">
            <img
              v-if="card.icon || card.detail?.logo"
              class="spt-sd__logo"
              :src="card.icon || card.detail?.logo"
              :alt="card.name"
            />
            <div v-else class="spt-sd__logo spt-sd__logo--placeholder">{{ card.name.slice(0, 1) }}</div>
            <div class="spt-sd__head-text">
              <div class="spt-sd__name-row">
                <span class="spt-sd__name">{{ card.name }}</span>
                <span class="spt-sd__status" :style="{ color: getStatusMeta(card.status).color }">
                  {{ getStatusMeta(card.status).label }}
                </span>
              </div>
              <div v-if="card.detail?.aka?.length" class="spt-sd__aka">
                {{ t('ssc.dAka') }}：{{ card.detail.aka.join(' / ') }}
              </div>
            </div>
          </div>

          <dl class="spt-sd__rows">
            <!-- ② 站点网址 -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dUrl') }}</dt>
              <dd>
                <a :href="safeUrl(card.url)" target="_blank" rel="noreferrer noopener" class="spt-sd__link">
                  {{ card.domain || card.url }}
                </a>
              </dd>
            </div>
            <!-- ③ 开站时间（含关站/运营时长） -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dOpen') }}</dt>
              <dd>
                {{ openDateText(card) }}
                <template v-if="card.duration_text">（{{ t('ssc.dDuration') }} {{ card.duration_text }}）</template>
              </dd>
            </div>
            <div v-if="isKnown(card.close_time)" class="spt-sd__row">
              <dt>{{ t('ssc.dClose') }}</dt>
              <dd>{{ card.close_time }}</dd>
            </div>
            <!-- ④ 站点特色：内外站 + 平台 + 资源标签 + 社区标签 -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dFeature') }}</dt>
              <dd class="spt-sd__tags">
                <span v-if="card.site_type === 'internal'" class="spt-sd__tag">内站</span>
                <span v-else-if="card.site_type === 'external'" class="spt-sd__tag">外站</span>
                <span v-if="card.detail?.schema" class="spt-sd__tag">{{ card.detail.schema }}</span>
                <span v-for="tag in card.detail?.tags ?? []" :key="tag" class="spt-sd__tag">{{ tag }}</span>
                <span
                  v-for="cl in card.detail?.community_labels ?? []"
                  :key="cl"
                  class="spt-sd__tag spt-sd__tag--community"
                >{{ cl }}</span>
              </dd>
            </div>
            <!-- ⑤ 站点人数 -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dUsers') }}</dt>
              <dd>
                {{ fmtNum(card.detail?.users) }}
                <template v-if="card.detail?.peers"> · {{ t('ssc.dPeers') }} {{ fmtNum(card.detail.peers) }}</template>
              </dd>
            </div>
            <!-- ⑥ 站点种子数 -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dTorrents') }}</dt>
              <dd>{{ fmtNum(card.detail?.torrents) }}</dd>
            </div>
            <!-- 注册状态/难度（ptseek 有则展示） -->
            <div v-if="card.detail?.registration" class="spt-sd__row">
              <dt>{{ t('ssc.dReg') }}</dt>
              <dd>
                {{ card.detail.registration }}
                <template v-if="card.detail.difficulty"> · {{ t('ssc.dDifficulty') }} {{ card.detail.difficulty }}</template>
              </dd>
            </div>
            <!-- 官方发布组 -->
            <div v-if="card.detail?.official_groups?.length" class="spt-sd__row">
              <dt>{{ t('ssc.dGroups') }}</dt>
              <dd class="spt-sd__tags">
                <span v-for="g in card.detail.official_groups" :key="g" class="spt-sd__tag">{{ g }}</span>
              </dd>
            </div>
            <!-- 站点架构（ptseek 的 site_engine 与本地 definitions 的 schema 二选一） -->
            <div v-if="card.detail?.site_engine || card.detail?.schema" class="spt-sd__row">
              <dt>{{ t('ssc.dEngine') }}</dt>
              <dd>{{ card.detail.site_engine || card.detail.schema }}</dd>
            </div>
            <!-- 站点形式 / 语言 -->
            <div class="spt-sd__row">
              <dt>{{ t('ssc.dForm') }}</dt>
              <dd>
                {{ serviceTypeLabel(card) }}
                <template v-if="card.detail?.language"> · {{ card.detail.language }}</template>
              </dd>
            </div>
          </dl>

          <!-- 社区画像指标（人数/种子/peer 之外的政策布尔位） -->
          <div
            v-if="card.detail && (card.detail.freeleech || card.detail.bonus_points || card.detail.hit_and_run || card.detail.ratio_required)"
            class="spt-sd__metrics"
          >
            <span v-if="card.detail.freeleech" class="spt-sd__metric spt-sd__metric--ok">{{ t('ssc.mFree') }}</span>
            <span v-if="card.detail.bonus_points" class="spt-sd__metric spt-sd__metric--ok">{{ t('ssc.mBonus') }}</span>
            <span v-if="card.detail.hit_and_run" class="spt-sd__metric spt-sd__metric--warn">{{ t('ssc.mHR') }}</span>
            <span v-if="card.detail.ratio_required" class="spt-sd__metric spt-sd__metric--warn">{{ t('ssc.mRatio') }}</span>
          </div>

          <!-- ⑦ 站点介绍 + 新手提示 -->
          <div v-if="card.detail?.description" class="spt-sd__intro">
            <div class="spt-sd__section-label">{{ t('ssc.dIntro') }}</div>
            <p class="spt-sd__intro-text">{{ card.detail.description }}</p>
          </div>
          <div v-if="card.detail?.beginner_tips" class="spt-sd__intro spt-sd__intro--tips">
            <div class="spt-sd__section-label">{{ t('ssc.dTips') }}</div>
            <p class="spt-sd__intro-text">{{ card.detail.beginner_tips }}</p>
          </div>

          <!-- 资料来源 -->
          <div v-if="card.detail?.sources?.length" class="spt-sd__sources">
            <span class="spt-sd__sources-label">{{ t('ssc.dSources') }}（{{ card.detail.sources.length }}）：</span>
            <a
              v-for="s in card.detail.sources"
              :key="s.name + s.url"
              :href="safeUrl(s.url) || undefined"
              target="_blank"
              rel="noreferrer noopener"
              class="spt-sd__source-link"
              >{{ s.name }}</a
            >
          </div>

          <!-- 元信息：排名 / 来源分类 / 研究状态 / 核对日期 -->
          <div
            v-if="card.detail && (card.detail.rank || card.detail.source_category || card.detail.research_status || card.detail.checked_at)"
            class="spt-sd__meta"
          >
            <span v-if="card.detail.rank" class="spt-sd__meta-item">{{ t('ssc.dRank') }} #{{ card.detail.rank }}</span>
            <span v-if="card.detail.source_category" class="spt-sd__meta-item">{{ card.detail.source_category }}</span>
            <span v-if="card.detail.research_status" class="spt-sd__meta-item">{{ card.detail.research_status }}</span>
            <span v-if="card.detail.checked_at" class="spt-sd__meta-item">{{ t('ssc.dChecked') }} {{ card.detail.checked_at }}</span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped src="./SaveptSiteDetail.css"></style>
