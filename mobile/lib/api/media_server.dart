import 'dart:typed_data';

import '../core/api_client.dart';

/// 媒体服务器（对齐后端 /mediaserver）：Emby / Jellyfin / Plex 多实例 + 库操作 + 封面。
///
/// snake_case JSON → camelCase 字段，解析范式同 [BrushFlowTask.fromJson]。
class MediaServer {
  final int id;
  final String name;
  final String type; // emby | jellyfin | plex
  final String url;
  final String username;
  final bool isDefault;
  final bool refreshOnDownload;
  final bool notifyOnPlay;
  final bool directLinkEnabled;
  final String directLinkBase;
  final DateTime createdAt;
  final DateTime updatedAt;

  MediaServer({
    required this.id,
    required this.name,
    required this.type,
    required this.url,
    required this.username,
    required this.isDefault,
    required this.refreshOnDownload,
    required this.notifyOnPlay,
    required this.directLinkEnabled,
    required this.directLinkBase,
    required this.createdAt,
    required this.updatedAt,
  });

  factory MediaServer.fromJson(Map<String, dynamic> j) => MediaServer(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? 'emby',
        url: j['url'] as String? ?? '',
        username: j['username'] as String? ?? '',
        isDefault: j['is_default'] as bool? ?? false,
        refreshOnDownload: j['refresh_on_download'] as bool? ?? false,
        notifyOnPlay: j['notify_on_play'] as bool? ?? false,
        directLinkEnabled: j['direct_link_enabled'] as bool? ?? false,
        directLinkBase: j['direct_link_base'] as String? ?? '',
        createdAt: DateTime.tryParse(j['created_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
        updatedAt: DateTime.tryParse(j['updated_at'] as String? ?? '') ??
            DateTime.fromMillisecondsSinceEpoch(0),
      );

  MediaServer copyWith({bool? refreshOnDownload, bool? notifyOnPlay}) =>
      MediaServer(
        id: id,
        name: name,
        type: type,
        url: url,
        username: username,
        isDefault: isDefault,
        refreshOnDownload: refreshOnDownload ?? this.refreshOnDownload,
        notifyOnPlay: notifyOnPlay ?? this.notifyOnPlay,
        directLinkEnabled: directLinkEnabled,
        directLinkBase: directLinkBase,
        createdAt: createdAt,
        updatedAt: updatedAt,
      );
}

/// 媒体库（GET /mediaserver/:id/libraries）。
class MediaLibrary {
  final String id;
  final String name;
  final String type;

  MediaLibrary({required this.id, required this.name, required this.type});

  factory MediaLibrary.fromJson(Map<String, dynamic> j) => MediaLibrary(
        id: j['id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? '',
      );
}

/// 播放记录（观影报告）。
class PlayItem {
  final String itemId;
  final String name;
  final String type;
  final int year;
  final String userName;
  final String lastPlayed;
  final int playCount;
  final int duration; // 秒

  PlayItem({
    required this.itemId,
    required this.name,
    required this.type,
    required this.year,
    required this.userName,
    required this.lastPlayed,
    required this.playCount,
    required this.duration,
  });

  factory PlayItem.fromJson(Map<String, dynamic> j) => PlayItem(
        itemId: j['item_id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? '',
        year: (j['year'] as num?)?.toInt() ?? 0,
        userName: j['user_name'] as String? ?? '',
        lastPlayed: j['last_played'] as String? ?? '',
        playCount: (j['play_count'] as num?)?.toInt() ?? 0,
        duration: (j['duration'] as num?)?.toInt() ?? 0,
      );
}

/// 观影报告用户（播放次数聚合）。
class PlayUser {
  final String id;
  final String name;
  final int playCount;
  PlayUser({required this.id, required this.name, required this.playCount});
  factory PlayUser.fromJson(Map<String, dynamic> j) => PlayUser(
        id: j['id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        playCount: (j['play_count'] as num?)?.toInt() ?? 0,
      );
}

/// 观影报告（GET /mediaserver/:id/playback-report）。
class PlaybackReport {
  final String server;
  final int days;
  final List<PlayUser> users;
  final List<PlayItem> recentPlays;
  final List<PlayItem> topItems;
  final int totalPlays;

  PlaybackReport({
    required this.server,
    required this.days,
    required this.users,
    required this.recentPlays,
    required this.topItems,
    required this.totalPlays,
  });

  factory PlaybackReport.fromJson(Map<String, dynamic> j) {
    final us = (j['users'] as List?) ?? const [];
    final rp = (j['recent_plays'] as List?) ?? const [];
    final tp = (j['top_items'] as List?) ?? const [];
    return PlaybackReport(
      server: j['server'] as String? ?? '',
      days: (j['days'] as num?)?.toInt() ?? 30,
      users: us.map((e) => PlayUser.fromJson(e as Map<String, dynamic>)).toList(),
      recentPlays: rp.map((e) => PlayItem.fromJson(e as Map<String, dynamic>)).toList(),
      topItems: tp.map((e) => PlayItem.fromJson(e as Map<String, dynamic>)).toList(),
      totalPlays: (j['total_plays'] as num?)?.toInt() ?? 0,
    );
  }
}

/// 重复检测的一组条目（GET /mediaserver/:id/duplicates）。
class DuplicateGroup {
  final String title;
  final int year;
  final int count;
  final List<String> items; // 各份条目名

  DuplicateGroup({
    required this.title,
    required this.year,
    required this.count,
    required this.items,
  });

  factory DuplicateGroup.fromJson(Map<String, dynamic> j) {
    final its = (j['items'] as List?) ?? const [];
    return DuplicateGroup(
      title: j['title'] as String? ?? '',
      year: (j['year'] as num?)?.toInt() ?? 0,
      count: (j['count'] as num?)?.toInt() ?? 0,
      items: its
          .map((e) => (e as Map<String, dynamic>)['name'] as String? ?? '')
          .toList(),
    );
  }
}

/// 媒体服务器 API（对齐后端 /mediaserver/*）。
class MediaServerApi {
  final _api = ApiClient();

  /// 列表（GET /mediaserver → {servers:[...]}）。
  Future<List<MediaServer>> list() async {
    final r = await _api.get('/mediaserver');
    final list = (r['servers'] as List?) ?? const [];
    return list
        .map((e) => MediaServer.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 新建（POST /mediaserver）。返回后端响应（含新建记录或 error）。
  Future<Map<String, dynamic>> create(Map<String, dynamic> body) =>
      _api.post('/mediaserver', body);

  /// 更新（PUT /mediaserver/:id）。
  Future<Map<String, dynamic>> update(int id, Map<String, dynamic> body) =>
      _api.put('/mediaserver/$id', body);

  /// 删除（DELETE /mediaserver/:id）。
  Future<void> remove(int id) async => _api.delete('/mediaserver/$id');

  /// 连接测试（POST /mediaserver/:id/test）。返回 {ok, info?, error?}。
  Future<Map<String, dynamic>> test(int id) async =>
      _api.post('/mediaserver/$id/test', <String, dynamic>{});

  /// 媒体库列表（GET /mediaserver/:id/libraries）。
  Future<List<MediaLibrary>> libraries(int id) async {
    final r = await _api.get('/mediaserver/$id/libraries');
    final list = (r['libraries'] as List?) ?? const [];
    return list
        .map((e) => MediaLibrary.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 触发扫描（POST /mediaserver/:id/scan）。libraryId 空串=全库。
  Future<Map<String, dynamic>> scan(int id, String libraryId) async =>
      _api.post('/mediaserver/$id/scan', <String, dynamic>{'library_id': libraryId});

  /// 刷新库元数据（POST /mediaserver/:id/refresh）。
  Future<Map<String, dynamic>> refresh(int id, String libraryId) async =>
      _api.post('/mediaserver/$id/refresh', <String, dynamic>{'library_id': libraryId});

  /// 通知（POST /mediaserver/:id/notify）。
  Future<Map<String, dynamic>> notify(int id, String title, String body) async =>
      _api.post('/mediaserver/$id/notify', <String, dynamic>{'title': title, 'body': body});

  /// 封面拼图字节（GET /mediaserver/:id/cover?library_id=…，返回 image/jpeg）。
  Future<Uint8List> cover(int id, String libraryId, {int cols = 3, int rows = 3}) async =>
      _api.getBytes(
          '/mediaserver/$id/cover?library_id=${Uri.encodeQueryComponent(libraryId)}&cols=$cols&rows=$rows');

  /// 观影报告（GET /mediaserver/:id/playback-report?days=…）。
  Future<PlaybackReport> playbackReport(int id, {int days = 30}) async {
    final r = await _api.get('/mediaserver/$id/playback-report?days=$days');
    return PlaybackReport.fromJson(r);
  }

  /// 重复媒体检测（GET /mediaserver/:id/duplicates?library_id=…）。
  Future<List<DuplicateGroup>> duplicates(int id, String libraryId) async {
    final r = await _api
        .get('/mediaserver/$id/duplicates?library_id=${Uri.encodeQueryComponent(libraryId)}');
    final list = (r['groups'] as List?) ?? const [];
    return list.map((e) => DuplicateGroup.fromJson(e as Map<String, dynamic>)).toList();
  }
}
