#!/usr/bin/env node
// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors
// 校验四处版本号一致：后端单一真源 / 前端 / 移动端 / 更新清单。
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
const __dirname = path.dirname(fileURLToPath(import.meta.url));

const root = path.resolve(__dirname, '..');
function read(rel) { return fs.readFileSync(path.join(root, rel), 'utf8'); }

const go = read('backend/internal/version/version.go');
const pkg = JSON.parse(read('frontend/package.json'));
const pubspec = read('mobile/pubspec.yaml');
const manifest = JSON.parse(read('update-manifest.json'));
const extManifest = JSON.parse(read('extension/manifest.json'));

const goVer = (go.match(/var\s+Version\s*=\s*"([^"]+)"/) || [])[1];
const pubVer = (pubspec.match(/^\s*version:\s*([0-9]+\.[0-9]+\.[0-9]+)/m) || [])[1];
const versions = {
  backend: goVer,
  frontend: pkg.version,
  mobile: pubVer,
  updateManifest: manifest.latestVersion,
  // 浏览器扩展 manifest 版本：扩展自更新检查（extension_update.js）拿它跟
  // 服务器 latestVersion 对比——漂移会导致「有新版却永远不提示」（1.03.21
  // bundle 过期事故的同型风险；扩展代码无需随版本发版时允许落后，见下方豁免）。
  extension: extManifest.version,
};
let bad = 0;
for (const [k, v] of Object.entries(versions)) {
  if (!v) { console.error(`[version-sync] ${k}: 未找到版本号`); bad++; }
}
const uniq = new Set(Object.values(versions).filter(Boolean));
if (uniq.size > 1) {
  // extension 豁免：扩展源码未改动时允许落后主版本（manifest 版本只在扩展
  // 内容变化时递增）；其余四处必须严格一致。
  const ext = versions.extension;
  const rest = Object.entries(versions).filter(([k]) => k !== 'extension').map(([, v]) => v);
  const restUniq = new Set(rest.filter(Boolean));
  if (restUniq.size > 1) {
    console.error('[version-sync] 核心四处版本号不一致:', JSON.stringify(versions));
    bad++;
  } else if (restUniq.size === 1 && ext && ext !== [...restUniq][0]) {
    console.error(`[version-sync] 扩展 manifest 版本 (${ext}) 落后主版本 (${[...restUniq][0]})——若扩展内容有变请 bump extension/manifest.json 并重跑 pack-extension.py`);
    // 提示但不阻断：扩展内容没变时不强制追平。
    console.error('[version-sync] （提示级：扩展内容未变可不处理）');
  }
}
if (bad) process.exit(1);
console.log('[version-sync] OK', Object.values(versions)[0]);
