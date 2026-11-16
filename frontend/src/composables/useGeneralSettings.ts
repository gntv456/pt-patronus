// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 通用设置：主题切换、版本号、在线更新检查、帮助/反馈/更新日志。
// 从 Settings.vue 抽出，供 GeneralPanel 与父级共享。
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage } from 'element-plus'
import { systemApi, APP_VERSION, type UpdateCheckResult } from '@/api/system'
import { copyText } from '@/utils/clipboard'
import { useUiPrefs } from '@/composables/useUiPrefs'

const dark = ref(false)
// 版本号：优先读后端 /system/info（部署态真源），失败兜底构建期常量。
const version = ref(APP_VERSION)

// 在线更新检查（服务端自检：platform=docker，有新版则弹对话框展示升级命令）
const updateChecking = ref(false)
const updateInfo = ref<UpdateCheckResult | null>(null)
const updateDlgVisible = ref(false)

function initThemeVersion() {
  dark.value = document.documentElement.classList.contains('dark')
  systemApi
    .info()
    .then((i) => {
      if (i?.version) version.value = i.version
    })
    .catch(() => {})
}

async function checkUpdate() {
  updateChecking.value = true
  try {
    const res = await systemApi.checkUpdate('docker')
    updateInfo.value = res
    if (res.hasUpdate) updateDlgVisible.value = true
    else ElMessage.success(t('opusegeneralsettings.k1'))
  } catch {
    ElMessage.error(t('opusegeneralsettings.k2'))
  } finally {
    updateChecking.value = false
  }
}

async function copyUpgradeCmd() {
  if (updateInfo.value?.instructions) {
    await copyText(updateInfo.value.instructions)
    ElMessage.success(t('opusegeneralsettings.k3'))
  }
}

function toggleTheme() {
  dark.value = document.documentElement.classList.toggle('dark')
  localStorage.setItem('theme', dark.value ? 'dark' : 'light')
  // 同步写偏好域（pref.ui.theme）：单一真源对齐。否则点调色盘等任何 pref.ui
  // 字段变更经 update() 重写整对象时，遗留的 theme:"light" 会经 watch(apply)
  // 把 dark 类摘掉——深色开关被调色盘「自动关闭」（实测复现）。
  // useUiPrefs() 单例缓存，此处拿到的是同一 prefs 引用，set 内部 update+apply。
  const { set } = useUiPrefs()
  set({ theme: dark.value ? 'dark' : 'light' })
}

// —— 帮助 / 反馈 / 更新日志 ——
const helpVisible = ref(false)

const ISSUES_URL = 'https://github.com/hxpt/ptpatronus/issues'
const README_URL = 'https://github.com/hxpt/ptpatronus#readme'

function openIssues() {
  window.open(ISSUES_URL, '_blank', 'noopener')
}

// 更新日志：优先展示已检查到的 releaseNotes，否则触发一次检查；都没有则跳 README。
async function openChangelog() {
  if (!updateInfo.value?.releaseNotes) {
    await checkUpdate()
  }
  if (updateInfo.value?.releaseNotes) {
    updateDlgVisible.value = true
  } else {
    window.open(README_URL, '_blank', 'noopener')
  }
}

const t = (k: string) => i18n.global.t(k)

export function useGeneralSettings() {
  return {
    dark,
    version,
    updateChecking,
    updateInfo,
    updateDlgVisible,
    helpVisible,
    initThemeVersion,
    checkUpdate,
    copyUpgradeCmd,
    toggleTheme,
    openIssues,
    openChangelog,
  }
}
