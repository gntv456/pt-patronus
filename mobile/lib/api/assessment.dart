// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';
import '../features/common/stream_account.dart';

// JSON 字符串字段容错：后端个别字段（如 end_time）可能返回 int（0 / epoch）而非字符串，
// `as String?` 会抛 "type 'int' is not a subtype of type 'String?' in type cast"（且 `?? ''`
// 救不了——cast 在前）。统一 toString 兜底，与审计「JSON 解析健壮性」同模式。
String _s(dynamic v) => v == null ? '' : v.toString();
String? _sOpt(dynamic v) => v?.toString();
double _n(dynamic v) => v is num ? v.toDouble() : (double.tryParse('$v') ?? 0);
int _i(dynamic v) => v is num ? v.toInt() : (int.tryParse('$v') ?? 0);

class AssessmentMetric {
  final String label;
  final double required;
  final double current;
  final String unit;
  final bool isCompleted;
  AssessmentMetric({
    required this.label,
    required this.required,
    required this.current,
    required this.unit,
    required this.isCompleted,
  });
  factory AssessmentMetric.fromJson(Map<String, dynamic> j) => AssessmentMetric(
        label: _s(j['label']),
        required: _n(j['required']),
        current: _n(j['current']),
        unit: _s(j['unit']),
        isCompleted: j['is_completed'] is bool ? j['is_completed'] as bool : false,
      );
}

class AssessmentCard {
  final int siteId;
  final String siteName;
  final String status;
  final double progressPercent;
  final int remainingDays;
  final List<AssessmentMetric> metrics;
  final String assessmentType;
  final String description;
  final String error;
  final String? endTime;
  AssessmentCard({
    required this.siteId,
    required this.siteName,
    required this.status,
    required this.progressPercent,
    required this.remainingDays,
    required this.metrics,
    required this.assessmentType,
    required this.description,
    required this.error,
    this.endTime,
  });
  factory AssessmentCard.fromJson(Map<String, dynamic> j) {
    final status = _s(j['status']);
    return AssessmentCard(
      siteId: _i(j['site_id']),
      siteName: _s(j['site_name']),
      status: status.isEmpty ? 'UNKNOWN' : status,
      progressPercent: _n(j['progress_percent']),
      remainingDays: _i(j['remaining_days']),
      metrics: ((j['metrics'] as List?) ?? const [])
          .map((e) => AssessmentMetric.fromJson(e as Map<String, dynamic>))
          .toList(),
      assessmentType: _s(j['assessment_type']),
      description: _s(j['description']),
      error: _s(j['error']),
      endTime: _sOpt(j['end_time']),
    );
  }
}

class AssessmentApi {
  AssessmentApi({StreamFrames? frames}) : _frames = frames;

  final StreamFrames? _frames;
  final _api = ApiClient();

  Stream<MapEntry<String, Map<String, dynamic>>> _stream(String path) =>
      _frames?.call(path) ?? _api.streamGet(path);
  Future<List<AssessmentCard>> all({bool refresh = false}) async {
    final r = await _api.get('/assessment${refresh ? '?refresh=1' : ''}');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => AssessmentCard.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 流式首屏（SSE）：每站完成推一帧 batch（单元素数组）；done 帧带清单账面。
  Stream<StreamBatch<AssessmentCard>> allStream({bool refresh = false}) {
    return _stream('/assessment/stream${refresh ? '?refresh=1' : ''}')
        .map((ev) => mapStreamFrame(ev, AssessmentCard.fromJson));
  }
}
