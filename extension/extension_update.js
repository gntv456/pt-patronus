// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 扩展更新检查：复用服务器已有的 /system/update/check 端点（零后端改动）。
// 动机（借鉴 MoviePilot-Tools 的 extension-update，但分发模型不同）：扩展无
// update_url（Chrome 商店外手动装），1.03.21 后 bundle 过期的教训之一就是
// 「用户拿着旧扩展没有感知」——服务器 24h 节流检查一次，有新版时弹通知 +
// popup/关于页提示，用户不必常来关于页看版本号。
// platform 参数**留空**：Check 对已知平台有「targets 未发布下载 → HasUpdate=false」
// 的防死链规则，留空即纯版本对比，不受 targets 分发影响。
// 下载地址不走该端点（targets 里没有扩展条目）——固定用 /system/extension.zip。

import { baseUrl } from './util.js'

// 结果落 storage.local 的键（popup 顶条/关于页读同一份）。
export const KEY_UPDATE = 'extension_update'
const KEY = KEY_UPDATE
const CHECK_ALARM = 'ptp-ext-update-check'
const CHECK_INTERVAL_MIN = 24 * 60

export async function getUpdateState() {
  try {
    const v = await chrome.storage.local.get(KEY)
    return (v && v[KEY]) || null
  } catch {
    return null
  }
}

// checkNow 立即检查一轮并把结果落 storage（popup/关于页读这份结果）。
// 不抛错——检查失败是常态（没登录/没配服务器），结果里带 error 供界面说明。
export async function checkNow(cfg, api) {
  const ver = (chrome.runtime.getManifest && chrome.runtime.getManifest().version) || ''
  const st = { at: Date.now(), version: ver, hasUpdate: false, latest: '', name: '', notes: '', error: '', server: '' }
  try {
    if (!cfg || !cfg.server || !cfg.token) {
      st.error = 'unconfigured'
      await chrome.storage.local.set({ [KEY]: st })
      return st
    }
    st.server = cfg.server
    const body = await api(cfg, `/api/v1/system/update/check?version=${encodeURIComponent(ver)}`)
    st.hasUpdate = !!(body && body.hasUpdate)
    st.latest = (body && body.latestVersion) || ''
    st.name = (body && body.releaseName) || ''
    st.notes = (body && body.releaseNotes) || ''
    if (st.hasUpdate && !st.latest) st.error = 'bad-payload' // 有更新却没版本号：数据不可信，不当更新报
  } catch (e) {
    st.error = String((e && e.message) || e)
  }
  await chrome.storage.local.set({ [KEY]: st })
  return st
}

// maybeCheckDaily 24h 节流：距上次成功检查不足一天就跳过（SW 每次唤醒都检查会打爆端点）。
// 失败结果也算「检查过了」——失败状态下每 24h 才重试一次，避免每轮唤醒都撞错误。
export async function maybeCheckDaily(cfg, api) {
  const prev = await getUpdateState()
  if (prev && prev.at && Date.now() - prev.at < CHECK_INTERVAL_MIN * 60 * 1000) return prev
  return checkNow(cfg, api)
}

// notifyIfNeeded 有新版时弹一条浏览器通知；当天去重（同一版只提醒一次）。
// 仿 autosync.notifyDead 的当天去重口径：storage 记「日期 → 已提醒版本」。
export async function notifyIfNeeded(st) {
  if (!st || !st.hasUpdate || !st.latest) return
  const k = `${KEY}_notified`
  const { [k]: notified = {} } = await chrome.storage.local.get(k)
  const day = new Date().toISOString().slice(0, 10)
  if (notified[day] === st.latest) return
  await chrome.storage.local.set({ [k]: { [day]: st.latest } })
  try {
    chrome.notifications.create({
      type: 'basic',
      iconUrl: chrome.runtime.getURL('icons/icon128.png'),
      title: 'PTPatronus 扩展有新版本',
      message: `当前 ${st.version}，最新 ${st.latest}——到扩展设置页「关于」查看并下载。`,
    })
  } catch { /* 通知失败不影响主流程 */ }
}

// downloadUrl 扩展包下载地址（guarded：浏览器已登录服务器时直接可下）。
export function downloadUrl(server) {
  return `${baseUrl({ server })}/api/v1/system/extension.zip`
}

// initExtensionUpdate background 装配：启动检查一轮 + 每 24h 闹钟兜底（SW 长睡后
// alarms 由浏览器补触发，比单纯依赖 onStartup 可靠）。
export function initExtensionUpdate() {
  const run = async () => {
    try {
      const { getConfig, api } = await import(chrome.runtime.getURL('api.js'))
      const st = await maybeCheckDaily(await getConfig(), api)
      await notifyIfNeeded(st)
    } catch { /* 检查失败静默——结果已带 error */ }
  }
  try {
    chrome.runtime.onStartup.addListener(run)
    chrome.alarms.create(CHECK_ALARM, { periodInMinutes: CHECK_INTERVAL_MIN, delayInMinutes: 1 })
    chrome.alarms.onAlarm.addListener((alarm) => {
      if (alarm.name === CHECK_ALARM) run()
    })
  } catch { /* 测试环境无 alarms */ }
}
