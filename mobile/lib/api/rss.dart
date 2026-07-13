import 'package:ptpatronus/core/api_client.dart';

// RSS 订阅（对齐后端 models.RssSubscription / RssItem + api/v1/rss.go）。

class RssSubscription {
  final int id;
  final String name;
  final String url;
  final int siteId;
  final bool enabled;
  final int intervalMin;
  final String titleInclude;
  final String titleExclude;
  final int minSize;
  final int maxSize;
  final String resolution;
  final bool autoDownload;
  final int downloaderId;
  final String savePath;
  final String category;
  final String tags;
  final String filterGroups; // 绑定过滤规则组名（| 分隔，空=不过滤）
  final String? lastCheckedAt;
  const RssSubscription({
    required this.id,
    required this.name,
    this.url = '',
    this.siteId = 0,
    this.enabled = true,
    this.intervalMin = 30,
    this.titleInclude = '',
    this.titleExclude = '',
    this.minSize = 0,
    this.maxSize = 0,
    this.resolution = '',
    this.autoDownload = false,
    this.downloaderId = 0,
    this.savePath = '',
    this.category = 'rss',
    this.tags = '',
    this.filterGroups = '',
    this.lastCheckedAt,
  });
  factory RssSubscription.fromJson(Map<String, dynamic> j) => RssSubscription(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        url: j['url'] as String? ?? '',
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        enabled: j['enabled'] as bool? ?? true,
        intervalMin: (j['interval_min'] as num?)?.toInt() ?? 30,
        titleInclude: j['title_include'] as String? ?? '',
        titleExclude: j['title_exclude'] as String? ?? '',
        minSize: (j['min_size'] as num?)?.toInt() ?? 0,
        maxSize: (j['max_size'] as num?)?.toInt() ?? 0,
        resolution: j['resolution'] as String? ?? '',
        autoDownload: j['auto_download'] as bool? ?? false,
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        savePath: j['save_path'] as String? ?? '',
        category: j['category'] as String? ?? 'rss',
        tags: j['tags'] as String? ?? '',
        filterGroups: j['filter_groups'] as String? ?? '',
        lastCheckedAt: j['last_checked_at'] as String?,
      );
}

class RssItem {
  final int id;
  final String title;
  final String link;
  final String torrentUrl;
  final int size;
  final bool downloaded;
  final String? createdAt;
  const RssItem({
    this.id = 0,
    this.title = '',
    this.link = '',
    this.torrentUrl = '',
    this.size = 0,
    this.downloaded = false,
    this.createdAt,
  });
  factory RssItem.fromJson(Map<String, dynamic> j) => RssItem(
        id: (j['id'] as num?)?.toInt() ?? 0,
        title: j['title'] as String? ?? '',
        link: j['link'] as String? ?? '',
        torrentUrl: j['torrent_url'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        downloaded: j['downloaded'] as bool? ?? false,
        createdAt: j['created_at'] as String?,
      );
}

class RssApi {
  final _api = ApiClient();

  Future<List<RssSubscription>> list() async {
    final r = await _api.get('/rss');
    final list = (r['subscriptions'] as List?) ?? const [];
    return list.map((e) => RssSubscription.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<RssSubscription> create(Map<String, dynamic> body) async {
    final r = await _api.post('/rss', body);
    return RssSubscription.fromJson(r);
  }

  Future<void> update(int id, Map<String, dynamic> body) => _api.put('/rss/$id', body);

  Future<void> remove(int id) => _api.delete('/rss/$id');

  Future<int> check(int id) async {
    final r = await _api.post('/rss/$id/check', {});
    return (r['new_count'] as num?)?.toInt() ?? 0;
  }

  Future<List<RssItem>> items(int id, {int limit = 100}) async {
    final r = await _api.get('/rss/$id/items?limit=$limit');
    final list = (r['items'] as List?) ?? const [];
    return list.map((e) => RssItem.fromJson(e as Map<String, dynamic>)).toList();
  }
}
