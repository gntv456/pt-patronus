// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import { http } from './http'

// === 磁盘管理（SSH 远程主机）===

export interface DiskPartition {
  name: string
  size: number
  size_human: string
  fstype?: string
  mountpoint?: string
  partlabel?: string
  used_bytes?: number
  used_human?: string
  usage_percent?: number
}

export interface DiskEntry {
  name: string // /dev/sda
  type: string // disk | rom
  size: number
  size_human: string
  model?: string
  vendor?: string
  serial?: string
  rota?: string // 1=HDD 0=SSD
  category: string // system | unpartitioned | normal
  is_system: boolean
  used_bytes?: number
  used_human?: string
  total_bytes?: number
  total_human?: string
  usage_percent?: number
  partitions?: DiskPartition[]
}

export interface DiskListResp {
  disks: DiskEntry[]
}

export interface PartitionRequest {
  device: string
  fstype: 'ext4' | 'xfs'
  mountpoint: string
  auto_mount?: boolean
  nofail?: boolean
  confirm_risk?: boolean
}

export interface MountRequest {
  device: string
  mountpoint: string
  auto_mount?: boolean
  nofail?: boolean
}

export interface UmountRequest {
  mountpoint: string
  remove_fstab?: boolean
}

export const diskApi = {
  list: (targetId: number) => http.get<DiskListResp>(`/ops/disk/${targetId}/list`).then((r) => r.data),
  partition: (targetId: number, data: PartitionRequest) =>
    http
      .post<{ ok: boolean; error?: string; partition?: string; fstab?: string }>(
        `/ops/disk/${targetId}/partition`,
        data,
      )
      .then((r) => r.data),
  mount: (targetId: number, data: MountRequest) =>
    http.post<{ ok: boolean; error?: string; fstab?: string }>(`/ops/disk/${targetId}/mount`, data).then((r) => r.data),
  umount: (targetId: number, data: UmountRequest) =>
    http.post<{ ok: boolean; error?: string }>(`/ops/disk/${targetId}/umount`, data).then((r) => r.data),
}
