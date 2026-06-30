import { http } from './http'

export interface Downloader {
  id: number
  name: string
  type: string // qb | tr
  url: string
  username: string
  is_default: boolean
}

export interface DownloaderInput {
  name: string
  type: string
  url: string
  username?: string
  password?: string
  is_default?: boolean
}

export interface Torrent {
  hash: string
  name: string
  size: number
  progress: number
  ratio: number
  state: string
  dl_rate: number
  up_rate: number
  save_path: string
  category: string
  tags: readonly string[]
  // —— 详情字段（随 WS 流实时推送，qB 全有，TR 子集）——
  downloaded?: number
  uploaded?: number
  num_seeds: number // 当前连接做种数
  num_leechs: number // 当前连接下载数
  num_complete: number // swarm 做种总数（qB）
  num_incomplete: number // swarm 下载总数（qB）
  availability: number // 可用性 0..1（qB）
  ratio_limit: number // -2=无限 -1=全局默认 >=0=值
  seeding_time_limit: number // 分钟，同上语义
  force_start: boolean
  auto_tmm: boolean
  sequential: boolean // 顺序下载（qB）
  first_last_piece: boolean // 首尾优先（qB）
  priority: number // 队列优先级（qB：0=最高）
  added_at: string // 创建时间 ISO
  completed_at: string // ISO；'0001-01-01...' 表示未完成
}

/** 限速（bytes/s，0=不限） */
export interface Limits {
  upload_limit: number
  download_limit: number
}

/** 做种（分享）限制。-2=无限 -1=全局默认 >=0=具体值 */
export interface ShareLimits {
  ratio_limit: number
  seeding_time_limit: number // 分钟（TR 为空闲时长）
}

export interface TorrentFile {
  index: number
  name: string
  size: number
  progress: number
  priority: number // qB: 0/1/2/7；TR: -1/0/1（保留后端原始取值）
}

export interface Tracker {
  url: string
  tier?: number
  status?: string
}

export interface Peer {
  ip: string
  client: string
  progress: number
  dl_rate: number
  up_rate: number
  flags?: string
}

export interface Stats {
  dl_rate: number
  up_rate: number
  total_size?: number
}

export interface DownloaderResult {
  ok: boolean
  error?: string
  level?: string
  code?: string // not_implemented 等
}

export interface AddOptions {
  category?: string
  tags?: string[]
  save_path?: string
  paused?: boolean
  upload_limit?: number // bytes/s
  download_limit?: number // bytes/s
}

