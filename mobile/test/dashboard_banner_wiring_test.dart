// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// #255 的接线证明：判据与文案两层各自绿，不代表仪表盘页面真的把这条账面挂上了屏幕。
// 负控实测：把 `dashboard_page.dart` 里那行 `_noticeBanner(_statReadNotice)` 删掉，
// 模型层与纯函数层的用例一个字都不会红 —— 所以这里补一条读真源码的护栏。
// 只扫剥掉 // 注释之后的代码，免得我自己写的说明文字造出假红（同后端 Go 护栏的做法）。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  const page = 'lib/features/dashboard/dashboard_page.dart';
  final src = _code(page);

  test('仪表盘页面必须渲染每站统计的读取失败账面（接线不能断在最后一米）', () {
    expect(src, contains('_noticeBanner(_statReadNotice)'));
    expect(src, contains('statReadNotice(_overview?.statReadError)'));
  });

  test('顶部两条账面共用同一个渲染出口（样式与判据不许抄第二遍）', () {
    // 并发区失败 + 统计读取失败都走 _noticeBanner；_noticeBanner 只许有一条实现。
    expect(src.contains('extrasNotice(_extrasFailed)'), isTrue);
    expect(RegExp(r'Widget _noticeBanner\(').allMatches(src).length, 1);
    // 除了这一个实现，别处不该再自己拼一份橙色告警条（那意味着两套样式在漂移）。
    expect(RegExp(r'warning_amber_rounded').allMatches(src).length, 1);
  });
}
