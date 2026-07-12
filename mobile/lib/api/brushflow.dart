import '../core/api_client.dart';

/// 刷流任务（对齐后端 /brushflow）：选种规则 + 删种规则 + 统计。
///
/// snake_case JSON → camelCase 字段，解析范式同 [Medal.fromJson]：
/// 数字走 `(j[k] as num?)?.toInt() ?? 0`，字符串兜底空串，日期 `DateTime.tryParse`。
class BrushFlowTask {
  final int id;
  final String name;
  final int siteId;
  final int downloaderId;
  final bool enabled;
  final String keyword;
  final String category;
  final int minSizeMb;
  final int maxSizeMb;
  final int minSeeders;
  final int maxSeeders;
  final bool freeOnly;
  final bool paused;
  final double deleteRatio;
  final double deleteSeedHours;
  final int deleteUploadMb;
  final bool deleteFiles;
  final int maxSeeds;
  final String savePath;
  final String categoryDl;
  final int totalAdded;
  final int totalDeleted;
  final int totalUpload;
  final DateTime? lastSelectAt;
  final DateTime? lastSweepAt;
  final String lastError;
  final DateTime createdAt;
  final DateTime updatedAt;

  BrushFlowTask({
    required this.id,
    required this.name,
    required this.siteId,
    required this.downloaderId,
    required this.enabled,
    required this.keyword,
    required this.category,
    required this.minSizeMb,
    required this.maxSizeMb,
    required this.minSeeders,
    required this.maxSeeders,
    required this.freeOnly,
    required this.paused,
    required this.deleteRatio,
    required this.deleteSeedHours,
    required this.deleteUploadMb,
    required this.deleteFiles,
    required this.maxSeeds,
    required this.savePath,
    required this.categoryDl,
    required this.totalAdded,
    required this.totalDeleted,
    required this.totalUpload,
    required this.lastSelectAt,
    required this.lastSweepAt,
    required this.lastError,
    required this.createdAt,
    required this.updatedAt,
  });

  factory BrushFlowTask.fromJson(Map<String, dynamic> j) => BrushFlowTask(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        enabled: j['enabled'] as bool? ?? false,
        keyword: j['keyword'] as String? ?? '',
        category: j['category'] as String? ?? '',
        minSizeMb: (j['min_size_mb'] as num?)?.toInt() ?? 0,
        maxSizeMb: (j['max_size_mb'] as num?)?.toInt() ?? 0,
        minSeeders: (j['min_seeders'] as num?)?.toInt() ?? 0,
        maxSeeders: (j['max_seeders'] as num?)?.toInt() ?? 0,
        freeOnly: j['free_only'] as bool? ?? false,
        paused: j['paused'] as bool? ?? false,
        deleteRatio: (j['delete_ratio'] as num?)?.toDouble() ?? 0,
        deleteSeedHours: (j['delete_seed_hours'] as num?)?.toDouble() ?? 0,
        deleteUploadMb: (j['delete_upload_mb'] as num?)?.toInt() ?? 0,
        deleteFiles: j['delete_files'] as bool? ?? false,
        maxSeeds: (j['max_seeds'] as num?)?.toInt() ?? 0,
        savePath: j['save_path'] as String? ?? '',
        categoryDl: j['category_dl'] as String? ?? '',
        totalAdded: (j['total_added'] as num?)?.toInt() ?? 0,
        totalDeleted: (j['total_deleted'] as num?)?.toInt() ?? 0,
        totalUpload: (j['total_upload'] as num?)?.toInt() ?? 0,
        lastSelectAt: DateTime.tryParse(j['last_select_at'] as String? ?? ''),
        lastSweepAt: DateTime.tryParse(j['last_sweep_at'] as String? ?? ''),
        lastError: j['last_error'] as String? ?? '',
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );

  /// 乐观更新辅助（切换 enabled 等即时反馈场景）。
  BrushFlowTask copyWith({bool? enabled}) => BrushFlowTask(
        id: id,
        name: name,
        siteId: siteId,
        downloaderId: downloaderId,
        enabled: enabled ?? this.enabled,
        keyword: keyword,
        category: category,
        minSizeMb: minSizeMb,
        maxSizeMb: maxSizeMb,
        minSeeders: minSeeders,
        maxSeeders: maxSeeders,
        freeOnly: freeOnly,
        paused: paused,
        deleteRatio: deleteRatio,
        deleteSeedHours: deleteSeedHours,
        deleteUploadMb: deleteUploadMb,
        deleteFiles: deleteFiles,
        maxSeeds: maxSeeds,
        savePath: savePath,
        categoryDl: categoryDl,
        totalAdded: totalAdded,
        totalDeleted: totalDeleted,
        totalUpload: totalUpload,
        lastSelectAt: lastSelectAt,
        lastSweepAt: lastSweepAt,
        lastError: lastError,
        createdAt: createdAt,
        updatedAt: updatedAt,
      );
}

/// 刷流历史种子（GET /brushflow/:id/seeds）。
class BrushFlowSeed {
  final int id;
  final int taskId;
  final int downloaderId;
  final String sourceKey;
  final String title;
  final int size;
  final int siteId;
  final DateTime? addedAt;
  final DateTime createdAt;
  final DateTime updatedAt;

  BrushFlowSeed({
    required this.id,
    required this.taskId,
    required this.downloaderId,
    required this.sourceKey,
    required this.title,
    required this.size,
    required this.siteId,
    required this.addedAt,
    required this.createdAt,
    required this.updatedAt,
  });

  factory BrushFlowSeed.fromJson(Map<String, dynamic> j) => BrushFlowSeed(
        id: (j['id'] as num?)?.toInt() ?? 0,
        taskId: (j['task_id'] as num?)?.toInt() ?? 0,
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        sourceKey: j['source_key'] as String? ?? '',
        title: j['title'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        addedAt: DateTime.tryParse(j['added_at'] as String? ?? ''),
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );
}

/// 刷流引擎 API（对齐后端 /brushflow/*）。
class BrushFlowApi {
  final _api = ApiClient();

  /// 任务列表（GET /brushflow → {tasks:[...]}）。
  Future<List<BrushFlowTask>> list() async {
    final r = await _api.get('/brushflow');
    final list = (r['tasks'] as List?) ?? const [];
    return list
        .map((e) => BrushFlowTask.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 新建任务（POST /brushflow）。返回后端响应（含新建任务或 error）。
  Future<Map<String, dynamic>> create(Map<String, dynamic> body) =>
      _api.post('/brushflow', body);

  /// 更新任务（PUT /brushflow/:id）。
  Future<Map<String, dynamic>> update(int id, Map<String, dynamic> body) =>
      _api.put('/brushflow/$id', body);

  /// 删除任务（DELETE /brushflow/:id）。
  Future<void> remove(int id) async => _api.delete('/brushflow/$id');

  /// 立即选种（POST /brushflow/:id/select）。返回 {added, message, ...}。
  Future<Map<String, dynamic>> select(int id) async =>
      _api.post('/brushflow/$id/select', <String, dynamic>{});

  /// 立即扫删（POST /brushflow/:id/sweep）。返回 {deleted, message, ...}。
  Future<Map<String, dynamic>> sweep(int id) async =>
      _api.post('/brushflow/$id/sweep', <String, dynamic>{});

  /// 任务历史种子（GET /brushflow/:id/seeds → {seeds:[...]}）。
  Future<List<BrushFlowSeed>> seeds(int id) async {
    final r = await _api.get('/brushflow/$id/seeds');
    final list = (r['seeds'] as List?) ?? const [];
    return list
        .map((e) => BrushFlowSeed.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}
