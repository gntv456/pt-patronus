// 选项页：服务器地址 + API Key 保存（chrome.storage.sync 跨设备同步）；测试连接打 /api/v1/system/info。
import { getConfig, saveConfig, api } from './api.js'

const $ = (id) => document.getElementById(id)

document.addEventListener('DOMContentLoaded', async () => {
  const cfg = await getConfig()
  $('server').value = cfg.server || ''
  $('apiKey').value = cfg.apiKey || ''

  $('save').onclick = async () => {
    await saveConfig({ server: $('server').value.trim(), apiKey: $('apiKey').value.trim() })
    $('msg').textContent = '已保存'
    $('msg').className = 'ok'
  }

  $('test').onclick = async () => {
    $('msg').textContent = '测试中…'
    $('msg').className = ''
    try {
      await saveConfig({ server: $('server').value.trim(), apiKey: $('apiKey').value.trim() })
      const cfg2 = await getConfig()
      // 任意轻量鉴权端点验证 key：sites 列表（admin 可见）。
      await api(cfg2, '/api/v1/sites')
      $('msg').textContent = '连接成功 ✅'
      $('msg').className = 'ok'
    } catch (e) {
      $('msg').textContent = `失败：${e.message}`
    }
  }
})
