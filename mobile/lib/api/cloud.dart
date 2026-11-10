// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// 网盘实例（对齐后端 /cloud/drives）：115 / 夸克 / 123云盘。snake_case → camelCase 解析。
class CloudDrive {
  final int id;
  final String name;
  final String type; // 115 | quark | 123
  final String rootPath;
  final bool isDefault;
  final DateTime createdAt;
  final DateTime updatedAt;

  CloudDrive({
    required this.id,
    required this.name,
    required this.type,
    required this.rootPath,
    required this.isDefault,
    required this.createdAt,
    required this.updatedAt,
  });

  factory CloudDrive.fromJson(Map<String, dynamic> j) => CloudDrive(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? '115',
        rootPath: j['root_path'] as String? ?? '/',
        isDefault: j['is_default'] as bool? ?? false,
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );
}

/// 网盘条目（GET /cloud/drives/:id/list）。
class CloudFile {
  final String name;
  final String path;
  final bool isDir;
  final int size;

  CloudFile({
    required this.name,
    required this.path,
    required this.isDir,
    required this.size,
  });

  factory CloudFile.fromJson(Map<String, dynamic> j) => CloudFile(
        name: j['name'] as String? ?? '',
        path: j['path'] as String? ?? '',
        isDir: j['is_dir'] as bool? ?? false,
        size: (j['size'] as num?)?.toInt() ?? 0,
      );
}

/// STRM 生成规则。
class StrmRule {
  final int id;
  final String name;
  final int driveId;
  final String cloudPath;
  final String outputDir;
  final String mountRoot;
  final String template;
  final bool recursive;
  final bool keepExt;
  final bool enabled;

  StrmRule({
    required this.id,
    required this.name,
    required this.driveId,
    required this.cloudPath,
    required this.outputDir,
    required this.mountRoot,
    required this.template,
    required this.recursive,
    required this.keepExt,
    required this.enabled,
  });

  factory StrmRule.fromJson(Map<String, dynamic> j) => StrmRule(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        driveId: (j['drive_id'] as num?)?.toInt() ?? 0,
        cloudPath: j['cloud_path'] as String? ?? '/',
        outputDir: j['output_dir'] as String? ?? '',
        mountRoot: j['mount_root'] as String? ?? '',
        template: j['template'] as String? ?? '',
        recursive: j['recursive'] as bool? ?? true,
        keepExt: j['keep_ext'] as bool? ?? true,
        enabled: j['enabled'] as bool? ?? true,
      );
}

/// 已生成的 STRM 记录。
class StrmRecord {
  final int id;
  final int driveId;
  final String cloudPath;
  final String strmPath;
  final int size;
  final int subscriptionId;

  StrmRecord({
    required this.id,
    required this.driveId,
    required this.cloudPath,
    required this.strmPath,
    required this.size,
    required this.subscriptionId,
  });

  factory StrmRecord.fromJson(Map<String, dynamic> j) => StrmRecord(
        id: (j['id'] as num?)?.toInt() ?? 0,
        driveId: (j['drive_id'] as num?)?.toInt() ?? 0,
        cloudPath: j['cloud_path'] as String? ?? '',
        strmPath: j['strm_path'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        subscriptionId: (j['subscription_id'] as num?)?.toInt() ?? 0,
      );
}

const Map<String, String> cloudTypeLabel = {
  '115': '115网盘',
  'quark': '夸克网盘',
  '123': '123云盘',
};

/// 网盘 · STRM API（对齐后端 /cloud/*）。
class CloudApi {
  final _api = ApiClient();

  // ---- 网盘 ----
  Future<List<CloudDrive>> listDrives() async {
    final r = await _api.get('/cloud/drives');
    final list = (r['drives'] as List?) ?? const [];
    return list.map((e) => CloudDrive.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Map<String, dynamic>> createDrive(Map<String, dynamic> body) =>
      _api.post('/cloud/drives', body);

  Future<Map<String, dynamic>> updateDrive(int id, Map<String, dynamic> body) =>
      _api.put('/cloud/drives/$id', body);

  Future<void> removeDrive(int id) async => _api.delete('/cloud/drives/$id');

  Future<Map<String, dynamic>> test(int id) async =>
      _api.post('/cloud/drives/$id/test', <String, dynamic>{});

  Future<Map<String, dynamic>?> quota(int id) async {
    final r = await _api.get('/cloud/drives/$id/quota');
    return r['quota'] as Map<String, dynamic>?;
  }

  /// 浏览（GET /cloud/drives/:id/list?path=）。
  Future<List<CloudFile>> browse(int id, String path) async {
    final r = await _api.get(
        '/cloud/drives/$id/list?path=${Uri.encodeQueryComponent(path)}');
    final list = (r['files'] as List?) ?? const [];
    return list.map((e) => CloudFile.fromJson(e as Map<String, dynamic>)).toList();
  }

  // ---- STRM 规则 ----
  Future<List<StrmRule>> listRules() async {
    final r = await _api.get('/cloud/rules');
    final list = (r['rules'] as List?) ?? const [];
    return list.map((e) => StrmRule.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Map<String, dynamic>> createRule(Map<String, dynamic> body) =>
      _api.post('/cloud/rules', body);

  Future<Map<String, dynamic>> updateRule(int id, Map<String, dynamic> body) =>
      _api.put('/cloud/rules/$id', body);

  Future<void> removeRule(int id) async => _api.delete('/cloud/rules/$id');

  /// 生成单条规则（返回 {generated, skipped, errors}）。
  Future<Map<String, dynamic>> generateRule(int id) async =>
      _api.post('/cloud/rules/$id/generate', <String, dynamic>{});

  /// 即时生成 STRM（文件页选中目录直接生成，不持久化为规则）。
  Future<Map<String, dynamic>> generate(Map<String, dynamic> opts) =>
      _api.post('/cloud/generate', opts);

  Future<void> generateAll() async => _api.post('/cloud/generate-all', <String, dynamic>{});

  Future<void> syncSubscriptions() async =>
      _api.post('/cloud/sync-subscriptions', <String, dynamic>{});

  // ---- STRM 记录 ----
  Future<List<StrmRecord>> listRecords({int? driveId}) async {
    final path = driveId == null
        ? '/cloud/records'
        : '/cloud/records?drive_id=$driveId';
    final r = await _api.get(path);
    final list = (r['records'] as List?) ?? const [];
    return list.map((e) => StrmRecord.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> removeRecord(int id) async => _api.delete('/cloud/records/$id');
}
