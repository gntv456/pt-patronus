// 下载任务面板数据层（对标 PT-depiler「下载器任务管理」）：
// 多下载器切换、任务列表（hash/name/progress/state/速度）、暂停/继续/删除、多来源添加。
// 后端已有完整代理端点：GET /downloaders、GET /downloaders/:id/torrents、
// POST /downloaders/:id/{pause,resume,remove}、POST /downloaders/:id/torrents。
// 请求统一走 api.js 的 api()（鉴权 + 非 2xx 抛错），URL 归一化走 util.js 的 baseUrl。
import { api } from './api.js'

export async function listDownloaders(cfg) {
  const body = await api(cfg, '/api/v1/downloaders')
  return body.downloaders || []
}

export async function listTorrents(cfg, dlId) {
  const body = await api(cfg, `/api/v1/downloaders/${dlId}/torrents?limit=500`).catch((e) => ({ error: e.message }))
  return { torrents: body.torrents || [], total: body.total || 0, error: body.error || '' }
}

// globalStats 下载器全局统计（含 free_space 剩余空间；协议不暴露时为 0）。
export async function globalStats(cfg, dlId) {
  return api(cfg, `/api/v1/downloaders/${dlId}/stats`)
}

async function op(cfg, dlId, action, payload) {
  return api(cfg, `/api/v1/downloaders/${dlId}/${action}`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
}

// pause/resume/remove 按后端契约：hash 数组（单任务包装成 [hash]）。
export const pauseTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'pause', { hashes: [hash] })
export const resumeTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'resume', { hashes: [hash] })
export const removeTorrent = (cfg, dlId, hash) => op(cfg, dlId, 'remove', { hashes: [hash] })

// addByUrl 添加下载（种子链接/磁力；服务端按 URL 注册域反查站点凭据取 .torrent）。
export function addByUrl(cfg, dlId, url) {
  return op(cfg, dlId, 'torrents', { url, tags: ['ptpatronus-extension'] })
}

// findReseed 跨站找同种（辅种最小版）：POST /keepseed/find {name,size}。
// 服务端跨站搜索 + 评分（名称/大小/TMDB/IMDb 指纹），score≥60 的候选即「同种」。
export async function findReseed(cfg, name, size = 0) {
  const body = await api(cfg, '/api/v1/keepseed/find', {
    method: 'POST',
    body: JSON.stringify({ name: String(name).slice(0, 512), size }),
  })
  return body.candidates || []
}
