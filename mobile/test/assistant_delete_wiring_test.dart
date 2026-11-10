// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// #268 移动端 AI 助手「删除会话」的接线证明。
// 后端 DELETE /agent/conversations/:id 有 requireExists + AgentConversationNotFound，
// 所以「会话已被别人删掉」是真会发生的 404 且带中文原因；
// 旧写法 `await _api.deleteConversation(id);` 没有兜底 ⇒ 异常逃出 async 的 onPressed
// 进未处理 Future：弹窗已关、列表没变、也不吭声 = 「删除点了没反应」。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  final page = _code('lib/features/assistant/assistant_page.dart');

  test('删除调用必须在 try 内，失败要把后端原因说出来', () {
    final call = page.indexOf('await _api.deleteConversation(id);');
    expect(call, greaterThan(-1), reason: '删除调用必须在（否则下面几条判据是空的）');
    final before = page.substring(page.lastIndexOf('Future<void> _delete', call), call);
    expect(before, contains('try {'));
    final after = page.substring(call, call + 260);
    final catchIdx = after.indexOf('} catch (');
    expect(catchIdx, greaterThan(-1));
    final body = after.substring(catchIdx);
    expect(body, contains('删除会话失败'));
    expect(body, contains(r'$e'), reason: '原因要带出来：404 那句中文是用户唯一的线索');
  });

  test('删除失败不许继续动本地状态（切新会话 + 重拉清单会把「没删掉」演成「已删掉」）', () {
    final catchIdx = page.indexOf('_toast(\'删除会话失败');
    expect(catchIdx, greaterThan(-1));
    final between = page.substring(catchIdx, catchIdx + 200);
    final ret = between.indexOf('return;');
    expect(ret, greaterThan(-1), reason: 'catch 里必须 return');
    final newChat = page.indexOf('_newChat();', catchIdx);
    expect(newChat, greaterThan(catchIdx + ret), reason: '本地收尾只能排在 return 之后');
  });
}
