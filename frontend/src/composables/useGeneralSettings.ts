// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 通用设置：主题切换、版本号、在线更新检查、帮助/反馈/更新日志。
// 从 Settings.vue 抽出，供 GeneralPanel 与父级共享。
import { ref } from 'vue'
import { i18n } from '@/i18n'
import { ElMessage } from 'element-plus'
import { systemApi, APP_VERSION, type UpdateCheckResult } from '@/api/system'
import { dashboardApi } from '@/api/dashboard'
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

// 复制脱敏诊断信息（ADR-003：不引入默认遥测，改为用户显式复制后自行粘贴到 issue）。
// 只采集结构性计数与版本/平台，绝不收集 Cookie、站点 URL、用户名或任何凭据。
// 纯函数：生成脱敏诊断文本（便于单测钉死脱敏契约）。
export function buildDiagnosticsLines(
  ver: string,
  info: { version?: string } | null,
  overview: { site_count?: number; downloader_count?: number } | null,
): string[] {
  return [
    'PTPatronus diagnostics (redacted)',
    'version: ' + (ver || info?.version || APP_VERSION),
    'platform: web',
    'site_count: ' + (overview?.site_count ?? 'n/a'),
    'downloader_count: ' + (overview?.downloader_count ?? 'n/a'),
    'generated_at: ' + new Date().toISOString(),
  ]
}
const diagCopying = ref(false)
async function copyDiagnostics() {
  diagCopying.value = true
  try {
    const [info, overview] = await Promise.all([
      systemApi.info().catch(() => null),
      dashboardApi.overview().catch(() => null),
    ])
    await copyText(buildDiagnosticsLines(version.value, info, overview).join('\n'))
    ElMessage.success(t('opusegeneralsettings.diagCopied'))
  } catch {
    ElMessage.error(t('opusegeneralsettings.diagFailed'))
  } finally {
    diagCopying.value = false
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

// 反馈/更新日志外链必须与仓库真实地址一致（remote: gntv456/pt-patronus）。
// 旧值 hxpt/ptpatronus 已 404，曾导致「问题反馈」跳空页（运营漏斗断点）。
const ISSUES_URL = 'https://github.com/gntv456/pt-patronus/issues'
const README_URL = 'https://github.com/gntv456/pt-patronus#readme'

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
    diagCopying,
    copyDiagnostics,
    toggleTheme,
    openIssues,
    openChangelog,
  }
}
