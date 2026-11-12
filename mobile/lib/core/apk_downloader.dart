// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';
import 'package:crypto/crypto.dart';
import 'package:dio/dio.dart';
import 'package:open_filex/open_filex.dart';
import 'package:path_provider/path_provider.dart';

// supportsInAppOta() 已收敛至 platform_caps.dart，此处 re-export 保持
// 老调用点（update_checker.dart）import 不变。
// 鸿蒙端返回 false：HarmonyOS NEXT 无应用内安装 API，走外链浏览器下载。
export 'package:ptpatronus/core/platform/platform_caps.dart'
    show supportsInAppOta;

/// 下载 APK 到临时目录，校验 sha256（expectedSha256 非空时），返回本地路径。
/// onProgress(received, total) 用于进度展示，total<=0 表示未知大小。
/// 失败抛异常；cancelToken 可取消。下载前清理上次残留 APK。
Future<String> downloadApk({
  required String url,
  String? expectedSha256,
  required void Function(int received, int total) onProgress,
  CancelToken? cancelToken,
}) async {
  // 仅允许 https 源：防中间人替换 APK（与 sha256 校验互为双保险）。
  final parsed = Uri.tryParse(url);
  if (parsed == null || parsed.scheme != 'https') {
    throw ArgumentError('仅允许 https 下载源');
  }
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
  // 完整性校验：sha256，防中间人替换 APK
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
