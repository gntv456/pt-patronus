<script setup lang="ts">
import { formatTorrentSize, type EnrichedMeta, type TorrentTag } from '@/utils/torrentMeta'
import { useI18n } from 'vue-i18n'
import SiteFavicon from '@/components/SiteFavicon.vue'
import MediaPoster from '@/components/MediaPoster.vue'

const { t } = useI18n()

export type FindRow = import('@/components/SiteTorrentFindDialog.vue').FindRow
type EnrichedFindRow = FindRow & EnrichedMeta

export type CardGroup = {
  key: string
  primary: EnrichedFindRow
  more: EnrichedFindRow[]
  tags: TorrentTag[]
  promo: { text: string; tone: string } | null
  timeText: string
}

defineProps<{
  group: CardGroup
  title: string
  posterUrl?: string
}>()

const emit = defineEmits<{
  (e: 'push', row: EnrichedFindRow): void
  (e: 'more', group: CardGroup): void
  (e: 'detail', row: EnrichedFindRow): void
}>()
</script>

<template>
  <article class="sf-card" :class="{ 'has-poster': !!posterUrl }" @click="emit('push', group.primary)">
    <div v-if="posterUrl" class="sf-card-bg" aria-hidden="true">
      <MediaPoster :url="posterUrl" kind="movie" />
    </div>
    <div class="sf-card-veil" />

    <div v-if="group.promo" class="sf-promo" :class="`tone-${group.promo.tone}`">{{ group.promo.text }}</div>

    <header class="sf-card-head">
      <div class="sf-media-title" :title="title">{{ title }}</div>
      <div class="sf-site-row">
        <div class="sf-site">
          <SiteFavicon :name="group.primary.site_name" :base-url="group.primary.base_url" :size="18" />
          <span class="sf-site-name">{{ group.primary.site_name }}</span>
        </div>
        <div class="sf-peers">
          <span class="up">↑ {{ group.primary.seeders ?? 0 }}</span>
          <span class="down">↓ {{ group.primary.leechers ?? 0 }}</span>
        </div>
      </div>
    </header>

    <div class="sf-card-body">
      <div class="sf-t-title" :title="group.primary.title">{{ group.primary.title }}</div>
      <div v-if="group.primary.subtitle" class="sf-t-sub text-muted" :title="group.primary.subtitle">
        {{ group.primary.subtitle }}
      </div>
      <div v-if="group.timeText" class="sf-time text-muted">⏱ {{ group.timeText }}</div>
      <div v-if="group.tags.length" class="sf-tags">
        <span v-for="tag in group.tags" :key="tag.label" class="sf-tag" :class="`tone-${tag.tone}`">{{
          tag.label
        }}</span>
      </div>
    </div>

    <footer class="sf-card-foot">
      <button v-if="group.more.length" type="button" class="sf-more" @click.stop="emit('more', group)">
        ▾ {{ t('tc.moreSources', { n: group.more.length }) }}
      </button>
      <span v-else />
      <div class="sf-foot-right">
        <span v-if="formatTorrentSize(group.primary.size)" class="sf-size">{{
          formatTorrentSize(group.primary.size)
        }}</span>
        <button
          v-if="group.primary.detail_url"
          type="button"
          class="sf-info"
          :title="t('tc.openDetail')"
          :aria-label="t('tc.openDetailAria')"
          @click.stop="emit('detail', group.primary)"
        >
          ⓘ
        </button>
      </div>
    </footer>
  </article>
</template>

<style scoped src="./torrentCard.css"></style>
