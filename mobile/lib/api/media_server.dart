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
}
