import '../core/api_client.dart';
import '../features/common/stream_account.dart';

// 统一媒体元数据（对齐 backend media.MediaInfo）。
class MediaInfo {
  final String source; // tmdb | douban | bangumi | musicbrainz
  final String sourceId;
  final String title;
  final String originalTitle;
  final int year;
  final String type; // movie | tv | music
  final String overview;
  final String posterUrl;
  final String releaseDate;
  final double rating;

  MediaInfo({
    required this.source,
    required this.sourceId,
    required this.title,
    required this.originalTitle,
    required this.year,
    required this.type,
    required this.overview,
    required this.posterUrl,
    required this.releaseDate,
    required this.rating,
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
        releaseDate: j['release_date'] as String? ?? '',
        rating: (j['rating'] as num?)?.toDouble() ?? 0,
      );
}

class RecommendApi {
  RecommendApi({StreamFrames? frames}) : _frames = frames;

  final StreamFrames? _frames;
  final _api = ApiClient();

  Stream<MapEntry<String, Map<String, dynamic>>> _stream(String path) =>
      _frames?.call(path) ?? _api.streamGet(path);

  // 热门/趋势信息流：GET /media/recommend?source=&category=&page=&refresh=1
  Future<List<MediaInfo>> feed({
    String source = '',
    String category = 'movie',
    int page = 1,
    bool refresh = false,
  }) async {
    var q = '?category=${Uri.encodeQueryComponent(category)}&page=$page';
    if (source.isNotEmpty) q += '&source=${Uri.encodeQueryComponent(source)}';
    if (refresh) q += '&refresh=1';
    final r = await _api.get('/media/recommend$q');
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => MediaInfo.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 流式首屏（SSE）：每源完成推一帧 batch；done 帧带源账面（跑了几个源、谁失败了）。
  Stream<StreamBatch<MediaInfo>> feedStream({
    String source = '',
    String category = 'movie',
    int page = 1,
    bool refresh = false,
  }) {
    var q = '?category=${Uri.encodeQueryComponent(category)}&page=$page';
    if (source.isNotEmpty) q += '&source=${Uri.encodeQueryComponent(source)}';
    if (refresh) q += '&refresh=1';
    return _stream('/media/recommend/stream$q').map((ev) {
      final batch = mapStreamFrame(ev, MediaInfo.fromJson);
      if (ev.key != 'done') return batch;
      return StreamBatch(
        items: batch.items,
        listError: recommendStreamNotice(ev.value),
      );
    });
  }
}
