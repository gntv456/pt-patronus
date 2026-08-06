import { http } from './http'

// === Docker 容器管理（SSH 远程主机）===

export interface DockerContainer {
  ID: string
  Names: string
  Image: string
  Status: string
  Ports: string
  State: string
}

export interface DockerImage {
  Repository: string
  Tag: string
  ID: string
  CreatedAt: string
  Size: string
}

export interface DockerVolume {
  Driver: string
  Name: string
}

export interface DockerNetwork {
  ID: string
  Name: string
  Driver: string
  Scope: string
}

export interface DockerStatsRow {
  Name: string
  CPUPerc: string
  MemUsage: string
  MemPerc: string
  NetIO: string
  BlockIO: string
  PIDs: string
  Container: string
}

export interface DockerListResp {
  containers: DockerContainer[]
  images: DockerImage[]
  volumes: DockerVolume[]
  networks: DockerNetwork[]
}

export interface DockerInspect {
  Id: string
  Name: string
  State: Record<string, any>
  Config: Record<string, any>
  NetworkSettings: Record<string, any>
  Mounts: Record<string, any>[]
  Created: string
}

export const dockerApi = {
  list: (targetId: number) =>
    http.get<DockerListResp>(`/ops/docker/${targetId}/list`).then((r) => r.data),
  stats: (targetId: number) =>
    http.get<{ stats: DockerStatsRow[] }>(`/ops/docker/${targetId}/stats`).then((r) => r.data),
  inspect: (targetId: number, name: string) =>
    http.get<DockerInspect>(`/ops/docker/${targetId}/containers/${name}/inspect`).then((r) => r.data),
  logs: (targetId: number, name: string, tail = 100) =>
    http.get<{ logs: string }>(`/ops/docker/${targetId}/containers/${name}/logs`, { params: { tail } }).then((r) => r.data),
  start: (targetId: number, name: string) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/start`).then((r) => r.data),
  stop: (targetId: number, name: string) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/stop`).then((r) => r.data),
  restart: (targetId: number, name: string) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/restart`).then((r) => r.data),
  pause: (targetId: number, name: string) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/pause`).then((r) => r.data),
  unpause: (targetId: number, name: string) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/unpause`).then((r) => r.data),
  remove: (targetId: number, name: string, force = false, confirmRisk = false) =>
    http.post(`/ops/docker/${targetId}/containers/${name}/remove`, { confirm_risk: confirmRisk }, { params: { force: force ? 1 : 0 } }).then((r) => r.data),
  removeImage: (targetId: number, ref: string, confirmRisk = false) =>
    http.post(`/ops/docker/${targetId}/images/${ref}/remove`, { confirm_risk: confirmRisk }).then((r) => r.data),
  composeUp: (targetId: number, path: string) =>
    http.post<{ ok: boolean; output: string }>(`/ops/docker/${targetId}/compose/up`, { path }).then((r) => r.data),
  composeDown: (targetId: number, path: string, confirmRisk = false) =>
    http.post<{ ok: boolean; output: string }>(`/ops/docker/${targetId}/compose/down`, { path, confirm_risk: confirmRisk }).then((r) => r.data),

  // === 本机模式（Docker Engine API / docker.sock，无需 SSH）===
  localStatus: () =>
    http.get<{ available: boolean }>('/ops/docker/local/status').then((r) => r.data),
  localList: () =>
    http.get<DockerListResp>('/ops/docker/local/list').then((r) => r.data),
  localStats: () =>
    http.get<{ stats: DockerStatsRow[] }>('/ops/docker/local/stats').then((r) => r.data),
  localInspect: (name: string) =>
    http.get<DockerInspect>(`/ops/docker/local/containers/${name}/inspect`).then((r) => r.data),
  localLogs: (name: string, tail = 100) =>
    http.get<{ logs: string }>(`/ops/docker/local/containers/${name}/logs`, { params: { tail } }).then((r) => r.data),
  localAction: (name: string, action: string) =>
    http.post(`/ops/docker/local/containers/${name}/${action}`).then((r) => r.data),
  localRemove: (name: string, force = false, confirmRisk = false) =>
    http.post(`/ops/docker/local/containers/${name}/remove`, { confirm_risk: confirmRisk }, { params: { force: force ? 1 : 0 } }).then((r) => r.data),
  localRemoveImage: (ref: string, confirmRisk = false) =>
    http.delete(`/ops/docker/local/images/${ref}`, { data: { confirm_risk: confirmRisk } }).then((r) => r.data),

  // === 本机扩展：拉取镜像 / 清理 / 卷网络管理 ===
  localPull: (ref: string) =>
    http.post<{ ok: boolean; progress?: string; error?: string }>('/ops/docker/local/images/pull', null, { params: { ref } }).then((r) => r.data),
  localPrune: (type: 'container' | 'image' | 'volume' | 'network') =>
    http.post<{ ok: boolean; result?: Record<string, any> }>('/ops/docker/local/prune', null, { params: { type } }).then((r) => r.data),
  localVolumeCreate: (name: string, driver?: string) =>
    http.post('/ops/docker/local/volumes', { name, driver }).then((r) => r.data),
  localVolumeRemove: (name: string) =>
    http.delete(`/ops/docker/local/volumes/${name}`).then((r) => r.data),
  localNetworkCreate: (name: string, driver?: string) =>
    http.post('/ops/docker/local/networks', { name, driver }).then((r) => r.data),
  localNetworkRemove: (name: string) =>
    http.delete(`/ops/docker/local/networks/${name}`).then((r) => r.data),

  // === 创建容器 + exec 终端 ===
  localCreate: (data: {
    image: string
    name?: string
    cmd?: string[]
    env?: Record<string, string>
    ports?: { container: number; host: number }[]
    binds?: string[]
    restart_policy?: string
    auto_start?: boolean
  }) =>
    http.post<{ ok: boolean; id?: string; error?: string }>('/ops/docker/local/containers/create', data).then((r) => r.data),
  // exec 终端的 WebSocket URL（前端自行 new WebSocket）
  localExecURL: (name: string, cmd = '/bin/sh') => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/ops/docker/local/containers/${name}/exec/ws?cmd=${encodeURIComponent(cmd)}&cols=120&rows=32`
  },
}
