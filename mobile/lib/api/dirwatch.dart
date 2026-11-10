// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// 单个被监视目录（dirwatch.WatchedDir）。发现媒体文件即喂给整理流水线。
class DirwatchDir {
  final String path; // 监视的源目录
  final String libraryPath; // 整理后入库的媒体库路径

  DirwatchDir({required this.path, required this.libraryPath});

  factory DirwatchDir.fromJson(Map<String, dynamic> j) => DirwatchDir(
        path: j['path'] as String? ?? '',
        libraryPath: j['library_path'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {'path': path, 'library_path': libraryPath};
}

/// 目录监视配置（对齐后端 /transfer/dirwatch）。
///
/// fsnotify 事件驱动 + debounce；compatibility 模式轮询兜底（覆盖 DoneHook +
/// cron 触及不到的手动投放 / 非托管来源）。
///
/// GET 与 PUT 同形（后端 `SetDirwatchConfig` 用 `dirwatch.Config` 直接绑定整个
/// 请求体整体替换），故 PUT 必须带上全部字段，否则被零值覆盖。
class DirwatchConfig {
  final bool enabled;
  final List<DirwatchDir> dirs;
  final String monitorMode; // auto(fsnotify) | compatibility(轮询)
  final int debounceMs; // 事件去抖毫秒（默认 2000）
  final int pollIntervalSec; // compatibility 模式轮询间隔秒

  DirwatchConfig({
    required this.enabled,
    required this.dirs,
    required this.monitorMode,
    required this.debounceMs,
    required this.pollIntervalSec,
  });

  factory DirwatchConfig.fromJson(Map<String, dynamic> j) => DirwatchConfig(
        enabled: j['enabled'] as bool? ?? false,
        dirs: ((j['dirs'] as List?) ?? const [])
            .map((e) => DirwatchDir.fromJson(e as Map<String, dynamic>))
            .toList(),
        monitorMode: j['monitor_mode'] as String? ?? 'auto',
        debounceMs: (j['debounce_ms'] as num?)?.toInt() ?? 2000,
        pollIntervalSec: (j['poll_interval_sec'] as num?)?.toInt() ?? 0,
      );

  /// 整体替换式 PUT body（必须包含全部字段）。
  Map<String, dynamic> toJson() => {
        'enabled': enabled,
        'dirs': dirs.map((d) => d.toJson()).toList(),
        'monitor_mode': monitorMode,
        'debounce_ms': debounceMs,
        'poll_interval_sec': pollIntervalSec,
      };
}

/// 目录监视 API（GET / PUT /transfer/dirwatch，管理员）。改动需重启进程生效。
class DirwatchApi {
  final _api = ApiClient();

  /// 读配置。
  Future<DirwatchConfig> get() async {
    final r = await _api.get('/transfer/dirwatch');
    return DirwatchConfig.fromJson(r);
  }

  /// 写配置（整体替换）。返回更新后的配置。
  Future<DirwatchConfig> update(DirwatchConfig cfg) async {
    final r = await _api.put('/transfer/dirwatch', cfg.toJson());
    return DirwatchConfig.fromJson(r);
  }
}
