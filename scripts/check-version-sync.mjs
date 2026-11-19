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

const goVer = (go.match(/var\s+Version\s*=\s*"([^"]+)"/) || [])[1];
const pubVer = (pubspec.match(/^\s*version:\s*([0-9]+\.[0-9]+\.[0-9]+)/m) || [])[1];
const versions = {
  backend: goVer,
  frontend: pkg.version,
  mobile: pubVer,
  updateManifest: manifest.latestVersion,
};
let bad = 0;
for (const [k, v] of Object.entries(versions)) {
  if (!v) { console.error(`[version-sync] ${k}: 未找到版本号`); bad++; }
}
const uniq = new Set(Object.values(versions).filter(Boolean));
if (uniq.size > 1) {
  console.error('[version-sync] 版本号不一致:', JSON.stringify(versions));
  bad++;
}
if (bad) process.exit(1);
console.log('[version-sync] OK', Object.values(versions)[0]);
