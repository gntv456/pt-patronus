// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// 后端日志键值属性（log.go 的 KV）。
class LogKV {
  final String key;
  final String value;
  LogKV({required this.key, required this.value});
  factory LogKV.fromJson(Map<String, dynamic> j) => LogKV(
        key: j['key'] as String? ?? '',
        value: '${j['value'] ?? ''}',
      );
}

/// 后端环形缓冲日志条目（internal/core/log.LogEntry）：time/level/message/attrs。
class LogEntry {
  final DateTime time;
  final String level; // DEBUG|INFO|WARN|ERROR
  final String message;
  final List<LogKV> attrs;
  LogEntry({
    required this.time,
    required this.level,
    required this.message,
    required this.attrs,
  });

  factory LogEntry.fromJson(Map<String, dynamic> j) => LogEntry(
        time: DateTime.tryParse(j['time'] as String? ?? '') ?? DateTime.now(),
        level: (j['level'] as String? ?? 'INFO').toUpperCase(),
        message: j['message'] as String? ?? '',
        attrs: ((j['attrs'] as List?) ?? const [])
            .map((e) => LogKV.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

/// 运行日志 API（对齐 Web Settings 日志 tab）：GET/DELETE /system/logs。
class SystemLogApi {
  final _api = ApiClient();

  /// 拉取环形缓冲日志。n=条数（默认 500），level=级别过滤（空=全部）。
  Future<List<LogEntry>> list({int n = 500, String level = ''}) async {
    final q = <String>['n=$n'];
    if (level.isNotEmpty) q.add('level=$level');
    final r = await _api.get('/system/logs?${q.join('&')}');
    final list = (r['entries'] as List?) ?? const [];
    return list.map((e) => LogEntry.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 清空环形缓冲。
  Future<void> clear() async => _api.delete('/system/logs');
}
