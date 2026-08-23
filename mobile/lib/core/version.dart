import 'package:package_info_plus/package_info_plus.dart';

/// 客户端版本号。
///
/// **客户端版本以本机安装包为准**（pubspec.yaml 的 `version:`，经 package_info_plus
/// 运行时读取），与所连后端无关——一个客户端可能连不同后端，app 版本是其固有属性。
/// 后端版本是另一回事（GET /system/info 的 version），二者各自独立更新，需分别显示。
///
/// 下面的 `appVersion` 常量仅作 package_info 取值失败/未就绪时的同步兜底，应随 pubspec
/// `version:` 同步维护；正常运行时 [getClientVersion] 读到的是真实安装版本。
const String appVersion = '1.03.17';

String? _cachedClientVersion;

/// 客户端真实版本（package_info_plus，形如 "1.0.0"；失败回退 appVersion 常量）。
/// 进程内缓存（安装版本运行期不变）。
Future<String> getClientVersion() async {
  final cached = _cachedClientVersion;
  if (cached != null) return cached;
  try {
    final info = await PackageInfo.fromPlatform();
    final v = info.version;
    _cachedClientVersion = v.isNotEmpty ? v : appVersion;
  } catch (_) {
    _cachedClientVersion = appVersion;
  }
  return _cachedClientVersion!;
}
