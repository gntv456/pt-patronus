// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// #258 的接线证明：链层把「站点清单读不出来」回成错误、后端回 list_error 之后，
// 移动端如果 SiteApi 只取 results、页面只判 results.isEmpty，
// 用户在手机上看到的仍是「没有启用的站点」。
// 负控实测：删掉 api 层的 listError 或页面里那条分支，模型/纯函数用例都不会红
// ——所以这里补一条读真源码的护栏（同 dashboard_banner_wiring_test 的做法）。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  final api = _code('lib/api/site.dart');
  final page = _code('lib/features/sites/sites_checkin.dart');

  test('SiteApi 必须把 list_error 带回调用方（只取 results 就等于把读失败咽掉）', () {
    expect(api, contains("r['list_error']"));
    expect(api, contains('listError:'));
  });

  test('一键签到先判「清单读不出」，再判空清单（顺序反了就还是那句假话）', () {
    final branch = page.indexOf('res.listError.isNotEmpty');
    final empty = page.indexOf("'没有启用的站点'");
    expect(branch, greaterThan(-1), reason: '页面必须有读失败的分支');
    expect(empty, greaterThan(-1), reason: '空清单仍要有自己的说法（正控）');
    expect(branch, lessThan(empty), reason: '读失败必须先于「没有启用的站点」返回');
  });

  test('读失败的文案要说清是「清单读不出来」并带上后端原因', () {
    expect(page, contains('站点清单读不出来'));
    // 原因必须透传，不能压成一句「签到失败」（用户唯一的线索在后端那句里）。
    expect(page, contains(r"'站点清单读不出来：${res.listError}'"));
  });
}
