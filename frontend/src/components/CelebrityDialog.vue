<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { CelebrityProfile } from '@/api/media'
import MediaPoster from '@/components/MediaPoster.vue'
// #405 豆瓣链接与作品链接都来自第三方 API 的响应，外部可控：Vue 的 :href 不过滤协议，
// javascript:/data: 一类能直接注入。统一走 safeUrl（单一来源，Savept/详情页等已用）。
import { safeUrl } from '@/utils/safeUrl'

const { t } = useI18n()

const visible = defineModel<boolean>('visible', { required: true })
const loading = defineModel<boolean>('loading', { required: true })
const profile = defineModel<CelebrityProfile | null>('profile', { required: true })
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="profile?.name || t('cel.title')"
    width="560px"
    class="cel-dialog"
    align-center
    append-to-body
  >
    <div v-loading="loading" class="cel-body">
      <template v-if="profile">
        <div class="cel-head">
          <div class="cel-avatar"><MediaPoster :url="profile.avatar || ''" kind="person" /></div>
          <div class="cel-headinfo">
            <div class="cel-name">{{ profile.name }}</div>
            <div v-if="profile.latin_name" class="cel-latin text-muted">{{ profile.latin_name }}</div>
            <div v-if="profile.short_info" class="cel-short text-muted">{{ profile.short_info }}</div>
            <div v-if="profile.followed_count" class="cel-follow text-muted">
              {{ profile.followed_count.toLocaleString('zh-CN') }} {{ t('cel.followers') }}
            </div>
          </div>
        </div>
        <div v-if="profile.info?.length" class="cel-info">
          <div v-for="(kv, i) in profile.info" :key="i" class="cel-inforow">
            <span class="cel-label text-muted">{{ kv[0] }}</span>
            <span class="cel-val">{{ kv[1] }}</span>
          </div>
        </div>
        <div v-if="profile.bio?.length" class="cel-section">
          <div class="cel-stitle">{{ t('cel.bio') }}</div>
          <p v-for="(para, i) in profile.bio" :key="i" class="cel-bio">{{ para }}</p>
        </div>
        <div v-if="profile.works?.length" class="cel-section">
          <div class="cel-stitle">{{ t('cel.works') }}</div>
          <div class="cel-works">
            <a
              v-for="(w, i) in profile.works"
              :key="i"
              :href="safeUrl(w.url)"
              target="_blank"
              rel="noopener"
              class="cel-work"
            >
              <span class="cel-wtitle">{{ w.title }}</span>
              <span v-if="w.role" class="cel-wrole text-muted">{{ w.role }}</span>
            </a>
          </div>
        </div>
        <div v-if="profile.douban_url" class="cel-footer">
          <a
            :href="safeUrl(profile.douban_url)"
            target="_blank"
            rel="noopener"
            class="text-muted"
            >{{ t('cel.douban') }} ↗</a
          >
        </div>
      </template>
    </div>
  </el-dialog>
</template>

<style scoped>
.cel-body {
  min-height: 120px;
}
.cel-head {
  display: flex;
  gap: var(--sp-4);
  margin-bottom: 16px;
}
.cel-avatar {
  width: 96px;
  height: 96px;
  flex: none;
  border-radius: 50%;
  overflow: hidden;
  background: var(--surface-muted);
}
.cel-headinfo {
  flex: 1;
  min-width: 0;
}
.cel-name {
  font-size: var(--fs-xl);
  font-weight: 700;
}
.cel-latin {
  font-size: var(--fs-body);
  margin-top: 2px;
}
.cel-short {
  font-size: var(--fs-body);
  line-height: 1.5;
  margin-top: 8px;
}
.cel-follow {
  font-size: var(--fs-xs);
  margin-top: 6px;
}
.cel-info {
  padding: 12px 0;
  border-top: 1px solid var(--border, rgba(128, 128, 128, 0.15));
  border-bottom: 1px solid var(--border, rgba(128, 128, 128, 0.15));
}
.cel-inforow {
  font-size: var(--fs-body);
  line-height: 1.6;
  display: flex;
  gap: var(--sp-3);
}
.cel-label {
  flex: none;
  width: 5em;
  color: var(--text-muted);
}
.cel-val {
  flex: 1;
  min-width: 0;
  word-break: break-word;
}
.cel-section {
  margin-top: 16px;
}
.cel-stitle {
  font-weight: 700;
  font-size: var(--fs-md);
  margin-bottom: 8px;
}
.cel-bio {
  font-size: var(--fs-body);
  line-height: 1.8;
  color: var(--text-muted);
  margin: 0 0 8px;
}
.cel-works {
  display: flex;
  flex-direction: column;
  gap: var(--sp-1);
}
.cel-work {
  display: flex;
  justify-content: space-between;
  gap: var(--sp-3);
  padding: 6px 8px;
  border-radius: var(--radius-xs);
  font-size: var(--fs-body);
  text-decoration: none;
  color: inherit;
}
.cel-work:hover {
  background: var(--surface-muted);
}
.cel-wtitle {
  font-weight: 600;
}
.cel-wrole {
  font-size: var(--fs-xs);
  text-align: right;
  color: var(--text-muted);
}
.cel-footer {
  margin-top: 16px;
  text-align: right;
  font-size: var(--fs-body);
}
.cel-footer a {
  text-decoration: none;
}
.cel-footer a:hover {
  text-decoration: underline;
}
</style>
