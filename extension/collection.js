// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 跨站收藏 + 下载历史（chrome.storage.local，仅本机；对标 PT-depiler my-collection / 下载历史）。
// 收藏：任意种子详情页经 background 消息写入 {url,title,site,added_at}，上限 500 条 FIFO。
// 历史：pushDownload 成功后追加 {title?,url,downloader_id,added_at}，上限 300 条。
const FAV_KEY = 'ptp_favorites'
const HIS_KEY = 'ptp_history'

// #338 读不到 / 数据不是数组（损坏、旧版残留、手改存储、导入坏备份）与「真的空」
// 必须分得开：从前的 `Array.isArray(list) ? … : []` 把前者一律回 []，界面于是显示
// 「暂无收藏」，与本机真的一条都没有完全同形 —— 而设置页的「清空」按钮正是在这个
// 读数之后直接 set({key: []})，把损坏数据当空覆盖掉，不可逆。
// 现在：键不存在（真的是空）→ []；键存在但不是数组 → 抛可读的原因，让调用方显示。
async function read(key, cap) {
  const { [key]: list } = await chrome.storage.local.get({ [key]: [] })
  if (list === undefined || list === null) return []
  if (!Array.isArray(list)) {
    throw new Error(`「${key}」存储内容不是列表（读出来是 ${typeof list}），可能是旧版数据或损坏：已停止操作，请先导出备份再清理该项`)
  }
  return list.slice(0, cap)
}

export async function getFavorites() {
  return read(FAV_KEY, 500)
}

export async function addFavorite(item) {
  if (!item || !item.url) return
  const list = await getFavorites()
  if (list.some((x) => x.url === item.url)) return // 去重
  list.unshift({ title: '', site: '', ...item, added_at: Date.now() })
  await chrome.storage.local.set({ [FAV_KEY]: list.slice(0, 500) })
}

export async function removeFavorite(url) {
  const list = (await getFavorites()).filter((x) => x.url !== url)
  await chrome.storage.local.set({ [FAV_KEY]: list })
}

export async function getHistory() {
  return read(HIS_KEY, 300)
}

export async function addHistory(entry) {
  if (!entry || !entry.url) return
  const list = await getHistory()
  list.unshift({ ...entry, added_at: Date.now() })
  await chrome.storage.local.set({ [HIS_KEY]: list.slice(0, 300) })
}
