// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

library;

/// 更新产物 URL 归一：清单里的 targets.url 支持两种口径——
///   1. 绝对直链（https://pan.example.com/android-1.03.31.apk，网盘/CDN 外链）
///   2. 服务器相对路径（`/api/v1/system/downloads/android-1.03.31.apk`，
///      产物托管在自建服务器的 data/downloads 目录，由后端 guarded 下发）
///
/// 客户端统一在此绝对化：应用内 OTA 的 https 判据与外链对话框都吃绝对 URL。
/// 归一规则（纯函数，无 IO 依赖，可单测）：
///   * 空 base（未配置服务器）时相对 url 无从解析 → 原样返回（外链对话框至少能
///     展示，OTA 的 https 判据会自然拦下）；
///   * base 去尾斜杠后拼接，避免出现 `host//api/...` 双斜杠；
///   * 绝对 url（http/https 开头）原样透传。

/// 把产物 url 归一为可直接下载/判据的绝对形态。
String resolveArtifactUrl(String url, String base) {
  final u = url.trim();
  if (u.isEmpty) return '';
  if (u.startsWith('http://') || u.startsWith('https://')) return u;
  final b = base.trim().replaceAll(RegExp(r'/+$'), '');
  if (b.isEmpty) return u;
  return '$b${u.startsWith('/') ? u : '/$u'}';
}
