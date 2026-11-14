// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 主题调色盘：预设组 + 运行时 accent 状态与持久化。
// 与 Web useUiPrefs.ACCENT_GROUPS 同源（三端同一套色值）；
// PTColors.primary 等 getter 依据本文件的 accentNotifier 取值。
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'dart:async';
import 'dart:convert';
import 'package:ptpatronus/core/api_client.dart';

/// 主题调色盘预设 —— 与 Web useUiPrefs ACCENT_GROUPS 同源（三端同一套色值）。
/// 每预设带浅/深双态主色：浅色 600-700 阶（白字 ≥4.5:1），深色 300-400 阶（暗底可读）。
class PTAccentPreset {
  final String key;
  final String label;
  final Color light;
  final Color dark;
  const PTAccentPreset(this.key, this.label, this.light, this.dark);
}

class PTAccents {
  static const presets = <PTAccentPreset>[
    // 经典工作台
    PTAccentPreset('blue', '默认蓝', Color(0xFF2563EB), Color(0xFF4C8DFF)),
    PTAccentPreset('graphite', '石墨灰', Color(0xFF475569), Color(0xFF94A3B8)),
    PTAccentPreset('teal', '沉稳青', Color(0xFF0F766E), Color(0xFF2DD4BF)),
    // 水墨国风
    PTAccentPreset('gold', '典雅金', Color(0xFFA16207), Color(0xFFEAB308)),
    PTAccentPreset('lilac', '丁香紫', Color(0xFF7E5AA2), Color(0xFFC084FC)),
    PTAccentPreset('bean', '芸豆青', Color(0xFF4D7C6F), Color(0xFF4ADE80)),
    PTAccentPreset('sand', '辰砂色', Color(0xFFB4443C), Color(0xFFF87171)),
    // 鲜亮活力
    PTAccentPreset('sky', '地海蔚蓝', Color(0xFF0369A1), Color(0xFF38BDF8)),
    PTAccentPreset('sakura', '樱语红粉', Color(0xFFC2417A), Color(0xFFF472B6)),
    PTAccentPreset('spring', '触手生春', Color(0xFF15803D), Color(0xFF4ADE80)),
    PTAccentPreset('ember', '龙行绚烂', Color(0xFFC2410C), Color(0xFFFB923C)),
  ];

  static PTAccentPreset byKey(String key) =>
      presets.firstWhere((p) => p.key == key, orElse: () => presets.first);
}

/// 当前主题色预设 key（值变更 → accentNotifier 通知全 app 重建）。
final accentNotifier = ValueNotifier<String>('blue');


const _accentPrefsKey = 'accent';

/// 启动时恢复：先取本地缓存即时上色，再拉服务端偏好（登录态）覆盖——
/// Web 端换的主题色在手机上自动跟随（服务端为真源，本地为离线兜底）。
Future<void> loadAccentPrefs() async {
  final prefs = await SharedPreferences.getInstance();
  accentNotifier.value = prefs.getString(_accentPrefsKey) ?? 'blue';
  try {
    final r = await ApiClient().get('/auth/preferences/ui');
    final raw = r['value'] as String?;
    if (raw == null || raw.isEmpty) return;
    final ui = jsonDecode(raw);
    final accent = ui is Map ? ui['accent'] : null;
    if (accent is String && accent.isNotEmpty) {
      final key = accent;
      if (PTAccents.presets.any((p) => p.key == key) &&
          key != accentNotifier.value) {
        accentNotifier.value = key;
        await prefs.setString(_accentPrefsKey, key);
      }
    }
  } catch (_) {
    /* 未登录/网络不可用：沿用本地缓存 */
  }
}

/// 选定主题色：本地即时生效 + 写服务端偏好（Web/其他设备随之跟随）。
/// 服务端写入失败不阻断本地切换（下次启动会再试本地值的同步由 Web 域覆盖——
/// 这里只保证「至少本地生效」，跨设备最终一致）。
Future<void> setAccent(String key) async {
  accentNotifier.value = key;
  final prefs = await SharedPreferences.getInstance();
  await prefs.setString(_accentPrefsKey, key);
  unawaited(_pushAccentToServer(key));
}

Future<void> _pushAccentToServer(String key) async {
  try {
    // 读出 ui 域现状合并（不踩 Web 存的 uiStyle/sidebar 等字段），整域 PUT。
    Map<String, dynamic> ui = {};
    try {
      final r = await ApiClient().get('/auth/preferences/ui');
      final raw = r['value'] as String?;
      if (raw != null && raw.isNotEmpty) {
        final decoded = jsonDecode(raw);
        if (decoded is Map) ui = Map<String, dynamic>.from(decoded);
      }
    } catch (_) {}
    ui['accent'] = key;
    await ApiClient().put('/auth/preferences/ui', ui);
  } catch (_) {
    /* 离线：本地已生效，联网后由下次读取/覆盖收敛 */
  }
}
