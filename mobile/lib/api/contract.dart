// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// 保种契约：为站点设置分享率 / 做种数 / 上传量 / 官种 / 体积下限，定时或手动检查是否达标。
/// 对齐后端 /contract/rules（按 site_id upsert）。
class ContractRule {
  final int id;
  final int siteId;
  final bool enabled;
  final double minRatio;
  final int minSeeding;
  final int minUploadGb;
  final bool officialOnly; // true=做种数/体积只统计官种
  final int minSeedingSizeGb; // 做种总体积下限(GiB)，0=不检
  final String period; // '' | weekly | monthly（告警去重窗口）
  final DateTime? startTime; // 生效起始，null=立即
  final DateTime? lastCheckedAt;
  final String lastStatus; // ok / breach / error / ''
  final String lastDetail;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  ContractRule({
    required this.id,
    required this.siteId,
    required this.enabled,
    required this.minRatio,
    required this.minSeeding,
    required this.minUploadGb,
    this.officialOnly = false,
    this.minSeedingSizeGb = 0,
    this.period = '',
    this.startTime,
    required this.lastCheckedAt,
    required this.lastStatus,
    required this.lastDetail,
    required this.createdAt,
    required this.updatedAt,
  });

  factory ContractRule.fromJson(Map<String, dynamic> j) => ContractRule(
        id: (j['id'] as num?)?.toInt() ?? 0,
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        enabled: j['enabled'] as bool? ?? false,
        minRatio: (j['min_ratio'] as num?)?.toDouble() ?? 0,
        minSeeding: (j['min_seeding'] as num?)?.toInt() ?? 0,
        minUploadGb: (j['min_upload_gb'] as num?)?.toInt() ?? 0,
        officialOnly: j['official_only'] as bool? ?? false,
        minSeedingSizeGb: (j['min_seeding_size_gb'] as num?)?.toInt() ?? 0,
        period: j['period'] as String? ?? '',
        startTime: DateTime.tryParse(j['start_time'] as String? ?? ''),
        lastCheckedAt: DateTime.tryParse(j['last_checked_at'] as String? ?? ''),
        lastStatus: j['last_status'] as String? ?? '',
        lastDetail: j['last_detail'] as String? ?? '',
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? ''),
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? ''),
      );
}

/// 一次契约检查的结果快照（POST /contract/rules/{id}/check）。
class ContractCheckResult {
  final int ruleId;
  final int siteId;
  final String siteName;
  final String status; // ok / breach / error
  final String detail;
  final double ratio;
  final int seeding;
  final double uploadGb;
  final int seedingOfficial; // 官种做种数（拉到明细时）
  final double seedingSizeGb; // 做种总体积(GiB)
  final String unsupported; // 不支持官种识别时的降级提示
  final List<String> breaches;

  ContractCheckResult({
    required this.ruleId,
    required this.siteId,
    required this.siteName,
    required this.status,
    required this.detail,
    required this.ratio,
    required this.seeding,
    required this.uploadGb,
    this.seedingOfficial = 0,
    this.seedingSizeGb = 0,
    this.unsupported = '',
    required this.breaches,
  });

  factory ContractCheckResult.fromJson(Map<String, dynamic> j) =>
      ContractCheckResult(
        ruleId: (j['rule_id'] as num?)?.toInt() ?? 0,
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        status: j['status'] as String? ?? '',
        detail: j['detail'] as String? ?? '',
        ratio: (j['ratio'] as num?)?.toDouble() ?? 0,
        seeding: (j['seeding'] as num?)?.toInt() ?? 0,
        uploadGb: (j['upload_gb'] as num?)?.toDouble() ?? 0,
        seedingOfficial: (j['seeding_official'] as num?)?.toInt() ?? 0,
        seedingSizeGb: (j['seeding_size_gb'] as num?)?.toDouble() ?? 0,
        unsupported: j['unsupported'] as String? ?? '',
        breaches: (j['breaches'] as List?)
                ?.map((e) => e as String)
                .toList() ??
            const [],
      );
}

class ContractApi {
  final _api = ApiClient();

  /// 全部契约规则（GET /contract/rules → {rules:[...]}）。
  Future<List<ContractRule>> list() async {
    final r = await _api.get('/contract/rules');
    final list = (r['rules'] as List?) ?? const [];
    return list
        .map((e) => ContractRule.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 新建 / 更新契约（POST /contract/rules，body=rule，按 site_id upsert）。
  Future<void> upsert(Map<String, dynamic> body) async {
    await _api.post('/contract/rules', body);
  }

  Future<void> remove(int id) => _api.delete('/contract/rules/$id');

  /// 触发一次检查，返回结果快照。
  Future<ContractCheckResult> check(int id) async {
    final r = await _api.post('/contract/rules/$id/check', <String, dynamic>{});
    return ContractCheckResult.fromJson(r);
  }
}
