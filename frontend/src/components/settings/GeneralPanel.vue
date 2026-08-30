<script setup lang="ts">
// 通用分区内容：主题切换、版本/更新检查、帮助/反馈/更新日志入口与弹窗。
// 状态与逻辑来自 useGeneralSettings 组合式函数（与 Settings.vue 共享单例）。
import { ref } from 'vue'
import { useGeneralSettings } from '@/composables/useGeneralSettings'
import { useI18n } from 'vue-i18n'

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

// 界面定制（对标 MSaber 界面自定义面板）：主题色/风格/壁纸/遮罩/卡片风格，偏好随账号同步。
const { normalized, set, fetchBingDaily } = useUiPrefs()
const wallpaperFile = ref<File | null>(null)

function onWallpaperFile(f: File | null) {
  wallpaperFile.value = f
  if (!f) return
  // 本地文件转 base64 存偏好（64KB 服务端上限：超大图直接读会超，提示用网络地址）
  if (f.size > 48 * 1024) {
    ElMessage.warning(t('gp.wallpaperTooLarge'))
    return
  }
  const reader = new FileReader()
  reader.onload = () => set({ wallpaperKind: 'local', wallpaperUrl: String(reader.result) })
  reader.readAsDataURL(f)
}
</script>

<template>
  <div class="card glass">
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
          @click="set({ accent: p.value })"
        />
      </div>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.uiStyle') }}</span>
        <span class="text-muted row-hint">{{ t('gp.uiStyleHint') }}</span>
      </div>
      <el-select :model-value="normalized().uiStyle" style="width: 120px" @change="(v: string) => set({ uiStyle: v })">
        <el-option :label="t('gp.styleGlass')" value="glass" />
        <el-option :label="t('gp.styleModern')" value="modern" />
      </el-select>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.wallpaper') }}</span>
        <span class="text-muted row-hint">{{ t('gp.wallpaperHint') }}</span>
      </div>
      <el-select
        :model-value="normalized().wallpaperKind"
        style="width: 130px"
        @change="
          (v: string) => {
            if (v === 'bing') fetchBingDaily()
            set({ wallpaperKind: v })
          }
        "
      >
        <el-option :label="t('gp.wpNone')" value="none" />
        <el-option :label="t('gp.wpBing')" value="bing" />
        <el-option :label="t('gp.wpUrl')" value="url" />
        <el-option :label="t('gp.wpLocal')" value="local" />
      </el-select>
    </div>
    <div v-if="normalized().wallpaperKind === 'url'" class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.wallpaperUrl') }}</span>
        <span class="text-muted row-hint">{{ t('gp.wallpaperUrlHint') }}</span>
      </div>
      <el-input
        :model-value="normalized().wallpaperUrl"
        placeholder="https://example.com/bg.jpg"
        style="width: 320px"
        @change="(v: string) => set({ wallpaperUrl: v.trim() })"
      />
    </div>
    <div v-else-if="normalized().wallpaperKind === 'local'" class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.pickFile') }}</span>
        <span class="text-muted row-hint">{{ t('gp.pickFileHint') }}</span>
      </div>
      <input
        type="file"
        accept="image/*"
        @change="(e: Event) => onWallpaperFile((e.target as HTMLInputElement).files?.[0] ?? null)"
      />
    </div>
    <div v-if="normalized().wallpaperKind !== 'none'" class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.mask') }}</span>
        <span class="text-muted row-hint">{{ t('gp.maskHint') }}</span>
      </div>
      <el-select :model-value="normalized().mask" style="width: 120px" @change="(v: string) => set({ mask: v })">
        <el-option :label="t('gp.maskLight')" value="light" />
        <el-option :label="t('gp.maskStandard')" value="standard" />
        <el-option :label="t('gp.maskStrong')" value="strong" />
      </el-select>
    </div>
    <div class="row">
      <div class="row-label">
        <span class="row-title">{{ t('gp.cardStyle') }}</span>
        <span class="text-muted row-hint">{{ t('gp.cardStyleHint') }}</span>
      </div>
      <el-select
        :model-value="normalized().cardStyle"
        style="width: 120px"
        @change="(v: string) => set({ cardStyle: v })"
      >
        <el-option :label="t('gp.cardShadow')" value="shadow" />
        <el-option :label="t('gp.cardBorder')" value="border" />
      </el-select>
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
  <div class="card glass">
    <div class="row about">
      <div class="row-label">
        <span class="row-title">PTPatronus</span>
        <span class="text-muted row-hint">{{ t('gp.aboutHint') }}</span>
      </div>
      <div class="about-actions">
        <el-tag size="small" type="info" effect="plain">v{{ version }}</el-tag>
        <el-button size="small" :loading="updateChecking" @click="checkUpdate">{{ t('gp.checkUpdate') }}</el-button>
      </div>
    </div>
  </div>

  <!-- 帮助 / 反馈 / 更新日志 -->
  <div class="card glass help-card">
    <div v-a11y-button class="help-row" @click="helpVisible = true">
      <div class="row-label">
        <span class="row-title">{{ t('gp.help') }}</span>
        <span class="text-muted row-hint">{{ t('gp.helpHint') }}</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
    <div v-a11y-button class="help-row" @click="openIssues">
      <div class="row-label">
        <span class="row-title">{{ t('gp.feedback') }}</span>
        <span class="text-muted row-hint">{{ t('gp.feedbackHint') }}</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
    <div v-a11y-button class="help-row" @click="openChangelog">
      <div class="row-label">
        <span class="row-title">{{ t('gp.changelog') }}</span>
        <span class="text-muted row-hint">{{ t('gp.changelogHint') }}</span>
      </div>
      <span class="help-arrow">›</span>
    </div>
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
      <el-link v-else-if="updateInfo.downloadUrl" type="primary" :href="updateInfo.downloadUrl" target="_blank">{{
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
