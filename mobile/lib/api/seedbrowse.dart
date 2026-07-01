import '../core/api_client.dart';

// 种子详情页（对齐 backend site.TorrentDetail）。字段 best-effort：站点未抽到的留空。
// 由搜索页「种子」模式的详情抽屉消费（对齐 Web Search.vue 的 openTorrentDetail）。
class TorrentDetail {
  final String title;
  final String subtitle;
  final String posterUrl;
  final String description; // 原始 HTML（展示前剥标签）
  final String mediainfo;
  final String detailUrl;
  final String downloadUrl;
  final String category;
  final String size;
  final String codec;
  final String source;
  final String resolution;
  final String imdbId;
  final String doubanId;
  final int seeders;
  final int leechers;
  final List<String> screenshots;
  final List<String> fileList;

  TorrentDetail({
    required this.title,
    required this.subtitle,
    required this.posterUrl,
    required this.description,
    required this.mediainfo,
    required this.detailUrl,
    required this.downloadUrl,
    required this.category,
    required this.size,
    required this.codec,
    required this.source,
    required this.resolution,
    required this.imdbId,
    required this.doubanId,
    required this.seeders,
    required this.leechers,
    required this.screenshots,
    required this.fileList,
  });

  factory TorrentDetail.fromJson(Map<String, dynamic> j) => TorrentDetail(
        title: j['title'] as String? ?? '',
        subtitle: j['subtitle'] as String? ?? '',
        posterUrl: j['poster_url'] as String? ?? '',
        description: j['description'] as String? ?? '',
        mediainfo: j['mediainfo'] as String? ?? '',
        detailUrl: j['detail_url'] as String? ?? '',
        downloadUrl: j['download_url'] as String? ?? '',
        category: j['category'] as String? ?? '',
        size: j['size'] as String? ?? '',
        codec: j['codec'] as String? ?? '',
        source: j['source'] as String? ?? '',
        resolution: j['resolution'] as String? ?? '',
        imdbId: j['imdb_id'] as String? ?? '',
        doubanId: j['douban_id'] as String? ?? '',
        seeders: (j['seeders'] as num?)?.toInt() ?? 0,
        leechers: (j['leechers'] as num?)?.toInt() ?? 0,
        screenshots: ((j['screenshots'] as List?) ?? const []).map((e) => e.toString()).toList(),
        fileList: ((j['file_list'] as List?) ?? const []).map((e) => e.toString()).toList(),
      );
}

class SeedBrowseApi {
  final _api = ApiClient();

  // 抓取并解析单条种子详情页：GET /sites/:id/torrent/detail?url=...
  Future<TorrentDetail?> detail(int siteId, String url) async {
    final r = await _api.get('/sites/$siteId/torrent/detail?url=${Uri.encodeQueryComponent(url)}');
    if (r['error'] != null) throw Exception(r['error']);
    final d = r['detail'];
    if (d == null) return null;
    return TorrentDetail.fromJson(d as Map<String, dynamic>);
  }
}
