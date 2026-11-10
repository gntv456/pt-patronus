// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// jessibuca 播放器资源的版本与字节数（前端侧副本）。
//
// 实际文件由**后端**按需下载到 DataDir/jessibuca 后直出（见 backend/internal/web/assets.go）——
// 不随仓库与镜像分发，因为 jessibuca 是 GPL-3.0。
//
// 这里仍保留一份清单，是为了让前端能在拉流前先探一次「资源是否已就位」：
// 未就位时立即显示可操作提示，而不是等后端同步下载完、或对着黑屏反复重连。
//
// ⚠️ 校验和与 `scripts/fetch-jessibuca.sh`、`backend/internal/web/assets.go` 三处一致，
// 由 `jessibucaAssets.test.ts` 交叉校验，改一处必须同步另两处。

export const JESSIBUCA_VERSION = 'v3.3.28'

interface Asset {
  file: string
  bytes: number
}

// 三个文件缺一不可：jessibuca.js（播放器）、decoder.js（wasm 胶水 + Worker）、
// decoder.wasm（ffmpeg 解码器，内含 H.264 与 H.265）。
export const JESSIBUCA_ASSETS: Asset[] = [
  { file: 'jessibuca.js', bytes: 219743 },
  { file: 'decoder.js', bytes: 110237 },
  { file: 'decoder.wasm', bytes: 1050314 },
]

export function assetUrl(file: string): string {
  return `/jessibuca/${file}`
}

/**
 * 探测资源是否已就位。
 *
 * 用 HEAD 而非 GET：wasm 约 1MB，三个文件合计 1.4MB，HEAD 只取头不取体。
 * 返回 false 不代表不可用——后端此刻可能正在首次下载，此时发起播放请求即可
 * （后端会等下载完成再返回），前端只是提前把「等待中」的提示显示出来。
 */
export async function assetsReady(): Promise<boolean> {
  try {
    const checks = await Promise.all(
      JESSIBUCA_ASSETS.map(async (a) => {
        const res = await fetch(assetUrl(a.file), { method: 'HEAD' })
        if (!res.ok) return false
        const len = Number(res.headers.get('content-length') ?? '0')
        return len === 0 || len === a.bytes
      }),
    )
    return checks.every(Boolean)
  } catch {
    return false
  }
}
