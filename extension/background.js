// background service worker：右键菜单「搜索选中词」→ 打开 popup 语境的 Web 搜索页；
// 快捷键 Alt+Shift+S 同路径。通知用于订阅/搜索失败提示。

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: 'ptp-search-selection',
    title: '在 PTPatronus 搜索「%s」',
    contexts: ['selection'],
  })
})

chrome.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== 'ptp-search-selection' || !info.selectionText) return
  const { server = '' } = await chrome.storage.sync.get({ server: '' })
  if (!server) {
    chrome.notifications.create({
      type: 'basic',
      iconUrl: chrome.runtime.getURL('icons/icon128.png'),
      title: 'PTPatronus 助手',
      message: '请先在扩展选项页配置服务器地址与 API Key',
    })
    chrome.runtime.openOptionsPage()
    return
  }
  const base = server.replace(/\/$/, '')
  chrome.tabs.create({ url: `${base}/search?keyword=${encodeURIComponent(info.selectionText.slice(0, 200))}` })
})

// 键盘快捷键：与右键菜单同路径（读取当前 tab 选中词）。
chrome.commands.onCommand.addListener(async (command) => {
  if (command !== 'search-selection') return
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (!tab) return
  try {
    const results = await chrome.scripting.executeScript({
      target: { tabId: tab.id },
      func: () => window.getSelection().toString(),
    })
    const sel = ((results && results[0] && results[0].result) || '').trim()
    const { server = '' } = await chrome.storage.sync.get({ server: '' })
    if (sel && server) {
      chrome.tabs.create({ url: `${server.replace(/\/$/, '')}/search?keyword=${encodeURIComponent(sel.slice(0, 200))}` })
    }
  } catch { /* 无权限页面忽略 */ }
})
