// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/dashboard/dashboard_page.dart';

// 「保存图片 / 分享图片」两条出口从前会把失败咽掉：截图取不到像素时直接 return
// （弹窗不关、一句提示都没有），异常则把英文原文当用户文案。这里钉住文案判据。
void main() {
  test('截图取不到：给出可行动的原因，而不是静默', () {
    expect(shareExportFailure(null), contains('截图失败'));
    expect(shareExportFailure(null), contains('重新打开导出预览'));
  });

  test('本地写盘/分享异常：中文主句在前，细节附在后面', () {
    final t = shareExportFailure('PathNotFoundException: cannot open file');
    expect(t, startsWith('导出图片失败：'));
    expect(t, contains('PathNotFoundException'));
  });

  test('分享通道缺失：指回可用的保存动作', () {
    expect(shareUnsupported(), contains('保存图片'));
  });
}
