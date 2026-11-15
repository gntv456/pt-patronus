// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 产物 url 归一测试（node 无关，flutter test）。
// 更新机制改造后清单 url 支持服务器相对路径，客户端统一在此绝对化。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/update_url.dart';

void main() {
  test('相对路径拼 baseUrl（产物托管直链口径）', () {
    expect(
      resolveArtifactUrl('/api/v1/system/downloads/android-1.03.31.apk',
          'http://192.168.1.10:8088'),
      'http://192.168.1.10:8088/api/v1/system/downloads/android-1.03.31.apk',
    );
  });

  test('base 带尾斜杠不产生双斜杠', () {
    expect(
      resolveArtifactUrl('/api/v1/system/downloads/ohos-1.03.31.hap',
          'https://pt.example.com/'),
      'https://pt.example.com/api/v1/system/downloads/ohos-1.03.31.hap',
    );
  });

  test('绝对 url 原样透传（网盘/CDN 外链兼容）', () {
    expect(resolveArtifactUrl('https://pan.example.com/a.apk', 'http://x'),
        'https://pan.example.com/a.apk');
    expect(resolveArtifactUrl('http://pan.example.com/a.apk', 'http://x'),
        'http://pan.example.com/a.apk');
  });

  test('无 base（未配置服务器）：相对 url 原样返回不崩', () {
    expect(resolveArtifactUrl('/api/v1/system/downloads/a.apk', ''),
        '/api/v1/system/downloads/a.apk');
  });

  test('空 url 恒空；无前导斜杠的相对值补斜杠', () {
    expect(resolveArtifactUrl('', 'http://x'), '');
    expect(resolveArtifactUrl('api/v1/x', 'http://x'), 'http://x/api/v1/x');
  });
}
