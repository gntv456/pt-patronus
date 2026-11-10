<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// OpsDocker 的镜像/卷/网络三张表（从 OpsDocker.vue 拆出；行为不变）。
import { useI18n } from 'vue-i18n'
import type { DockerListResp, DockerImage, DockerVolume, DockerNetwork } from '@/api/docker'

defineProps<{
  activeTab: string
  mode: 'local' | 'remote'
  data: DockerListResp
}>()

const emit = defineEmits<{
  openPull: []
  prune: [kind: 'image' | 'volume' | 'network']
  removeImage: [img: DockerImage]
  createVolume: []
  removeVolume: [v: DockerVolume]
  createNetwork: []
  removeNetwork: [n: DockerNetwork]
}>()

function netDriverClass(driver: string): string {
  const d = (driver || '').toLowerCase()
  if (['bridge', 'vlan', 'host', 'overlay'].includes(d)) return 'driver-chip--' + d
  return ''
}

const { t } = useI18n()
</script>

<template>
      <!-- 镜像列表 -->
      <div v-if="activeTab === 'images'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="emit('openPull')">{{ t('opsdocker.pullImage') }}</el-button>
          <el-button size="small" type="warning" @click="emit('prune', 'image')">{{ t('opsdocker.pruneImages') }}</el-button>
        </div>
        <table>
          <thead>
            <tr>
              <th>{{ t('opsdocker.repo') }}</th>
              <th>{{ t('opsdocker.tag') }}</th>
              <th>ID</th>
              <th>{{ t('cloud.size') }}</th>
              <th>{{ t('common.action') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(img, i) in data.images" :key="i">
              <td>{{ img.Repository }}</td>
              <td>
                <span class="pt-chip">{{ img.Tag }}</span>
              </td>
              <td class="td-mono">{{ img.ID }}</td>
              <td>{{ img.Size }}</td>
              <td>
                <button class="mini-btn danger" @click="emit('removeImage', img)">
                  <svg viewBox="0 0 24 24" class="btn-ico">
                    <path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z" />
                  </svg>
                </button>
              </td>
            </tr>
            <tr v-if="!data.images.length">
              <td colspan="5" class="td-empty">{{ t('opsdocker.noImages') }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 卷列表 -->
      <div v-if="activeTab === 'volumes'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="emit('createVolume')">{{ t('opsdocker.createVolume') }}</el-button>
          <el-button size="small" type="warning" @click="emit('prune', 'volume')">{{
            t('opsdocker.pruneVolumes')
          }}</el-button>
        </div>
        <table>
          <thead>
            <tr>
              <th>{{ t('common.name') }}</th>
              <th>{{ t('opsdocker.driver') }}</th>
              <th v-if="mode === 'local'">{{ t('common.action') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(v, i) in data.volumes" :key="i">
              <td class="td-mono">{{ v.Name }}</td>
              <td>{{ v.Driver }}</td>
              <td v-if="mode === 'local'">
                <button class="mini-btn danger" @click="emit('removeVolume', v)">
                  <svg viewBox="0 0 24 24" class="btn-ico">
                    <path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z" />
                  </svg>
                </button>
              </td>
            </tr>
            <tr v-if="!data.volumes.length">
              <td :colspan="mode === 'local' ? 3 : 2" class="td-empty">{{ t('opsdocker.noVolumes') }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 网络列表 -->
      <div v-if="activeTab === 'networks'" class="docker-table glass">
        <div v-if="mode === 'local'" class="tab-toolbar">
          <el-button size="small" type="primary" @click="emit('createNetwork')">{{ t('opsdocker.createNetwork') }}</el-button>
          <el-button size="small" type="warning" @click="emit('prune', 'network')">{{
            t('opsdocker.pruneNetworks')
          }}</el-button>
        </div>
        <table>
          <thead>
            <tr>
              <th>{{ t('common.name') }}</th>
              <th>{{ t('opsdocker.driver') }}</th>
              <th>{{ t('opsdocker.scope') }}</th>
              <th v-if="mode === 'local'">{{ t('common.action') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(n, i) in data.networks" :key="i">
              <td class="td-name">
                <span class="net-name">{{ n.Name }}</span>
              </td>
              <td>
                <span class="driver-chip" :class="netDriverClass(n.Driver)">{{ n.Driver }}</span>
              </td>
              <td>
                <span class="scope-chip">{{ n.Scope }}</span>
              </td>
              <td v-if="mode === 'local'">
                <button class="mini-btn danger" :title="t('opsdocker.removeNetwork')" @click="emit('removeNetwork', n)">
                  <svg viewBox="0 0 24 24" class="btn-ico">
                    <path d="M9 3h6v2h5v2H4V5h5V3zm-3 6h12l-1 12H7L6 9zm3 2v8H8v-8h1zm3 0v8h-1v-8h1z" />
                  </svg>
                </button>
              </td>
            </tr>
            <tr v-if="!data.networks.length">
              <td :colspan="mode === 'local' ? 4 : 3" class="td-empty">{{ t('opsdocker.noNetworks') }}</td>
            </tr>
          </tbody>
        </table>
      </div>
</template>
