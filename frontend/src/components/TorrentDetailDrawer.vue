<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
/**
 * 种子详情抽屉：抓取并解析单条种子详情页（简介/截图/MediaInfo/文件列表）。
 * 受控显隐（v-model:visible）；详情数据与加载态由父组件传入。
 */
import { useI18n } from 'vue-i18n'
import { safeUrl } from '@/utils/safeUrl'
import { copyText } from '@/utils/clipboard'
import DOMPurify from 'dompurify'
import MediaPoster from '@/components/MediaPoster.vue'
import type { TorrentDetail } from '@/api/seedbrowse'

const { t } = useI18n()

const visible = defineModel<boolean>('visible', { required: true })
defineProps<{
  detail: TorrentDetail | null
  loading: boolean
}>()

// 种子简介来自 PT 站原始 HTML，渲染前必须净化（防存储型 XSS：注入 onerror/script 窃 token）。
const sanitizeHtml = (html?: string) => (html ? DOMPurify.sanitize(html) : '')

// 下载链接：打开 + 复制（详情页主操作，比小 el-link 更显眼）
function openDl(d: TorrentDetail | null) {
  if (d?.download_url) window.open(d.download_url, '_blank', 'noopener')
}
async function copyDl(d: TorrentDetail | null) {
  if (!d?.download_url) return
  await copyText(d.download_url, t('tddr.linkCopied'), t('tddr.copyFailed'))
}
function imdbUrl(id: string) {
  return id ? `https://www.imdb.com/title/${id}/` : ''
}
</script>

<template>
  <el-drawer v-model="visible" :title="t('tddr.title')" size="50%" direction="rtl">
    <div v-loading="loading" class="detail-body">
      <template v-if="detail">
        <!-- 头部：海报 + 标题/副标题 + 规格彩签 + 下载操作 -->
        <div class="detail-head glass">
          <div v-if="detail.poster_url" class="dposter"><MediaPoster :url="detail.poster_url" kind="movie" /></div>
          <div class="meta">
            <h3 class="dt-title">{{ detail.title }}</h3>
            <div v-if="detail.subtitle" class="dt-sub text-muted">{{ detail.subtitle }}</div>
            <div class="specs">
              <span v-if="detail.category" class="pt-chip pt-chip--muted">{{ detail.category }}</span>
              <span v-if="detail.resolution" class="pt-chip pt-chip--info">{{ detail.resolution }}</span>
              <span v-if="detail.codec" class="pt-chip pt-chip--muted">{{ detail.codec }}</span>
              <span v-if="detail.source" class="pt-chip pt-chip--muted">{{ detail.source }}</span>
              <span v-if="detail.size" class="pt-chip pt-chip--muted">{{ detail.size }}</span>
              <span v-if="detail.seeders !== undefined" class="pt-chip pt-chip--success">↑ {{ detail.seeders }}</span>
              <a
                v-if="detail.imdb_id"
                :href="imdbUrl(detail.imdb_id)"
                target="_blank"
                rel="noopener"
                class="chip imdb"
                >{{ detail.imdb_id }}</a
              >
            </div>
            <div v-if="detail.download_url" class="dl-row">
              <el-button type="primary" @click="openDl(detail)">⬇ {{ t('trt.download') }}</el-button>
              <el-button @click="copyDl(detail)">{{ t('tddr.copyLink') }}</el-button>
            </div>
          </div>
        </div>

        <div v-if="detail.screenshots?.length" class="card glass">
          <div class="sec-title">
            {{ t('tddr.screenshots') }} <span class="text-muted">({{ detail.screenshots.length }})</span>
          </div>
          <div class="shots">
            <a
              v-for="(s, i) in detail.screenshots"
              :key="i"
              :href="safeUrl(s)"
              target="_blank"
              rel="noopener"
              class="shot"
              ><MediaPoster :url="s" kind="movie"
            /></a>
          </div>
        </div>

        <div v-if="detail.description" class="card glass">
          <div class="sec-title">{{ t('tddr.desc') }}</div>
          <div class="desc" v-html="sanitizeHtml(detail.description)"></div>
        </div>

        <div v-if="detail.mediainfo" class="card glass">
          <div class="sec-title">MediaInfo</div>
          <pre class="mediainfo">{{ detail.mediainfo }}</pre>
        </div>

        <div v-if="detail.file_list?.length" class="card glass">
          <div class="sec-title">
            {{ t('tddr.files') }} <span class="text-muted">({{ detail.file_list.length }})</span>
          </div>
          <div class="file-list">
            <div v-for="(f, i) in detail.file_list" :key="i" class="file-row">
              <span class="file-ic">📄</span>{{ f }}
            </div>
          </div>
        </div>
      </template>
    </div>
  </el-drawer>
