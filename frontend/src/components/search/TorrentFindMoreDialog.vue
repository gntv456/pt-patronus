<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { formatTorrentSize, volumeFactorLabel } from '@/utils/torrentMeta'
import SiteFavicon from '@/components/SiteFavicon.vue'
import type { FindRow } from '@/components/SiteTorrentFindDialog.vue'

const { t } = useI18n()

const visible = defineModel<boolean>('visible', { required: true })

defineProps<{ items: FindRow[] }>()

const emit = defineEmits<{ (e: 'push', row: FindRow): void }>()

function promoOf(row: FindRow) {
  return volumeFactorLabel(row.uploadvolumefactor, row.downloadvolumefactor, row.free, [
    ...(row.tags || []),
    ...(row.labels || []),
  ])
}

function openDetail(row: FindRow) {
  if (row.detail_url) window.open(row.detail_url, '_blank', 'noopener')
}
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="t('tfd.otherSources')"
    width="min(420px, 92vw)"
    append-to-body
    align-center
    class="sf-more-dialog"
  >
    <div class="sf-more-list">
      <button
        v-for="(item, i) in items"
        :key="`${item.site_id}-${item.id || i}`"
        type="button"
        class="sf-more-row"
        @click="emit('push', item)"
      >
        <div class="sf-more-left">
          <SiteFavicon :name="item.site_name" :base-url="item.base_url" :size="16" />
          <span class="sf-more-name">{{ item.site_name }}</span>
          <span v-if="promoOf(item)" class="sf-promo-chip" :class="`tone-${promoOf(item)!.tone}`">{{
            promoOf(item)!.text
          }}</span>
        </div>
        <div class="sf-more-right">
          <span v-if="formatTorrentSize(item.size)" class="sf-size">{{ formatTorrentSize(item.size) }}</span>
          <span class="up">↑ {{ item.seeders ?? 0 }}</span>
          <span class="down">↓ {{ item.leechers ?? 0 }}</span>
          <span v-if="item.detail_url" class="sf-ext" :title="t('tfd.detail')" @click.stop="openDetail(item)">↗</span>
        </div>
      </button>
    </div>
  </el-dialog>
</template>

<style scoped src="./torrentFindMore.css"></style>
