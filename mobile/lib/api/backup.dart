import 'dart:typed_data';
import 'package:http/http.dart' as http;
import '../core/api_client.dart';

/// 导入结果（PTPatronus 自身格式）。
class ImportResult {
  final int sites;
  final int downloaders;
  const ImportResult({required this.sites, required this.downloaders});
}

/// 备份 API：导出/导入（自身格式）/ 导入 MoviePilot 站点。对齐后端 /backup/*。
/// 注意：导入是 raw JSON body（后端 ShouldBindJSON(&RawMessage)），不是 multipart。
class BackupApi {
  final _api = ApiClient();

  /// 导出全部站点与下载器（密钥明文）为 JSON 字节流。
  Future<Uint8List> export() => _api.getBytes('/backup/export');

  /// 导入 PTPatronus 备份 JSON（整个对象作 body）。重复项后端按 name 跳过。
  Future<ImportResult> importOwn(Map<String, dynamic> data) async {
    final r = await _api.postJson('/backup/import', data);
    return ImportResult(
      sites: r['imported_sites'] as int? ?? 0,
      downloaders: r['imported_downloaders'] as int? ?? 0,
    );
  }

  /// 导入 MoviePilot 站点 JSON 数组（强制建为 nexusphp 启用站点）。
  Future<int> importMoviePilot(List<dynamic> data) async {
    final r = await _api.postJson('/backup/import-moviepilot', data);
    return r['imported_sites'] as int? ?? 0;
  }

  /// 导入 PT-Depiler (.ptd) 备份（multipart 文件上传；.ptd 是二进制 ZIP）。
  /// 加密备份 / 非 PTD 文件后端返回 4xx，ApiClient 转 ApiException（userMessage 友好中文）。
  Future<int> importPTD(Uint8List bytes, String filename) async {
    final r = await _api.postMultipart(
      '/backup/import-ptd',
      files: [
        http.MultipartFile.fromBytes('file', bytes,
            filename: filename.isEmpty ? 'backup.ptd' : filename),
      ],
    );
    return r['imported_sites'] as int? ?? 0;
  }
}
