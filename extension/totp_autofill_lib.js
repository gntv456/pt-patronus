// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// TOTP 自动填充后台库：service worker 不能用 ES module import（manifest 声明为 classic），
// 此文件通过 dynamic import(chrome.runtime.getURL(...)) 在 SW 内加载，复用 totp.js 的生成逻辑。
// 只导出 totpNow——自动填充永不触碰 vault 加解密（锁定语义在 background 里判定）。
export { totpNow, parseOTPAuth } from './totp.js'
