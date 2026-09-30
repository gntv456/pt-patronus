import '../core/api_client.dart';

class Site {
  final int id;
  final String name;
  final String platform;
  final String definition;
  final String baseUrl;
  final bool isActive;
  final int gameCount; // 已适配娱乐项目数（仅 /entertainment 端点返回；/sites 默认 0）
  Site({
    required this.id,
    required this.name,
    required this.platform,
    required this.definition,
    required this.baseUrl,
    required this.isActive,
    this.gameCount = 0,
  });
  factory Site.fromJson(Map<String, dynamic> j) => Site(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        platform: j['platform'] as String? ?? '',
        definition: j['definition'] as String? ?? '',
        baseUrl: j['base_url'] as String? ?? '',
        isActive: j['is_active'] as bool? ?? false,
        gameCount: (j['game_count'] as num?)?.toInt() ?? 0,
      );
}

enum SiteConnectivityState { connected, disconnected, disabled }

class SiteConnectivityStatus {
  final int siteId;
  final String siteName;
  final SiteConnectivityState state;
  final String level;
  final String error;

  const SiteConnectivityStatus({
    required this.siteId,
    required this.siteName,
    required this.state,
    this.level = '',
    this.error = '',
  });

  factory SiteConnectivityStatus.fromJson(Map<String, dynamic> j) {
    final raw = j['status'] as String? ?? '';
    final state = switch (raw) {
      'connected' => SiteConnectivityState.connected,
      'disabled' => SiteConnectivityState.disabled,
      _ => SiteConnectivityState.disconnected,
    };
    return SiteConnectivityStatus(
      siteId: (j['site_id'] as num?)?.toInt() ?? 0,
      siteName: j['site_name'] as String? ?? '',
      state: state,
      level: j['level'] as String? ?? '',
      error: j['error'] as String? ?? '',
    );
  }
}

/// 站点定义（definitions/*.yaml 模板，供「添加站点」选择）。
class SiteDef {
  final String id;
  final String name;
  final String schema; // nexusphp / mteam / ...
  final bool dead; // 已关站：仅展示标注，不参与活跃抓取
  final List<String> urls; // 站点真实地址（首个作添加站点默认地址）
  final List<String> aka; // 中文名别名（很多站 name 是英文，靠 aka 才能按中文名检索）
  final String description;
  final List<String> tags;
  const SiteDef({
    required this.id,
    required this.name,
    required this.schema,
    this.dead = false,
    this.urls = const [],
    this.aka = const [],
    this.description = '',
    this.tags = const [],
  });
  factory SiteDef.fromJson(Map<String, dynamic> j) => SiteDef(
        id: j['id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        schema: j['schema'] as String? ?? '',
        dead: (j['dead'] as bool?) ?? false,
        urls: (j['urls'] as List?)?.map((e) => e as String).toList() ?? const [],
        aka: (j['aka'] as List?)?.map((e) => e as String).toList() ?? const [],
        description: j['description'] as String? ?? '',
        tags: (j['tags'] as List?)?.map((e) => e as String).toList() ?? const [],
      );
}

/// 签到历史记录（cron 自动 + 手动触发均落库）。
class CheckinRecord {
  final int id;
  final int siteId;
  final String siteName;
  final bool success;
  final String message;
  final double bonus;
  final String error;
  final String source; // cron | manual
  final String createdAt;
  const CheckinRecord({
    required this.id,
    required this.siteId,
    required this.siteName,
    required this.success,
    required this.message,
    required this.bonus,
    required this.error,
    required this.source,
    required this.createdAt,
  });
  factory CheckinRecord.fromJson(Map<String, dynamic> j) => CheckinRecord(
        id: (j['id'] as num?)?.toInt() ?? 0,
        siteId: j['site_id'] as int? ?? 0,
        siteName: j['site_name'] as String? ?? '',
        success: j['success'] == true,
        message: j['message'] as String? ?? '',
        bonus: (j['bonus'] as num?)?.toDouble() ?? 0,
        error: j['error'] as String? ?? '',
        source: j['source'] as String? ?? '',
        createdAt: j['created_at'] as String? ?? '',
      );
}

class SiteApi {
  final _api = ApiClient();

