import 'dart:typed_data';
import 'package:http/http.dart' as http;
import '../core/api_client.dart';

/// 导入结果（PTPatronus 自身格式）。
/// 后端 /backup/import 一次给全四份账：导入数、跳过数、失败数、失败明细，以及
/// 「这份备份其实什么都没导进来」（empty）。从前这里只取前两个数，
/// 于是跳过/失败与空备份在手机上都说成「导入完成」（#256）。
class ImportResult {
  final int sites;
  final int downloaders;
  final int sitesSkipped;
  final int downloadersFailed;
  final List<String> failures;
  final bool empty;
  const ImportResult({
    required this.sites,
    required this.downloaders,
    this.sitesSkipped = 0,
    this.downloadersFailed = 0,
    this.failures = const [],
    this.empty = false,
  });

  static ImportResult fromJson(Map<String, dynamic> j) {
    final f = (j['failures'] as List?)
            ?.map((e) => e.toString())
            .toList(growable: false) ??
        const <String>[];
    return ImportResult(
      sites: (j['imported_sites'] as num?)?.toInt() ?? 0,
      downloaders: (j['imported_downloaders'] as num?)?.toInt() ?? 0,
      sitesSkipped: (j['sites_skipped'] as num?)?.toInt() ?? 0,
      downloadersFailed: (j['downloaders_failed'] as num?)?.toInt() ?? 0,
      failures: f,
      empty: j['empty'] == true,
    );
  }
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
    return ImportResult.fromJson(r);
  }

  /// 导入 MoviePilot 站点 JSON 数组（强制建为 nexusphp 启用站点）。
  /// 后端这条链同样给 `sites_skipped` / `empty`（缺 name、注册域重复都在跳过账里），
  /// 所以按 ImportResult 返回，别让调用方只能显示「导入了几个」。
  Future<ImportResult> importMoviePilot(List<dynamic> data) async {
    final r = await _api.postJson('/backup/import-moviepilot', data);
    return ImportResult.fromJson(r);
  }

  /// 导入 PT-Depiler (.ptd) 备份（multipart 文件上传；.ptd 是二进制 ZIP）。
  /// 加密备份 / 非 PTD 文件后端返回 4xx，ApiClient 转 ApiException（userMessage 友好中文）。
  /// 成功时也带回 `sites_skipped` / `empty`（无可用 URL、注册域重复都在跳过账里）。
  Future<ImportResult> importPTD(Uint8List bytes, String filename) async {
    final r = await _api.postMultipart(
      '/backup/import-ptd',
      files: [
        http.MultipartFile.fromBytes('file', bytes,
            filename: filename.isEmpty ? 'backup.ptd' : filename),
      ],
    );
    return ImportResult.fromJson(r);
  }
}
