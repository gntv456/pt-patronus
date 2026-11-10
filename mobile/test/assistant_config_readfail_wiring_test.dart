// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// #267 移动端 AI 助手设置面板的接线证明。
// 后端把空 enabled_tools 解释成「全开」（aiagent/tools.go: toolAllowed 空表 ⇒ true），
// 所以「工具目录读不出来 ⇒ 当成一个工具都没有」不是显示问题，而是权限问题：
// 用户关掉「启用全部工具」想收窄权限，保存下来的却是全开。
// 面板要真跑到 showModalBottomSheet 才碰得到，这里按仓内惯例补读真源码的护栏。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  final view = _code('lib/features/assistant/assistant_widgets.dart');

  test('工具目录读失败必须单独记账，不许压成空清单', () {
    final read = view.indexOf('tools = await _api.tools()');
    expect(read, greaterThan(-1), reason: '取数口必须在（否则下面这条判据是空的）');
    final after = view.substring(read);
    expect(after, contains('toolsError ='));
    // 空清单本身仍要能显示（真的没有工具 ≠ 读不出来，但两者不能同形）。
    expect(after.indexOf('tools = [];'), lessThan(after.indexOf('toolsError =')), reason: '先兜空清单，再记原因');
  });

  test('目录没读出来时「启用全部工具」开关必须锁死，且要说清空清单=全开', () {
    expect(view, contains('onChanged: toolsError.isEmpty'));
    expect(view, contains(': null,'), reason: '读失败分支要给 onChanged 交 null（SwitchListTile 的禁用态）');
    expect(view, contains('工具目录读不出来'));
    expect(view, contains('全部允许'));
  });

  test('正控：目录读得到时开关照常可改', () {
    final toggle = view.indexOf('onChanged: toolsError.isEmpty');
    expect(toggle, greaterThan(-1));
    expect(view.substring(toggle, toggle + 260), contains('allowAll = v'));
  });

  test('配置本体读不出来时不许带着默认值打开面板（整份 PUT 会把真配置覆盖成默认）', () {
    final cfg = view.indexOf('cfg = await _api.getConfig()');
    expect(cfg, greaterThan(-1));
    // 只看这一句 catch 的体内（到 return; 为止）：窗口跨到隔壁的 tools catch 会假绿。
    final after = view.substring(cfg, cfg + 300);
    final end = after.indexOf('return;');
    expect(end, greaterThan(-1), reason: '读失败必须直接返回，不进面板');
    final body = after.substring(0, end);
    expect(body, contains('_toast('));
    expect(body, contains(r'$e'), reason: '原因要带在本句提示里，不能只剩「配置读不出来」');
  });
}
