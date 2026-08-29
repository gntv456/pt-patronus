// E2E mock：最小 qBittorrent WebUI API（内存态）。
// 用途：Playwright 核心链路测试在无真实下载器的环境下验证
// 「创建下载器 → 测连接 → 下发种子 → 任务出现」全链路。
// 端点（backend/internal/downloader/qb 实际调用到的最小集）：
//   POST /api/v2/auth/login      → 200 + SID cookie（固定 e2e-sid）
//   GET  /api/v2/app/version     → "v5.0-e2e-mock"
//   GET  /api/v2/app/webapiVersion → "2.11"
//   POST /api/v2/torrents/add    → "Ok."（记入内存种子表）
//   GET  /api/v2/torrents/info   → 种子 JSON 数组
//   GET  /api/v2/sync/maindata   → 同 info（部分代码走 sync 增量）
import http from 'node:http'

const PORT = Number(process.env.MOCK_QB_PORT || 8099)
const torrents = []
let addCount = 0
// rid 语义（对齐真实 qB）：每次状态变化单调 +1；客户端带相同 rid 轮询 → 无增量；
// 带旧 rid 或 0 → 全量。曾用 torrents.length 当 rid：添加种子后若长度恰好等于
// 上一轮返回的 rid，客户端相同 rid 的轮询会被误判「无增量」→ 新种子永不出现
// （全量套件里前置用例先消费过 rid=1 时必现，单跑用例碰不到）。
let rid = 0

function readBody(req) {
  return new Promise((resolve) => {
    let data = ''
    req.on('data', (c) => (data += c))
    req.on('end', () => resolve(data))
  })
}

import fs from 'node:fs'

const log = (m) => { try { fs.appendFileSync(process.env.MOCK_LOG || 'mock-qb.log', m + '\n') } catch { /* 日志失败不影响 mock */ } }
const server = http.createServer(async (req, res) => {
  const url = req.url || ''
  res.setHeader('Content-Type', 'text/plain; charset=utf-8')

  if (url.startsWith('/api/v2/auth/login')) {
    res.setHeader('Set-Cookie', 'SID=e2e-sid; path=/')
    res.end('Ok.')
    return
  }
  if (url.startsWith('/api/v2/app/version')) {
    res.end('v5.0-e2e-mock')
    return
  }
  if (url.startsWith('/api/v2/app/webapiVersion')) {
    res.end('2.11')
    return
  }
  if (url.startsWith('/api/v2/torrents/add')) {
    const body = await readBody(req)
    addCount++
    rid++ // 状态变化 → 新 rid（客户端旧 rid 轮询将拿到全量） // 状态变化 → 新 rid（客户端旧 rid 轮询将拿到全量）
    const magnet = body.match(/magnet:[^&\s]+/)
    torrents.push({
      hash: 'e2e' + String(addCount).padStart(28, '0'),
      name: 'E2E Mock Torrent #' + addCount + (magnet ? ' (magnet)' : ''),
      size: 1024 * 1024 * 100,
      progress: 0,
      state: 'downloading',
      dlspeed: 0,
      upspeed: 0,
      num_seeds: 1,
      num_leechs: 0,
      ratio: 0,
      added_on: Math.floor(Date.now() / 1000),
      category: '',
      tags: (body.match(/tags=([^&]*)/) || [, ''])[1].split('%2C').filter(Boolean),
      save_path: '/e2e',
    })
    res.end('Ok.')
    return
  }
  if (url.startsWith('/api/v2/torrents/info')) {
    res.setHeader('Content-Type', 'application/json')
    res.end(JSON.stringify(torrents))
    return
  }
  // sync/maindata：qB 增量格式 {rid, full_update?, torrents: {hash: {...}}}
  if (url.startsWith('/api/v2/sync/maindata')) {
    res.setHeader('Content-Type', 'application/json')
    const clientRid = Number(url.match(/rid=(\d+)/)?.[1] || 0)
    const byHash = {}
    for (const t of torrents) byHash[t.hash] = t
    if (clientRid > 0 && clientRid === rid) {
      res.end(JSON.stringify({ rid })) // 无增量
    } else {
      res.end(JSON.stringify({ rid, full_update: true, torrents: byHash, tags: ['e2e'] }))
    }
    return
  }
  res.statusCode = 404
  res.end('Not Found')
})

server.listen(PORT, '127.0.0.1', () => {
  console.log(`[mock-qb] listening on http://127.0.0.1:${PORT}`)
})
