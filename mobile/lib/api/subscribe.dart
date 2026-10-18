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
  final bool? enableRatingGuard; // 三态：null=跟随全局；true/false=订阅自定义
  final bool? bestVersion; // 三态：null=跟随全局；true/false=订阅自定义
  final List<String> promoTypes; // 接受的促销类型多选；空=跟随全局/不过滤
  final List<int> siteIds; // 参与站点 id；空=全部活跃站点
  final int currentPriority; // 洗版电影当前最优 PriOrder（0=未下过）
  final String? lastCheckedAt;
  // 上一轮检查里「读不出来的站」留下的原因（空串=整轮干净）。
  // 只有检查时刻时，「2 个站里 1 个 cookie 失效所以什么都没搜到」看着就像「确实没新命中」。
  final String lastCheckError;
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
    this.enableRatingGuard,
    this.bestVersion,
    this.promoTypes = const [],
    this.siteIds = const [],
    this.currentPriority = 0,
    required this.lastCheckedAt,
    this.lastCheckError = '',
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
        enableRatingGuard: j['enable_rating_guard'] as bool?,
        bestVersion: j['best_version'] as bool?,
        promoTypes: (j['promo_types'] as List?)?.map((e) => e.toString()).toList() ?? const [],
        siteIds: (j['site_ids'] as List?)?.map((e) => (e as num).toInt()).toList() ?? const [],
        currentPriority: (j['current_priority'] as num?)?.toInt() ?? 0,
        lastCheckedAt: j['last_checked_at'] as String?,
        lastCheckError: j['last_check_error'] as String? ?? '',
        createdAt: j['created_at'] as String? ?? '',
      );

  /// 乐观更新用：复制指定字段（订阅列表卡片 inline 编辑回写本地）。
  Subscription copyWith({
    List<String>? promoTypes,
    List<int>? siteIds,
    bool? enableRatingGuard,
    bool? bestVersion,
    String? resolution,
  }) {
    return Subscription(
      id: id,
      source: source,
      sourceId: sourceId,
      title: title,
      year: year,
      type: type,
      category: category,
      posterUrl: posterUrl,
      resolution: resolution ?? this.resolution,
      downloaderId: downloaderId,
      savePath: savePath,
      status: status,
      enableRatingGuard: enableRatingGuard ?? this.enableRatingGuard,
      bestVersion: bestVersion ?? this.bestVersion,
      promoTypes: promoTypes ?? this.promoTypes,
      siteIds: siteIds ?? this.siteIds,
      currentPriority: currentPriority,
      lastCheckedAt: lastCheckedAt,
      createdAt: createdAt,
    );
  }
}

/// 全局订阅默认设置：未单独设置某项的订阅回退到此（运行时回退）。
class GlobalDefaults {
  final String resolution; // 空=不限
  final List<String> promoTypes; // 空=不限
  final List<int> siteIds; // 空=全部活跃站点
  final bool enableRatingGuard; // 全局二态，默认 false
  final bool bestVersion; // 全局二态，默认 false

  const GlobalDefaults({
    this.resolution = '',
    this.promoTypes = const [],
    this.siteIds = const [],
    this.enableRatingGuard = false,
    this.bestVersion = false,
  });

  factory GlobalDefaults.fromJson(Map<String, dynamic> j) => GlobalDefaults(
        resolution: j['resolution'] as String? ?? '',
        promoTypes: (j['promo_types'] as List?)?.map((e) => e.toString()).toList() ?? const [],
        siteIds: (j['site_ids'] as List?)?.map((e) => (e as num).toInt()).toList() ?? const [],
        enableRatingGuard: j['enable_rating_guard'] == true,
        bestVersion: j['best_version'] == true,
      );

  Map<String, dynamic> toJson() => {
        'resolution': resolution,
        'promo_types': promoTypes,
        'site_ids': siteIds,
        'enable_rating_guard': enableRatingGuard,
        'best_version': bestVersion,
      };

  GlobalDefaults copyWith({
    String? resolution,
    List<String>? promoTypes,
    List<int>? siteIds,
    bool? enableRatingGuard,
    bool? bestVersion,
  }) =>
      GlobalDefaults(
        resolution: resolution ?? this.resolution,
        promoTypes: promoTypes ?? this.promoTypes,
        siteIds: siteIds ?? this.siteIds,
        enableRatingGuard: enableRatingGuard ?? this.enableRatingGuard,
        bestVersion: bestVersion ?? this.bestVersion,
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

  // 全局订阅默认设置：GET/PUT /subscribe/settings（未单独设置的订阅回退到此）。
  Future<GlobalDefaults> getGlobalDefaults() async {
    final r = await _api.get('/subscribe/settings');
    return GlobalDefaults.fromJson(r);
  }

  Future<GlobalDefaults> updateGlobalDefaults(GlobalDefaults d) async {
    final r = await _api.put('/subscribe/settings', d.toJson());
    return GlobalDefaults.fromJson(r);
  }

  // 清空订阅指定字段回退全局默认：POST /subscriptions/:id/reset
  Future<Subscription> reset(int id, List<String> fields) async {
    final r = await _api.post('/subscriptions/$id/reset', {'fields': fields});
    return Subscription.fromJson(r);
  }
}
