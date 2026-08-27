// 跨站收藏 + 下载历史（chrome.storage.local，仅本机；对标 PT-depiler my-collection / 下载历史）。
// 收藏：任意种子详情页经 background 消息写入 {url,title,site,added_at}，上限 500 条 FIFO。
// 历史：pushDownload 成功后追加 {title?,url,downloader_id,added_at}，上限 300 条。
const FAV_KEY = 'ptp_favorites'
const HIS_KEY = 'ptp_history'

async function read(key, cap) {
  const { [key]: list } = await chrome.storage.local.get({ [key]: [] })
  return Array.isArray(list) ? list.slice(0, cap) : []
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
