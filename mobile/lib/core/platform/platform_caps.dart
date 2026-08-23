// 平台能力抽象层：全项目平台差异的唯一收敛点（见 docs/鸿蒙端完善.md）。
//
// 鸿蒙（ohos）是 OpenHarmony fork Flutter 独有的 TargetPlatform 枚举成员，
// 官方 stable SDK 没有它：`case TargetPlatform.ohos` 在官方 SDK 下是编译期
// 错误（chewie 1.13.x 踩过的坑，pubspec 已锁回 1.12.1）。因此本文件判定
// 鸿蒙统一走运行时字符串判断 Platform.operatingSystem == 'ohos'
// （fork SDK 在 ohos 上返回该值；官方 SDK 的平台集合里永远不会出现，
// 全平台安全），避免任何对 fork 独有枚举的编译期引用。
//
// 历史教训：update_checker / apk_downloader 曾用 switch 处理平台，鸿蒙
// 落 default 分支被当作 Android——导致鸿蒙端拿到 APK 直链尝试应用内
// 安装，而 HarmonyOS NEXT 不兼容 APK，必然失败。平台分支必须收敛到本文件。
import 'dart:io';

import 'package:flutter/foundation.dart'
    show kIsWeb, defaultTargetPlatform, TargetPlatform;

/// 当前运行平台是否为鸿蒙（HarmonyOS / OpenHarmony）。
bool get isOhosPlatform => !kIsWeb && Platform.operatingSystem == 'ohos';

/// 更新清单 platform key，对应 update-manifest.json 的 targets 键
/// （backend 对未知 key 只是不回填下载信息，不会报错）。
/// 鸿蒙为 'ohos'：url 指向 HAP 下载页/直链均可。
String currentUpdatePlatform() {
  if (kIsWeb) return 'docker';
  if (isOhosPlatform) return 'ohos';
  switch (defaultTargetPlatform) {
    case TargetPlatform.windows:
      return 'windows';
    case TargetPlatform.iOS:
      return 'ios';
    case TargetPlatform.macOS:
      return 'macos';
    default:
      return 'android';
  }
}

/// 是否支持应用内 OTA（下载 → sha256 校验 → 拉起系统安装器），仅 Android。
/// 鸿蒙 HarmonyOS NEXT 无应用内安装 API，走外链浏览器下载
/// （与桌面/iOS 同策略）；open_filex 打开 .hap 需用户手动确认，
/// 暂不启用，待真机验证后再评估。
bool supportsInAppOta() =>
    !kIsWeb && !isOhosPlatform && defaultTargetPlatform == TargetPlatform.android;
