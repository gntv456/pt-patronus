<!--
  SPDX-License-Identifier: AGPL-3.0-or-later
  Copyright (C) 2026 The PTPatronus Authors
-->

<script setup lang="ts">
// 通用分区内容：主题切换、版本/更新检查、帮助/反馈/更新日志入口与弹窗。
// 状态与逻辑来自 useGeneralSettings 组合式函数（与 Settings.vue 共享单例）。
import { useGeneralSettings } from '@/composables/useGeneralSettings'
import { useI18n } from 'vue-i18n'
import SptIcon from '@/components/SptIcon.vue'
// #405 下载地址来自环境变量/更新接口，仍走协议白名单（收口一致，勿让某处裸绑 href）。
import { safeUrl } from '@/utils/safeUrl'

const { t } = useI18n()
import { useUiPrefs, ACCENT_PRESETS } from '@/composables/useUiPrefs'

const {
  dark,
  version,
  updateChecking,
  updateInfo,
  updateDlgVisible,
  helpVisible,
  checkUpdate,
  copyUpgradeCmd,
  toggleTheme,
  openIssues,
  openChangelog,
} = useGeneralSettings()

// 界面定制：仅保留主题色与侧栏默认态（偏好随账号同步）。
const { normalized, set } = useUiPrefs()
</script>

<template>
  <div class="card solid-card">
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.darkTheme') }}</span>
        <span class="text-muted row-hint">{{ t('gp.darkHint') }}</span>
      </div>
      <el-switch :model-value="dark" @change="toggleTheme" />
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.accent') }}</span>
        <span class="text-muted row-hint">{{ t('gp.accentHint') }}</span>
      </div>
      <div class="accent-picker">
        <button
          v-for="p in ACCENT_PRESETS"
          :key="p.value"
          class="accent-dot"
          :class="{ active: normalized().accent === p.value }"
          :style="{ background: p.color }"
          :title="p.label"
          :aria-label="p.label"
          :aria-pressed="normalized().accent === p.value"
          @click="set({ accent: p.value })"
        />
      </div>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.sidebarCollapsed') }}</span>
        <span class="text-muted row-hint">{{ t('gp.sidebarHint') }}</span>
      </div>
      <el-switch
        :model-value="normalized().sidebarCollapsed"
        @change="(v: string | number | boolean) => set({ sidebarCollapsed: !!v })"
      />
    </div>
  </div>
  <div class="card solid-card">
    <div class="row about">
      <div class="row-label">
        <span class="row-title">PTPatronus</span>
        <span class="text-muted row-hint">{{ t('gp.aboutHint') }}</span>
      </div>
      <div class="about-actions">
        <el-tag size="small" type="info" effect="plain">v{{ version }}</el-tag>
        <!-- 浏览器扩展包：后端 embed 分发（/system/extension.zip），Docker 用户
             无需进仓库找 extension/ 目录；下载后解压按快速开始加载。 -->
        <el-button size="small" tag="a" href="/api/v1/system/extension.zip" download="ptpatronus-extension.zip">
          {{ t('gp.downloadExtension') }}
        </el-button>
        <el-button size="small" :loading="updateChecking" @click="checkUpdate">{{ t('gp.checkUpdate') }}</el-button>
      </div>
    </div>
  </div>

  <!-- 帮助 / 反馈 / 更新日志 -->
  <div class="card solid-card help-card">
    <button type="button" class="help-row" @click="helpVisible = true">
      <span class="row-label">
        <span class="row-title">{{ t('gp.help') }}</span>
        <span class="text-muted row-hint">{{ t('gp.helpHint') }}</span>
      </span>
      <SptIcon name="chevron-right" :size="16" class="help-arrow" aria-hidden="true" />
    </button>
    <button type="button" class="help-row" @click="openIssues">
      <span class="row-label">
        <span class="row-title">{{ t('gp.feedback') }}</span>
        <span class="text-muted row-hint">{{ t('gp.feedbackHint') }}</span>
      </span>
      <SptIcon name="chevron-right" :size="16" class="help-arrow" aria-hidden="true" />
    </button>
    <button type="button" class="help-row" @click="openChangelog">
      <span class="row-label">
        <span class="row-title">{{ t('gp.changelog') }}</span>
        <span class="text-muted row-hint">{{ t('gp.changelogHint') }}</span>
      </span>
      <SptIcon name="chevron-right" :size="16" class="help-arrow" aria-hidden="true" />
    </button>
  </div>

  <!-- 在线更新检查弹窗 -->
  <el-dialog
    class="pt-dialog"
    append-to-body
    align-center
    v-model="updateDlgVisible"
    :title="t('gp.newVersion', { v: updateInfo?.latestVersion ?? '' })"
    width="560px"
  >
    <div v-if="updateInfo" class="update-body">
      <p class="update-ver">
        {{ t('gp.versionLine', { cur: updateInfo.currentVersion, latest: updateInfo.latestVersion })
        }}<span v-if="updateInfo.isBelowMin" class="update-forced">{{ t('gp.versionTooLow') }}</span>
      </p>
      <div v-if="updateInfo.releaseName" class="update-name">{{ updateInfo.releaseName }}</div>
      <div v-if="updateInfo.releaseNotes" class="update-notes">{{ updateInfo.releaseNotes }}</div>
      <template v-if="updateInfo.instructions">
        <div class="update-cmd-label">{{ t('gp.upgradeCmd') }}</div>
        <pre class="update-cmd">{{ updateInfo.instructions }}</pre>
        <el-button size="small" @click="copyUpgradeCmd">{{ t('gp.copyCmd') }}</el-button>
      </template>
      <el-link v-else-if="updateInfo.downloadUrl" type="primary" :href="safeUrl(updateInfo.downloadUrl)" target="_blank">{{
        t('gp.downloadUpdate')
      }}</el-link>
    </div>
    <template #footer>
      <el-button @click="updateDlgVisible = false">{{
        updateInfo?.isForced ? t('gp.acknowledged') : t('gp.later')
      }}</el-button>
    </template>
  </el-dialog>

  <!-- 使用帮助 -->
  <el-dialog class="pt-dialog" append-to-body align-center v-model="helpVisible" :title="t('gp.help')" width="560px">
    <div class="help-body">
      <h4>{{ t('gp.coreValue') }}</h4>
      <p>{{ t('gp.intro') }}</p>
      <ol>
        <li>{{ t('gp.step1') }}</li>
        <li>{{ t('gp.step2') }}</li>
        <li>{{ t('gp.step3') }}</li>
      </ol>

      <h4>{{ t('gp.jargonTitle') }}</h4>
      <ul class="jargon">
        <li>
          <b>{{ t('gp.j.reseed') }}</b
          >{{ t('gp.jd.reseed') }}
        </li>
        <li>
          <b>{{ t('gp.j.icu') }}</b
          >{{ t('gp.jd.icu') }}
        </li>
        <li>
          <b>{{ t('gp.j.harem') }}</b
          >{{ t('gp.jd.harem') }}
        </li>
        <li>
          <b>{{ t('gp.j.free') }}</b
          >{{ t('gp.jd.free') }}
        </li>
        <li>
          <b>{{ t('gp.j.assess') }}</b
          >{{ t('gp.jd.assess') }}
        </li>
        <li><b>H&amp;R</b>{{ t('gp.jd.hr') }}</li>
      </ul>
    </div>
    <template #footer>
      <el-button @click="helpVisible = false">{{ t('gp.gotIt') }}</el-button>
    </template>
  </el-dialog>
</template>

<style scoped src="./GeneralPanel.css"></style>
