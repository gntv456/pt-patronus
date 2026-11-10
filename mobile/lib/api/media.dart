// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

// 演职人员（Detail 富字段）：名字 + 饰演角色 + 头像。
class CreditPerson {
  final String name;
  final String role;
  final String avatar;
  const CreditPerson({required this.name, this.role = '', this.avatar = ''});
  factory CreditPerson.fromJson(Map<String, dynamic> j) => CreditPerson(
        name: j['name'] as String? ?? '',
        role: j['role'] as String? ?? '',
        avatar: j['avatar'] as String? ?? '',
      );
}

// MediaInfo 是一条媒体元数据匹配（对齐后端 media.MediaInfo）。
// 跨源（TMDB/豆瓣/MusicBrainz）搜索结果统一为此结构。
class MediaInfo {
  final String source; // tmdb | douban | musicbrainz
  final String sourceId;
  final String title;
  final String originalTitle;
  final int year;
  final String type; // movie | tv | music
  final String overview;
  final String posterUrl;
  final String backdropUrl;
  final String releaseDate;
  final double rating;
  final List<String> genres;
  // 富字段（Detail 填充，搜索结果为空）：
  final int runtime; // 分钟
  final List<String> countries;
  final List<String> directors;
  final List<CreditPerson> cast;
  final List<MediaInfo> recommendations;

  MediaInfo({
    required this.source,
    required this.sourceId,
    required this.title,
    required this.originalTitle,
    required this.year,
    required this.type,
    required this.overview,
    required this.posterUrl,
    required this.backdropUrl,
    required this.releaseDate,
    required this.rating,
    this.genres = const [],
    this.runtime = 0,
    this.countries = const [],
    this.directors = const [],
    this.cast = const [],
    this.recommendations = const [],
  });

  factory MediaInfo.fromJson(Map<String, dynamic> j) => MediaInfo(
        source: j['source'] as String? ?? '',
        sourceId: j['source_id'] as String? ?? '',
        title: j['title'] as String? ?? '',
        originalTitle: j['original_title'] as String? ?? '',
        year: (j['year'] as num?)?.toInt() ?? 0,
        type: j['type'] as String? ?? '',
        overview: j['overview'] as String? ?? '',
        posterUrl: j['poster_url'] as String? ?? '',
        backdropUrl: j['backdrop_url'] as String? ?? '',
        releaseDate: j['release_date'] as String? ?? '',
        rating: (j['rating'] as num?)?.toDouble() ?? 0,
        genres: (j['genres'] as List?)?.map((e) => e as String).toList() ?? const [],
        runtime: (j['runtime'] as num?)?.toInt() ?? 0,
        countries: (j['countries'] as List?)?.map((e) => e as String).toList() ?? const [],
        directors: (j['directors'] as List?)?.map((e) => e as String).toList() ?? const [],
        cast: (j['cast'] as List?)
                ?.map((e) => CreditPerson.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
        recommendations: (j['recommendations'] as List?)
                ?.map((e) => MediaInfo.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
      );
}

/// 拆分 source_id 里的「类型:数字id」前缀；无冒号返回原值。
String _sourceSid(String id) {
  final i = id.indexOf(':');
  return i >= 0 ? id.substring(i + 1) : id;
}

extension MediaInfoDetail on MediaInfo {
  /// 各源官方详情页 URL（点卡片跳转用）。无匹配源或缺 id 返回空串。
  String get detailUrl {
    switch (source) {
      case 'douban':
        final sid = _sourceSid(sourceId);
        return sid.isEmpty ? '' : 'https://movie.douban.com/subject/$sid/';
      case 'tmdb':
        final parts = sourceId.split(':');
        if (parts.length < 2 || parts[1].isEmpty) return '';
        final kind = parts[0] == 'tv' ? 'tv' : 'movie';
        return 'https://www.themoviedb.org/$kind/${parts[1]}';
      case 'musicbrainz':
        return sourceId.isEmpty ? '' : 'https://musicbrainz.org/release-group/$sourceId';
    }
    return '';
  }
}

class MediaApi {
  final _api = ApiClient();

  // search 跨源搜媒体元数据：GET /media/search?keyword=...&year=...
  Future<List<MediaInfo>> search(String keyword, [int year = 0]) async {
    final q = 'keyword=${Uri.encodeQueryComponent(keyword)}&year=$year';
    final r = await _api.get('/media/search?$q');
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => MediaInfo.fromJson(e as Map<String, dynamic>)).toList();
  }

  // detail 取单条媒体富字段（演职/推荐/时长等）：GET /media/detail?source=&id=
  Future<MediaInfo> detail(String source, String id) async {
    final q = 'source=${Uri.encodeQueryComponent(source)}&id=${Uri.encodeQueryComponent(id)}';
    final r = await _api.get('/media/detail?$q');
    return MediaInfo.fromJson(r['result'] as Map<String, dynamic>);
  }
}

// 媒体源配置（TMDB/豆瓣/Bangumi 凭据）。GET 回显 secret 脱敏 + *_configured 标记；
// PUT 各字段空串=不修改（便于只改部分项），保存后即时 rebuild 源、无需重启。
class MediaSourceConfig {
  final String tmdbApiKey; // 脱敏
  final bool tmdbApiKeyConfigured;
  final String tmdbApiDomain; // 明文（非敏感）
  final String doubanSecret; // 脱敏
  final bool doubanSecretConfigured;
  final String doubanAppKey; // 脱敏
  final bool doubanAppKeyConfigured;
  final String bangumiToken; // 脱敏
  final bool bangumiTokenConfigured;

  const MediaSourceConfig({
    this.tmdbApiKey = '',
    this.tmdbApiKeyConfigured = false,
    this.tmdbApiDomain = '',
    this.doubanSecret = '',
    this.doubanSecretConfigured = false,
    this.doubanAppKey = '',
    this.doubanAppKeyConfigured = false,
    this.bangumiToken = '',
    this.bangumiTokenConfigured = false,
  });

  factory MediaSourceConfig.fromJson(Map<String, dynamic> j) => MediaSourceConfig(
        tmdbApiKey: j['tmdb_api_key'] as String? ?? '',
        tmdbApiKeyConfigured: j['tmdb_api_key_configured'] as bool? ?? false,
        tmdbApiDomain: j['tmdb_api_domain'] as String? ?? '',
        doubanSecret: j['douban_secret'] as String? ?? '',
        doubanSecretConfigured: j['douban_secret_configured'] as bool? ?? false,
        doubanAppKey: j['douban_app_key'] as String? ?? '',
        doubanAppKeyConfigured: j['douban_app_key_configured'] as bool? ?? false,
        bangumiToken: j['bangumi_token'] as String? ?? '',
        bangumiTokenConfigured: j['bangumi_token_configured'] as bool? ?? false,
      );
}

class MediaConfigApi {
  final _api = ApiClient();

  // 当前配置（secret 脱敏）：GET /media/config
  Future<MediaSourceConfig> get() async =>
      MediaSourceConfig.fromJson(await _api.get('/media/config'));

  // 保存配置（空串=不改）：PUT /media/config
  Future<void> set({
    String tmdbApiKey = '',
    String tmdbApiDomain = '',
    String doubanSecret = '',
    String doubanAppKey = '',
    String bangumiToken = '',
  }) async {
    await _api.put('/media/config', {
      'tmdb_api_key': tmdbApiKey,
      'tmdb_api_domain': tmdbApiDomain,
      'douban_secret': doubanSecret,
      'douban_app_key': doubanAppKey,
      'bangumi_token': bangumiToken,
    });
  }
}
