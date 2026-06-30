import '../core/api_client.dart';

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
        label: j['label'] as String? ?? '',
        required: (j['required'] as num?)?.toDouble() ?? 0,
        current: (j['current'] as num?)?.toDouble() ?? 0,
        unit: j['unit'] as String? ?? '',
        isCompleted: j['is_completed'] as bool? ?? false,
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
  });
  factory AssessmentCard.fromJson(Map<String, dynamic> j) => AssessmentCard(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        status: j['status'] as String? ?? 'UNKNOWN',
        progressPercent: (j['progress_percent'] as num?)?.toDouble() ?? 0,
        remainingDays: (j['remaining_days'] as num?)?.toInt() ?? -1,
        metrics: ((j['metrics'] as List?) ?? const [])
            .map((e) => AssessmentMetric.fromJson(e as Map<String, dynamic>))
            .toList(),
        assessmentType: j['assessment_type'] as String? ?? '',
        description: j['description'] as String? ?? '',
        error: j['error'] as String? ?? '',
      );
}

class AssessmentApi {
  final _api = ApiClient();
  Future<List<AssessmentCard>> all({bool refresh = false}) async {
    final r = await _api.get('/assessment${refresh ? '?refresh=1' : ''}');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => AssessmentCard.fromJson(e as Map<String, dynamic>)).toList();
  }
}
