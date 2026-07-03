import 'package:ptpatronus/core/api_client.dart';

// ReseedTask — 辅种候选任务（对齐后端 models.CrossSeedTask）。
class ReseedTask {
  final int id;
  final int downloaderId;
  final String infoHash;
  final String sourceName;
  final String candidateSite;
  final String candidateUrl;
  final String savePath;
  final String status; // pending | added | skipped | failed
  final String? resolvedAt;

  ReseedTask({
    required this.id,
    required this.downloaderId,
    required this.infoHash,
    required this.sourceName,
    required this.candidateSite,
    required this.candidateUrl,
    required this.savePath,
    required this.status,
    this.resolvedAt,
  });

  factory ReseedTask.fromJson(Map<String, dynamic> j) => ReseedTask(
        id: (j['id'] as num?)?.toInt() ?? 0,
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        infoHash: j['info_hash'] as String? ?? '',
        sourceName: j['source_name'] as String? ?? '',
        candidateSite: j['candidate_site'] as String? ?? '',
        candidateUrl: j['candidate_url'] as String? ?? '',
        savePath: j['save_path'] as String? ?? '',
        status: j['status'] as String? ?? 'pending',
        resolvedAt: j['resolved_at'] as String?,
      );
}

// ReseedSetting — 辅种设置（per-downloader，对齐后端 models.CrossSeedSetting）。
class ReseedSetting {
  final int downloaderId;
  final String savePath;
  final bool pauseAfterAdd;
  final bool autoReseed;
  final bool autoReseedAdd;

  ReseedSetting({
    required this.downloaderId,
    this.savePath = '',
    this.pauseAfterAdd = true,
    this.autoReseed = false,
    this.autoReseedAdd = false,
  });

  factory ReseedSetting.fromJson(Map<String, dynamic> j) => ReseedSetting(
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        savePath: j['save_path'] as String? ?? '',
        pauseAfterAdd: j['pause_after_add'] as bool? ?? true,
        autoReseed: j['auto_reseed'] as bool? ?? false,
        autoReseedAdd: j['auto_reseed_add'] as bool? ?? false,
      );

  Map<String, dynamic> toJson() => {
        'downloader_id': downloaderId,
        'save_path': savePath,
        'pause_after_add': pauseAfterAdd,
        'auto_reseed': autoReseed,
        'auto_reseed_add': autoReseedAdd,
      };

  ReseedSetting copyWith({bool? autoReseed, bool? autoReseedAdd, String? savePath, bool? pauseAfterAdd}) => ReseedSetting(
        downloaderId: downloaderId,
        savePath: savePath ?? this.savePath,
        pauseAfterAdd: pauseAfterAdd ?? this.pauseAfterAdd,
        autoReseed: autoReseed ?? this.autoReseed,
        autoReseedAdd: autoReseedAdd ?? this.autoReseedAdd,
      );
}

class ReseedApi {
  final _api = ApiClient();

  // GET /keepseed/config → {iyuu_token_masked, configured}
  Future<Map<String, dynamic>> getConfig() => _api.get('/keepseed/config');

  // PUT /keepseed/config {iyuu_token}
  Future<void> setToken(String token) =>
      _api.put('/keepseed/config', {'iyuu_token': token});

  // POST /keepseed/reseed {downloader_id} → {tasks, count}
  Future<List<ReseedTask>> reseed(int downloaderId) async {
    final r = await _api.post('/keepseed/reseed', {'downloader_id': downloaderId});
    final list = (r['tasks'] as List?) ?? const [];
    return list.map((e) => ReseedTask.fromJson(e as Map<String, dynamic>)).toList();
  }

  // GET /keepseed/tasks?status=
  Future<List<ReseedTask>> tasks(String status) async {
    final path = status.isEmpty ? '/keepseed/tasks' : '/keepseed/tasks?status=$status';
    final r = await _api.get(path);
    final list = (r['tasks'] as List?) ?? const [];
    return list.map((e) => ReseedTask.fromJson(e as Map<String, dynamic>)).toList();
  }

  // POST /keepseed/tasks/:id/resolve {action: add|skip}
  Future<void> resolve(int id, String action) =>
      _api.post('/keepseed/tasks/$id/resolve', {'action': action});

  // GET /keepseed/setting?downloader_id=
  Future<ReseedSetting> setting(int downloaderId) async {
    final r = await _api.get('/keepseed/setting?downloader_id=$downloaderId');
    return ReseedSetting.fromJson(r);
  }

  // PUT /keepseed/setting
  Future<void> saveSetting(ReseedSetting s) => _api.put('/keepseed/setting', s.toJson());
}
