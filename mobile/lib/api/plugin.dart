// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:ptpatronus/core/api_client.dart';

class PluginInfo {
  final String id;
  final String name;
  final String version;
  final String description;
  final bool enabled;
  final bool installed;
  final List<dynamic> permissions;

  const PluginInfo({
    required this.id,
    required this.name,
    required this.version,
    required this.description,
    required this.enabled,
    required this.installed,
    required this.permissions,
  });

  factory PluginInfo.fromJson(Map<String, dynamic> j) => PluginInfo(
        id: j['id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        version: j['version'] as String? ?? '',
        description: j['description'] as String? ?? '',
        enabled: j['enabled'] as bool? ?? false,
        installed: j['installed'] as bool? ?? false,
        permissions: (j['permissions'] as List?) ?? const [],
      );
}

/// 插件只读 + 启停；配置与市场安装仍以 Web 为准（移动端明确提示）。
class PluginApi {
  final ApiClient _api;

  PluginApi({ApiClient? api}) : _api = api ?? ApiClient();

  Future<List<PluginInfo>> list() async {
    final r = await _api.get('/plugins');
    final list = (r['plugins'] as List?) ?? const [];
    return list
        .map((e) => PluginInfo.fromJson(e as Map<String, dynamic>))
        .where((p) => p.installed)
        .toList();
  }

  Future<void> enable(PluginInfo p) =>
      _api.post('/plugins/${p.id}/enable', {'permissions': p.permissions});

  Future<void> disable(PluginInfo p) =>
      _api.post('/plugins/${p.id}/disable', {'permissions': p.permissions});
}