export const downloaderApi = {
  list: () => http.get<{ downloaders: Downloader[] }>('/downloaders').then((r) => r.data.downloaders),
  create: (input: DownloaderInput) => http.post<Downloader>('/downloaders', input).then((r) => r.data),
  update: (id: number, input: DownloaderInput) => http.put(`/downloaders/${id}`, input).then((r) => r.data),
  remove: (id: number) => http.delete(`/downloaders/${id}`).then((r) => r.data),
  test: (id: number) => http.post<DownloaderResult>(`/downloaders/${id}/test`).then((r) => r.data),
  torrents: (id: number) =>
    http.get<{ torrents: Torrent[] }>(`/downloaders/${id}/torrents`).then((r) => r.data.torrents),
  add: (id: number, url: string, opts: AddOptions = {}) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents`, { url, ...opts }).then((r) => r.data),
  /** 上传 .torrent 文件（FormData；axios 自动设 multipart boundary，勿手动设 Content-Type） */
  upload: (id: number, form: FormData) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/upload`, form).then((r) => r.data),
  pause: (id: number, hashes: string[]) => http.post(`/downloaders/${id}/pause`, { hashes }).then((r) => r.data),
  resume: (id: number, hashes: string[]) => http.post(`/downloaders/${id}/resume`, { hashes }).then((r) => r.data),
  removeTorrents: (id: number, hashes: string[], deleteFiles = false) =>
    http.post(`/downloaders/${id}/remove`, { hashes, delete_files: deleteFiles }).then((r) => r.data),
  stats: (id: number) => http.get<Stats>(`/downloaders/${id}/stats`).then((r) => r.data),

  // —— 分类 / 标签 ——
  categories: (id: number) =>
    http.get<{ categories?: string[] } & DownloaderResult>(`/downloaders/${id}/categories`).then((r) => r.data),
  tags: (id: number) =>
    http.get<{ tags?: string[] } & DownloaderResult>(`/downloaders/${id}/tags`).then((r) => r.data),
  setCategory: (id: number, hashes: string[], category: string) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/category`, { hashes, category }).then((r) => r.data),
  editTags: (id: number, hashes: string[], tags: string[], mode: 'set' | 'add' | 'remove') =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/tags`, { hashes, tags, mode }).then((r) => r.data),
  createCategory: (id: number, name: string, savePath = '') =>
    http.post<DownloaderResult>(`/downloaders/${id}/categories/create`, { name, save_path: savePath }).then((r) => r.data),
  editCategory: (id: number, name: string, savePath: string) =>
    http.post<DownloaderResult>(`/downloaders/${id}/categories/edit`, { name, save_path: savePath }).then((r) => r.data),
  deleteCategories: (id: number, names: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/categories/delete`, { names }).then((r) => r.data),

  // —— 生命周期 ——
  recheck: (id: number, hashes: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/recheck`, { hashes }).then((r) => r.data),
  reannounce: (id: number, hashes: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/reannounce`, { hashes }).then((r) => r.data),
  setLocation: (id: number, hashes: string[], location: string) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/location`, { hashes, location }).then((r) => r.data),
  setForceStart: (id: number, hashes: string[], enable: boolean) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/force-start`, { hashes, enable }).then((r) => r.data),

  // —— 队列 ——
  setQueuePriority: (id: number, hashes: string[], action: 'top' | 'up' | 'down' | 'bottom') =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/queue`, { hashes, action }).then((r) => r.data),

  // —— 顺序下载 / 首尾优先 ——
  toggleSequential: (id: number, hashes: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/sequential`, { hashes }).then((r) => r.data),
  toggleFirstLast: (id: number, hashes: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/first-last`, { hashes }).then((r) => r.data),

  // —— 做种限制 ——
  setShareLimits: (id: number, hashes: string[], limits: ShareLimits) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/share-limits`, { hashes, ...limits }).then((r) => r.data),

  // —— 限速 ——
  setLimits: (id: number, hashes: string[], limits: Limits) =>
    http
      .post<DownloaderResult>(`/downloaders/${id}/torrents/limits`, { hashes, ...limits })
      .then((r) => r.data),
  getSpeedLimits: (id: number) => http.get<Limits>(`/downloaders/${id}/speed-limits`).then((r) => r.data),
  setSpeedLimits: (id: number, limits: Limits) =>
    http.post<DownloaderResult>(`/downloaders/${id}/speed-limits`, limits).then((r) => r.data),

  // —— 文件 ——
  files: (id: number, hash: string) =>
    http.get<{ files?: TorrentFile[] } & DownloaderResult>(`/downloaders/${id}/torrents/files`, {
      params: { hash },
    }).then((r) => r.data),
  setFilePriority: (id: number, hash: string, indices: number[], priority: number) =>
    http
      .post<DownloaderResult>(`/downloaders/${id}/torrents/file-priority`, { hash, indices, priority })
      .then((r) => r.data),

  // —— Peers ——
  peers: (id: number, hash: string) =>
    http
      .get<{ peers?: Peer[] } & DownloaderResult>(`/downloaders/${id}/torrents/peers`, { params: { hash } })
      .then((r) => r.data),

  // —— Tracker ——
  trackers: (id: number, hash: string) =>
    http
      .get<{ trackers?: Tracker[] } & DownloaderResult>(`/downloaders/${id}/torrents/trackers`, {
        params: { hash },
      })
      .then((r) => r.data),
  addTrackers: (id: number, hash: string, urls: string[]) =>
    http.post<DownloaderResult>(`/downloaders/${id}/torrents/trackers/add`, { hash, urls }).then((r) => r.data),
  removeTrackers: (id: number, hash: string, urls: string[]) =>
    http
      .post<DownloaderResult>(`/downloaders/${id}/torrents/trackers/remove`, { hash, urls })
      .then((r) => r.data),
}
