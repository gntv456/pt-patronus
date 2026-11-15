// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 平台能力判定测试：debugDefaultTargetPlatformOverride 逐平台钉全部 switch 分支。
// （flutter test 宿主的 defaultTargetPlatform 默认伪装 android，不能拿它断言
// 宿主平台；用 override 才能覆盖每个分支。）
// 核心：linux 分支回归门禁——historical bug 是 switch 无 linux case 落 default
// 返回 android，Linux 用户被引导下载 APK（错误产物）。
import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/platform/platform_caps.dart';

void main() {
  // 每个用例自管 override（tearDown 兜底复位，避免污染同文件其它用例）。
  tearDown(() => debugDefaultTargetPlatformOverride = null);

  void withPlatform(TargetPlatform p, void Function() body) {
    debugDefaultTargetPlatformOverride = p;
    try {
      body();
    } finally {
      debugDefaultTargetPlatformOverride = null;
    }
  }

  test('各平台 → 更新清单 platform key（对应 targets 键）', () {
    withPlatform(TargetPlatform.android,
        () => expect(currentUpdatePlatform(), 'android'));
    withPlatform(TargetPlatform.windows,
        () => expect(currentUpdatePlatform(), 'windows'));
    withPlatform(TargetPlatform.iOS, () => expect(currentUpdatePlatform(), 'ios'));
    withPlatform(
        TargetPlatform.macOS, () => expect(currentUpdatePlatform(), 'macos'));
    withPlatform(TargetPlatform.linux, () {
      // 回归门禁：修复前该分支落 default 返回 'android'。
      expect(currentUpdatePlatform(), 'linux');
    });
  });

  test('应用内 OTA 仅 Android（桌面/模拟 iOS 全否）', () {
    withPlatform(
        TargetPlatform.android, () => expect(supportsInAppOta(), isTrue));
    withPlatform(TargetPlatform.linux, () => expect(supportsInAppOta(), isFalse));
    withPlatform(TargetPlatform.windows,
        () => expect(supportsInAppOta(), isFalse));
    withPlatform(TargetPlatform.iOS, () => expect(supportsInAppOta(), isFalse));
    withPlatform(
        TargetPlatform.macOS, () => expect(supportsInAppOta(), isFalse));
  });
}
