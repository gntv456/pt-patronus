import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart' show kIsWeb, defaultTargetPlatform, TargetPlatform;
import 'package:open_filex/open_filex.dart';
import 'package:path_provider/path_provider.dart';

/// 是否支持应用内 OTA（仅 Android 原生，非 Web）。
/// 桌面 / iOS / Web 走外链兜底（无可靠的应用内安装机制）。
bool supportsInAppOta() =>
    !kIsWeb && defaultTargetPlatform == TargetPlatform.android;

/// 下载 APK 到临时目录，校验 sha256（expectedSha256 非空时），返回本地路径。
/// onProgress(received, total) 用于进度展示，total<=0 表示未知大小。
/// 失败抛异常；cancelToken 可取消。下载前清理上次残留 APK（同 pt_mate clearDownloadedApks）。
Future<String> downloadApk({
  required String url,
  String? expectedSha256,
  required void Function(int received, int total) onProgress,
  CancelToken? cancelToken,
}) async {
  final dir = await getTemporaryDirectory();
  _cleanOldApks(dir);
  final savePath = '${dir.path}/ptpatronus-update.apk';
  final dio = Dio(BaseOptions(
    connectTimeout: const Duration(seconds: 15),
    receiveTimeout: const Duration(minutes: 30),
    followRedirects: true,
  ));
  try {
    await dio.download(
      url,
      savePath,
      onReceiveProgress: onProgress,
      cancelToken: cancelToken,
    );
  } finally {
    dio.close(force: true);
  }
  // 完整性校验（pt_mate 缺的短板：补 sha256，防中间人替换 APK）
  if (expectedSha256 != null && expectedSha256.isNotEmpty) {
    final digest = sha256.convert(await File(savePath).readAsBytes());
    if (digest.toString().toUpperCase() != expectedSha256.toUpperCase()) {
      final f = File(savePath);
      if (await f.exists()) await f.delete();
      throw StateError('完整性校验失败：APK 已损坏或被篡改（sha256 不匹配）');
    }
  }
  return savePath;
}

/// 调系统安装器打开 APK。Android 8+ 首次会自动引导「允许此来源安装应用」。
Future<void> installApk(String path) async {
  final res = await OpenFilex.open(path);
  // OpenResult.type 为 done 表示成功拉起安装器；其它（noAppToOpen 等）视为失败
  if (res.type.name != 'done') {
    throw StateError('无法打开安装界面：${res.message}');
  }
}

void _cleanOldApks(Directory dir) {
  try {
    for (final e in dir.listSync()) {
      if (e is File && e.path.endsWith('.apk')) {
        e.deleteSync();
      }
    }
  } catch (_) {
    /* 忽略清理失败 */
  }
}
