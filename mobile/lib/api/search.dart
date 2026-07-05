import '../core/api_client.dart';

// SearchTorrent 是一条搜索命中的种子（区别于 downloader.Torrent：那是下载器任务，
// 含 hash/progress；这里是站点搜索结果，含 seeders/download_url）。
class SearchTorrent {
  final String id;
  final String title;
  final String subtitle;
  final String size; // 站点已格式化的显示串，如 "1.5 GB"
  final int seeders;
  final int leechers;
  final int completed;
  final bool free;
  final String detailUrl;
  final String downloadUrl; // nexusphp 有；mteam 等需 genDlToken 的为空

  SearchTorrent({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.size,
    required this.seeders,
    required this.leechers,
    required this.completed,
    required this.free,
    required this.detailUrl,
    required this.downloadUrl,
  });

  factory SearchTorrent.fromJson(Map<String, dynamic> j) => SearchTorrent(
        id: j['id']?.toString() ?? '',
        title: j['title'] as String? ?? '',
        subtitle: j['subtitle'] as String? ?? '',
        size: j['size'] as String? ?? '',
        seeders: (j['seeders'] as num?)?.toInt() ?? 0,
        leechers: (j['leechers'] as num?)?.toInt() ?? 0,
        completed: (j['completed'] as num?)?.toInt() ?? 0,
        free: j['free'] as bool? ?? false,
        detailUrl: j['detail_url'] as String? ?? '',
        downloadUrl: j['download_url'] as String? ?? '',
      );
}

// SiteSearchResult 是单站搜索结果（含错误，便于聚合容错展示）。
class SiteSearchResult {
  final int siteId;
  final String siteName;
  final List<SearchTorrent> torrents;
  final String error;

  SiteSearchResult({
    required this.siteId,
    required this.siteName,
    required this.torrents,
    required this.error,
  });

  factory SiteSearchResult.fromJson(Map<String, dynamic> j) => SiteSearchResult(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        torrents: ((j['torrents'] as List?) ?? const [])
            .map((e) => SearchTorrent.fromJson(e as Map<String, dynamic>))
            .toList(),
        error: j['error'] as String? ?? '',
      );
}

class SearchApi {
  final _api = ApiClient();

  // search 跨站聚合搜索：POST /search?keyword=...[&category=...][&site_ids=...]。
  // 单站失败不影响其它（error 在结果里）。category=movie/tv/music（空=综合）：
  // MTeam 映射 mode、NexusPHP 发 cat[]+后置过滤音乐。siteIds 非空时仅搜指定启用站点。
  Future<List<SiteSearchResult>> search(
    String keyword, {
    String category = '',
    List<int> siteIds = const [],
  }) async {
    var qs = 'keyword=${Uri.encodeQueryComponent(keyword)}';
    if (category.isNotEmpty) {
      qs += '&category=${Uri.encodeQueryComponent(category)}';
    }
    if (siteIds.isNotEmpty) {
      qs += '&site_ids=${siteIds.join(',')}';
    }
    final r = await _api.post('/search?$qs', {});
    final list = (r['results'] as List?) ?? const [];
    return list
        .map((e) => SiteSearchResult.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}
