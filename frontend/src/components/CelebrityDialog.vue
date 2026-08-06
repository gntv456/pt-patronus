<script setup lang="ts">
import type { CelebrityProfile } from '@/api/media'
import MediaPoster from '@/components/MediaPoster.vue'

const visible = defineModel<boolean>('visible', { required: true })
const loading = defineModel<boolean>('loading', { required: true })
const profile = defineModel<CelebrityProfile | null>('profile', { required: true })
</script>

<template>
  <el-dialog v-model="visible" :title="profile?.name || '演员简介'" width="560px" class="cel-dialog" align-center append-to-body>
    <div v-loading="loading" class="cel-body">
      <template v-if="profile">
        <div class="cel-head">
          <div class="cel-avatar"><MediaPoster :url="profile.avatar || ''" kind="person" /></div>
          <div class="cel-headinfo">
            <div class="cel-name">{{ profile.name }}</div>
            <div v-if="profile.latin_name" class="cel-latin text-muted">{{ profile.latin_name }}</div>
            <div v-if="profile.short_info" class="cel-short text-muted">{{ profile.short_info }}</div>
            <div v-if="profile.followed_count" class="cel-follow text-muted">{{ profile.followed_count.toLocaleString('zh-CN') }} 人关注</div>
          </div>
        </div>
        <div v-if="profile.info?.length" class="cel-info">
          <div v-for="(kv, i) in profile.info" :key="i" class="cel-inforow">
            <span class="cel-label text-muted">{{ kv[0] }}</span>
            <span class="cel-val">{{ kv[1] }}</span>
          </div>
        </div>
        <div v-if="profile.bio?.length" class="cel-section">
          <div class="cel-stitle">生平</div>
          <p v-for="(para, i) in profile.bio" :key="i" class="cel-bio">{{ para }}</p>
        </div>
        <div v-if="profile.works?.length" class="cel-section">
          <div class="cel-stitle">参演作品</div>
          <div class="cel-works">
            <a v-for="(w, i) in profile.works" :key="i" :href="w.url" target="_blank" rel="noopener" class="cel-work">
              <span class="cel-wtitle">{{ w.title }}</span>
              <span v-if="w.role" class="cel-wrole text-muted">{{ w.role }}</span>
            </a>
          </div>
        </div>
        <div v-if="profile.douban_url" class="cel-footer">
          <a :href="profile.douban_url" target="_blank" rel="noopener" class="text-muted">查看豆瓣页面 ↗</a>
        </div>
      </template>
    </div>
  </el-dialog>
</template>

<style scoped>
.cel-body { min-height: 120px; }
.cel-head { display: flex; gap: 16px; margin-bottom: 16px; }
.cel-avatar { width: 96px; height: 96px; flex: none; border-radius: 50%; overflow: hidden; background: var(--surface-muted); }
.cel-headinfo { flex: 1; min-width: 0; }
.cel-name { font-size: 18px; font-weight: 700; }
.cel-latin { font-size: 13px; margin-top: 2px; }
.cel-short { font-size: 13px; line-height: 1.5; margin-top: 8px; }
.cel-follow { font-size: 12px; margin-top: 6px; }
.cel-info { padding: 12px 0; border-top: 1px solid var(--border, rgba(128,128,128,0.15)); border-bottom: 1px solid var(--border, rgba(128,128,128,0.15)); }
.cel-inforow { font-size: 13px; line-height: 1.6; display: flex; gap: 12px; }
.cel-label { flex: none; width: 5em; color: var(--text-muted); }
.cel-val { flex: 1; min-width: 0; word-break: break-word; }
.cel-section { margin-top: 16px; }
.cel-stitle { font-weight: 700; font-size: 14px; margin-bottom: 8px; }
.cel-bio { font-size: 13px; line-height: 1.8; color: var(--text-muted); margin: 0 0 8px; }
.cel-works { display: flex; flex-direction: column; gap: 2px; }
.cel-work { display: flex; justify-content: space-between; gap: 12px; padding: 6px 8px; border-radius: 6px; font-size: 13px; text-decoration: none; color: inherit; }
.cel-work:hover { background: var(--surface-muted); }
.cel-wtitle { font-weight: 600; }
.cel-wrole { font-size: 12px; text-align: right; color: var(--text-muted); }
.cel-footer { margin-top: 16px; text-align: right; font-size: 13px; }
.cel-footer a { text-decoration: none; }
.cel-footer a:hover { text-decoration: underline; }
</style>
