import '../core/api_client.dart';

// 订阅记录（对齐 backend models.Subscription）：cron 自动跨站搜种子 → 命中推送默认下载器 + 通知。
class Subscription {
  final int id;
  final String source; // tmdb | douban | bangumi
  final String sourceId;
  final String title;
  final int year;
  final String type; // movie | tv
  final String category;
  final String posterUrl;
  final String resolution; // 1080p/2160p，空=不过滤
  final int downloaderId; // 0=默认下载器
  final String savePath;
  final String status; // subscribed | completed | paused
  final String? lastCheckedAt;
  final String createdAt;

  Subscription({
    required this.id,
    required this.source,
    required this.sourceId,
    required this.title,
    required this.year,
    required this.type,
    required this.category,
    required this.posterUrl,
    required this.resolution,
    required this.downloaderId,
    required this.savePath,
    required this.status,
    required this.lastCheckedAt,
    required this.createdAt,
  });

  factory Subscription.fromJson(Map<String, dynamic> j) => Subscription(
        id: (j['id'] as num?)?.toInt() ?? 0,
        source: j['source'] as String? ?? '',
        sourceId: j['source_id'] as String? ?? '',
        title: j['title'] as String? ?? '',
        year: (j['year'] as num?)?.toInt() ?? 0,
        type: j['type'] as String? ?? '',
        category: j['category'] as String? ?? '',
        posterUrl: j['poster_url'] as String? ?? '',
        resolution: j['resolution'] as String? ?? '',
        downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
        savePath: j['save_path'] as String? ?? '',
        status: j['status'] as String? ?? '',
        lastCheckedAt: j['last_checked_at'] as String?,
        createdAt: j['created_at'] as String? ?? '',
      );
}

class SubscribeApi {
  final _api = ApiClient();

  // 列出全部订阅：GET /subscriptions
  Future<List<Subscription>> list() async {
    final r = await _api.get('/subscriptions');
    final list = (r['subscriptions'] as List?) ?? const [];
    return list.map((e) => Subscription.fromJson(e as Map<String, dynamic>)).toList();
  }

  // 新建订阅（建后后端立即异步检查一次）：POST /subscriptions。同源同 id 幂等。
  Future<Subscription> create({
    required String source,
    required String sourceId,
    required String title,
    int year = 0,
    String type = '',
    String category = '',
    String posterUrl = '',
    String resolution = '',
  }) async {
    final r = await _api.post('/subscriptions', {
      'source': source,
      'source_id': sourceId,
      'title': title,
      'year': year,
      'type': type,
      'category': category,
      'poster_url': posterUrl,
      if (resolution.isNotEmpty) 'resolution': resolution,
    });
    return Subscription.fromJson(r);
  }

  // 编辑订阅（resolution/status/downloader_id/save_path，空=不改）：PUT /subscriptions/:id
  Future<Subscription> update(int id, Map<String, dynamic> patch) async {
    final r = await _api.put('/subscriptions/$id', patch);
    return Subscription.fromJson(r);
  }

  // 删除订阅：DELETE /subscriptions/:id
  Future<void> remove(int id) async {
    await _api.delete('/subscriptions/$id');
  }

  // 手动触发单个订阅检查，返回 {ok, pushed, error?}：POST /subscriptions/:id/check
  Future<Map<String, dynamic>> check(int id) async {
    return _api.post('/subscriptions/$id/check', <String, dynamic>{});
  }
}
