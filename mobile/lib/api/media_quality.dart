// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// 多评分订阅质量配置（对齐后端 /media/quality）。
class MediaQualityConfig {
  final bool enabled;
  final double minScore;
  final int minVotes;
  final int minReleaseDays;
  final List<String> sources;

  MediaQualityConfig({
    required this.enabled,
    required this.minScore,
    required this.minVotes,
    required this.minReleaseDays,
    required this.sources,
  });

  factory MediaQualityConfig.fromJson(Map<String, dynamic> j) {
    final srcs = (j['sources'] as List?) ?? const [];
    return MediaQualityConfig(
      enabled: j['enabled'] as bool? ?? false,
      minScore: (j['min_score'] as num?)?.toDouble() ?? 6.0,
      minVotes: (j['min_votes'] as num?)?.toInt() ?? 100,
      minReleaseDays: (j['min_release_days'] as num?)?.toInt() ?? 7,
      sources: srcs.map((e) => e.toString()).toList(),
    );
  }

  Map<String, dynamic> toJson() => {
        'enabled': enabled,
        'min_score': minScore,
        'min_votes': minVotes,
        'min_release_days': minReleaseDays,
        'sources': sources,
      };
}

/// 单源评分明细。
class SourceRating {
  final String source;
  final double score;
  final int votes;
  SourceRating({required this.source, required this.score, required this.votes});
  factory SourceRating.fromJson(Map<String, dynamic> j) => SourceRating(
        source: j['source'] as String? ?? '',
        score: (j['score'] as num?)?.toDouble() ?? 0,
        votes: (j['votes'] as num?)?.toInt() ?? 0,
      );
}

/// 多源聚合评分 + 闸门判定（对齐后端 /media/rating）。
class RatingSummary {
  final String title;
  final int year;
  final double aggregated;
  final int votes;
  final int releaseDays;
  final List<SourceRating> sources;
  final bool passed;
  final String reason;

  RatingSummary({
    required this.title,
    required this.year,
    required this.aggregated,
    required this.votes,
    required this.releaseDays,
    required this.sources,
    required this.passed,
    required this.reason,
  });

  factory RatingSummary.fromJson(Map<String, dynamic> j) {
    final srcs = (j['sources'] as List?) ?? const [];
    return RatingSummary(
      title: j['title'] as String? ?? '',
      year: (j['year'] as num?)?.toInt() ?? 0,
      aggregated: (j['aggregated'] as num?)?.toDouble() ?? 0,
      votes: (j['votes'] as num?)?.toInt() ?? 0,
      releaseDays: (j['release_days'] as num?)?.toInt() ?? 0,
      sources: srcs.map((e) => SourceRating.fromJson(e as Map<String, dynamic>)).toList(),
      passed: j['passed'] as bool? ?? false,
      reason: j['reason'] as String? ?? '',
    );
  }
}

const List<({String value, String label})> kRatingSources = [
  (value: 'tmdb', label: 'TMDB'),
  (value: 'douban', label: '豆瓣'),
  (value: 'imdb', label: 'IMDb(OMDb)'),
  (value: 'bangumi', label: 'Bangumi'),
];

String sourceLabel(String s) {
  for (final o in kRatingSources) {
    if (o.value == s) return o.label;
  }
  return s;
}

/// 媒体质量 API（对齐后端 /media/quality + /media/rating）。
class MediaQualityApi {
  final _api = ApiClient();

  Future<MediaQualityConfig> getConfig() async {
    final r = await _api.get('/media/quality');
    return MediaQualityConfig.fromJson(r);
  }

  Future<void> setConfig(MediaQualityConfig cfg) async =>
      _api.put('/media/quality', cfg.toJson());

  Future<RatingSummary> rating(String title, {int year = 0, String type = ''}) async {
    final q = 'title=${Uri.encodeQueryComponent(title)}&year=$year&type=$type';
    final r = await _api.get('/media/rating?$q');
    return RatingSummary.fromJson(r);
  }
}