  Future<List<Site>> list() async {
    final r = await _api.get('/sites');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => Site.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 可用站点定义列表（添加站点表单的下拉项来源）。
  Future<List<SiteDef>> definitions() async {
    final r = await _api.get('/sites/definitions');
    final list = (r['definitions'] as List?) ?? const [];
    return list.map((e) => SiteDef.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 添加站点。platform 由所选定义的 schema 推断；apiKey 仅 mteam 等需 x-api-key 的站用；
  /// cookie/username/password 对齐 Web 添加表单（非馒头站可手填，留空不发）。
  Future<Site> add({
    required String name,
    required String platform,
    required String definition,
    required String baseUrl,
    String? apiKey,
    String? cookie,
    String? username,
    String? password,
    bool isActive = true,
  }) async {
    final body = <String, dynamic>{
      'name': name,
      'platform': platform,
      'definition': definition,
      'base_url': baseUrl,
      'is_active': isActive,
    };
    if (apiKey != null && apiKey.isNotEmpty) body['api_key'] = apiKey;
    if (cookie != null && cookie.isNotEmpty) body['cookie'] = cookie;
    if (username != null && username.isNotEmpty) body['username'] = username;
    if (password != null && password.isNotEmpty) body['password'] = password;
    final r = await _api.post('/sites', body);
    return Site.fromJson(r);
  }

  /// 编辑站点（对齐 Web PUT /sites/:id）。platform 必传（后端 required）；
  /// apiKey/cookie/password 留空 = 不修改。
  Future<Site> update(
    int id, {
    required String name,
    required String platform,
    required String baseUrl,
    String? apiKey,
    String? cookie,
    String? username,
    String? password,
    bool? isActive,
  }) async {
    final body = <String, dynamic>{'name': name, 'platform': platform, 'base_url': baseUrl};
    if (isActive != null) body['is_active'] = isActive;
    if (apiKey != null && apiKey.isNotEmpty) body['api_key'] = apiKey;
    if (cookie != null && cookie.isNotEmpty) body['cookie'] = cookie;
    if (username != null) body['username'] = username;
    if (password != null && password.isNotEmpty) body['password'] = password;
    final r = await _api.put('/sites/$id', body);
    return Site.fromJson(r);
  }

  /// 连接测试：返回 {ok, level?, error?}。馒头走此端点验证 x-api-key。
  Future<Map<String, dynamic>> test(int id) async =>
      _api.post('/sites/$id/test', <String, dynamic>{});

  /// 批量检查站点联通状态：返回 {statuses:[{site_id, site_name, status, level?, error?}]}。
  Future<List<SiteConnectivityStatus>> statuses() async {
    final r = await _api.get('/sites/status');
    final list = (r['statuses'] as List?) ?? const [];
    return list
        .map((e) => SiteConnectivityStatus.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 单站签到：返回 {site_id, site_name, success, message?, bonus, error?}。
  /// 仅 nexusphp 等实现了签到的适配器有效；馒头等会 success=false 且 error 含 not supported。
  Future<Map<String, dynamic>> checkin(int id) async =>
      _api.post('/sites/$id/checkin', <String, dynamic>{});

  /// 一键签到全部启用站点，返回每站结果列表。
  Future<List<Map<String, dynamic>>> checkinAll() async {
    final r = await _api.post('/sites/checkin', <String, dynamic>{});
    final list = (r['results'] as List?) ?? const [];
    return list.map((e) => Map<String, dynamic>.from(e as Map)).toList();
  }

  /// 签到历史记录（siteId=null 全部；按时间倒序，默认 100 条）。
  Future<List<CheckinRecord>> records({int? siteId, int limit = 100, int offset = 0}) async {
    final qs = <String>['limit=$limit', 'offset=$offset'];
    if (siteId != null) qs.add('site_id=$siteId');
    final r = await _api.get('/sites/checkin/records?${qs.join('&')}');
    final list = (r['records'] as List?) ?? const [];
    return list.map((e) => CheckinRecord.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> remove(int id) async => _api.delete('/sites/$id');

  /// 批量删除站点（admin）。返回 deleted（成功数）与 failed（失败 id 列表）。
  Future<Map<String, dynamic>> batchDelete(List<int> ids) async =>
      _api.post('/sites/batch-delete', {'ids': ids});

  /// 上传站点 cookie（WebView 登录后调用）。
  Future<void> uploadCookie(int id, String cookie) async =>
      _api.post('/sites/$id/cookie', {'cookie': cookie});
}
