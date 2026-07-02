import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:http/http.dart' as http;

import '../core/api_client.dart';

/// 存储后端（WebDAV / Alist / SMB / Agent）。对齐后端 FileStorage（密码 json:"-" 不外泄）。
class FileStorage {
  final int id;
  final String name;
  final String type; // webdav | alist | smb | agent
  final String url;
  final String username;
  final String basePath;
  final int agentId; // type=agent 时关联 Agent；其他类型为 0
  FileStorage({
    required this.id,
    required this.name,
    required this.type,
    required this.url,
    required this.username,
    required this.basePath,
    required this.agentId,
  });
  factory FileStorage.fromJson(Map<String, dynamic> j) => FileStorage(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        type: j['type'] as String? ?? '',
        url: j['url'] as String? ?? '',
        username: j['username'] as String? ?? '',
        basePath: j['base_path'] as String? ?? '',
        agentId: j['agent_id'] as int? ?? 0,
      );
}

/// 目录条目。对齐后端 Entry（snake_case）。
class FileEntry {
  final String name;
  final String path;
  final bool isDir;
  final int size;
  final String modTime;
  FileEntry({
    required this.name,
    required this.path,
    required this.isDir,
    required this.size,
    required this.modTime,
  });
  factory FileEntry.fromJson(Map<String, dynamic> j) => FileEntry(
        name: j['name'] as String? ?? '',
        path: j['path'] as String? ?? '',
        isDir: j['is_dir'] as bool? ?? false,
        size: j['size'] as int? ?? 0,
        modTime: j['mod_time'] as String? ?? '',
      );
}

/// 文件存储 API：列存储 / 列目录 / 上传 / 下载 / 建目录。对齐后端 /storages/*。
/// 注意后端软失败约定：files/upload/mkdir 出错返 200 + error/ok 字段，必须显式检查。
class FileStorageApi {
  final _api = ApiClient();

  Future<List<FileStorage>> list() async {
    final r = await _api.get('/storages');
    final arr = (r['storages'] as List?) ?? const [];
    return arr.map((e) => FileStorage.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 列目录。后端软失败：出错返 {entries:[], error}，这里转成异常。
  Future<List<FileEntry>> files(int id, String path) async {
    final r = await _api.get('/storages/$id/files?path=${Uri.encodeComponent(path)}');
    final err = r['error'] as String?;
    if (err != null && err.isNotEmpty) throw Exception(err);
    final arr = (r['entries'] as List?) ?? const [];
    return arr.map((e) => FileEntry.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 上传文件到 dir 目录（multipart：path 走 fields，文件字段名 file）。
  Future<void> upload(int id, String dir, http.MultipartFile file) async {
    final r = await _api.postMultipart(
      '/storages/$id/upload',
      files: [file],
      fields: {'path': dir},
    );
    _checkOk(r);
  }

  /// 下载文件字节流（出错后端返 500 → getBytes 抛 ApiException）。
  Future<Uint8List> download(int id, String path) =>
      _api.getBytes('/storages/$id/download?path=${Uri.encodeComponent(path)}');

  /// 流式下载到本地文件（大文件避免全量入内存 OOM；支持进度/取消）。
  Future<void> downloadToFile(int id, String path, String savePath,
          {void Function(int, int)? onProgress, CancelToken? cancelToken}) =>
      _api.downloadToFile(
        '/storages/$id/download?path=${Uri.encodeComponent(path)}',
        savePath,
        onProgress: onProgress,
        cancelToken: cancelToken,
      );

  /// 建目录（path 走 query，body 空）。
  Future<void> mkdir(int id, String path) async {
    final r = await _api.post('/storages/$id/mkdir?path=${Uri.encodeComponent(path)}', {});
    _checkOk(r);
  }

  /// 删除文件/目录（path 走 query，soft-fail 检查 ok）。
  Future<void> remove(int id, String path) async {
    final r = await _api.post('/storages/$id/delete?path=${Uri.encodeComponent(path)}', {});
    _checkOk(r);
  }

  /// 创建存储后端（对齐后端 POST /storages）。
  Future<FileStorage> create(FileStorageInput in_) async {
    final r = await _api.postJson('/storages', in_.toJson());
    return FileStorage.fromJson(r);
  }

  /// 更新存储后端（密码留空=保留，对齐后端 PUT /storages/:id）。
  Future<FileStorage> update(int id, FileStorageInput in_) async {
    final r = await _api.put('/storages/$id', in_.toJson());
    return FileStorage.fromJson(r);
  }

  /// 删除存储后端（DELETE /storages/:id）。
  Future<void> delete(int id) async => _api.delete('/storages/$id');

  void _checkOk(Map<String, dynamic> r) {
    if (r['ok'] != true) {
      throw Exception((r['error'] as String?) ?? '操作失败');
    }
  }
}

/// 创建/更新存储入参（对齐后端 oper.FileStorageInput）。密码留空=不修改/不设密码。
class FileStorageInput {
  final String name;
  final String type; // webdav | alist | smb | agent
  final String url;
  final String username;
  final String password;
  final String basePath;
  final int agentId; // type=agent 时关联 Agent；其他类型为 0
  FileStorageInput({
    required this.name,
    required this.type,
    this.url = '',
    this.username = '',
    this.password = '',
    this.basePath = '/',
    this.agentId = 0,
  });
  Map<String, dynamic> toJson() => {
        'name': name,
        'type': type,
        'url': url,
        'username': username,
        'password': password,
        'base_path': basePath,
        'agent_id': agentId,
      };
}
