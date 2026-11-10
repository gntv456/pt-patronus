<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// OpsDocker 的概览卡片区（运行中/总数/镜像数等 tile；从 OpsDocker.vue 拆出）。
import { useI18n } from 'vue-i18n'
import type { DockerListResp } from '@/api/docker'

defineProps<{
  data: DockerListResp
  stats: unknown[]
}>()

const { t } = useI18n()
</script>

<template>
      <!-- 概览卡片 -->
      <div class="docker-stats">
        <div class="stat-tile glass stat--running">
          <span class="stat-ic"
            ><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z" /></svg
          ></span>
          <div class="stat-meta">
            <strong>{{ (data.containers || []).filter((c) => c.State === 'running').length }}</strong
            ><span>{{ t('opsdocker.running') }}</span>
          </div>
        </div>
        <div class="stat-tile glass stat--container">
          <span class="stat-ic"
            ><svg viewBox="0 0 24 24">
              <path d="M3 6h18v12H3z" fill="none" stroke="currentColor" stroke-width="2" />
              <path d="M7 6v12M12 6v12M17 6v12" stroke="currentColor" stroke-width="1.5" /></svg
          ></span>
          <div class="stat-meta">
            <strong>{{ data.containers.length }}</strong
            ><span>{{ t('opsdocker.totalContainers') }}</span>
          </div>
        </div>
        <div class="stat-tile glass stat--image">
          <span class="stat-ic"
            ><svg viewBox="0 0 24 24">
              <path d="M21 5H3v14h18V5zm-9 2c2.2 0 4 1.8 4 4s-1.8 4-4 4-4-1.8-4-4 1.8-4 4-4z" /></svg
          ></span>
          <div class="stat-meta">
            <strong>{{ data.images.length }}</strong
            ><span>{{ t('opsdocker.images') }}</span>
          </div>
        </div>
        <div class="stat-tile glass stat--volume">
          <span class="stat-ic"
            ><svg viewBox="0 0 24 24"><path d="M4 6h16l-1 14H5L4 6zm5 0V4h6v2" /></svg
          ></span>
          <div class="stat-meta">
            <strong>{{ data.volumes.length }}</strong
            ><span>{{ t('opsdocker.volumes') }}</span>
          </div>
        </div>
        <div class="stat-tile glass stat--network">
          <span class="stat-ic"
            ><svg viewBox="0 0 24 24">
              <path
                d="M12 2a10 10 0 100 20 10 10 0 000-20zm0 4a6 6 0 016 6M7 9a6 6 0 015-3"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
              />
              <circle cx="12" cy="12" r="2" /></svg
          ></span>
          <div class="stat-meta">
            <strong>{{ data.networks.length }}</strong
            ><span>{{ t('opsdocker.networks') }}</span>
          </div>
        </div>
      </div>
</template>