</template>

<style scoped>
/* 种子详情抽屉 */
.detail-body {
  padding: 0 2px;
  display: flex;
  flex-direction: column;
  gap: var(--sp-3);
}
.detail-head {
  display: flex;
  gap: var(--sp-4);
  padding: 16px;
  border-radius: var(--radius-sm);
}
.dposter {
  width: 120px;
  height: 170px;
  border-radius: var(--radius-sm);
  overflow: hidden;
  flex: none;
  background: var(--surface-muted);
  box-shadow: var(--shadow-md);
}
.detail-head .meta {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--sp-2);
}
.dt-title {
  margin: 0;
  font-size: var(--fs-xl);
  font-weight: 700;
  line-height: 1.35;
  word-break: break-word;
}
.dt-sub {
  font-size: var(--fs-xs);
  line-height: 1.4;
}
.specs {
  display: flex;
  flex-wrap: wrap;
  gap: var(--sp-1);
}
.specs .pt-chip {
  margin: 0;
  font-weight: 600;
}
.specs .chip.imdb {
  background: rgba(247, 197, 41, 0.18);
  color: #c6a000;
  text-decoration: none;
  cursor: pointer;
}
.specs .chip.imdb:hover {
  filter: brightness(1.05);
}
.dl-row {
  display: flex;
  gap: var(--sp-2);
  margin-top: 4px;
}
.card {
  padding: 16px;
  border-radius: var(--radius-sm);
}
.sec-title {
  font-weight: 700;
  font-size: var(--fs-md);
  margin-bottom: 10px;
  color: var(--text-strong);
}
.shots {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--sp-2);
}
.shot {
  display: block;
  aspect-ratio: 16/9;
  border-radius: var(--radius-xs);
  overflow: hidden;
  background: var(--surface-muted);
  transition: transform var(--dur-fast) var(--ease-out-smooth);
}
.shot:hover {
  transform: scale(1.02);
}
.desc {
  line-height: 1.7;
  font-size: var(--fs-md);
  max-height: 520px;
  overflow: auto;
  padding-right: 4px;
  word-break: break-word;
}
.desc :deep(img) {
  max-width: 100%;
  height: auto;
  border-radius: var(--radius-xs);
  margin: 4px 0;
}
.desc :deep(a) {
  color: var(--pt-primary);
}
.desc :deep(table) {
  max-width: 100%;
  border-collapse: collapse;
}
.mediainfo {
  background: rgba(0, 0, 0, 0.78);
  color: #d4d4d4;
  padding: 12px;
  border-radius: var(--radius-xs);
  font-size: var(--fs-xs);
  line-height: 1.5;
  font-family: ui-monospace, 'Cascadia Code', monospace;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 320px;
  overflow: auto;
}
.file-list {
  max-height: 280px;
  overflow: auto;
  font-size: var(--fs-xs);
  font-family: ui-monospace, monospace;
  border-radius: var(--radius-xs);
  background: var(--surface-muted);
}
.file-row {
  padding: 5px 10px;
  border-bottom: 1px solid rgba(0, 0, 0, 0.05);
  display: flex;
  gap: var(--sp-1);
  align-items: baseline;
  word-break: break-all;
}
.file-row:last-child {
  border-bottom: none;
}
.file-ic {
  flex: none;
  opacity: 0.6;
}
</style>
