// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// MissingSample — 未辅种样本（一个未辅种的种子：infohash + 名称）。
class MissingSample {
  final String hash;
  final String name;

  MissingSample({required this.hash, required this.name});

  factory MissingSample.fromJson(Map<String, dynamic> j) => MissingSample(
        hash: j['hash'] as String? ?? '',
        name: j['name'] as String? ?? '',
      );
}

// MissingSeed — 单站未辅种汇总。count=-1 表示该站未映射到 IYUU 目录无法判定。
class MissingSeed {
  final int siteId;
  final String siteName;
  final int count;
  final List<MissingSample> samples;

  MissingSeed({
    required this.siteId,
    required this.siteName,
    required this.count,
    required this.samples,
  });

  factory MissingSeed.fromJson(Map<String, dynamic> j) => MissingSeed(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        count: (j['count'] as num?)?.toInt() ?? 0,
        samples: ((j['samples'] as List?) ?? const [])
            .map((e) => MissingSample.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

// MissingSummary — POST /keepseed/missing 响应：
// 源做种总数 / 已提交校验数 / 已映射目标站数 / 各目标站缺失明细。
class MissingSummary {
  final int downloaderId;
  final int total;
  final int checked;
  final int targets;
  final List<MissingSeed> missing;

  MissingSummary({
    required this.downloaderId,
    required this.total,
    required this.checked,
    required this.targets,
    required this.missing,
  });

  factory MissingSummary.fromJson(Map<String, dynamic> j) => MissingSummary(
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        total: (j['total'] as num?)?.toInt() ?? 0,
        checked: (j['checked'] as num?)?.toInt() ?? 0,
        targets: (j['targets'] as num?)?.toInt() ?? 0,
        missing: ((j['missing'] as List?) ?? const [])
            .map((e) => MissingSeed.fromJson(e as Map<String, dynamic>))
            .toList(),
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

  // POST /keepseed/missing {downloader_id, target_site_ids} → 各目标站未辅种明细。
  Future<MissingSummary> missing(int downloaderId, List<int> targetSiteIds) async {
    final r = await _api.post('/keepseed/missing', {
      'downloader_id': downloaderId,
      'target_site_ids': targetSiteIds,
    });
    return MissingSummary.fromJson(r);
  }
}
