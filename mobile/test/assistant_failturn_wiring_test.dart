import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

// #266 移动端的接线证明：AI 助手这一轮失败后，会话里必须看得见原因，
// 实时气泡也必须收场。这两件事都只发生在 mixin/页面的分支里，
// 纯函数用例碰不到（ModelAgent 的流解析本来就绿），所以按仓内惯例补读真源码的护栏。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  final ops = _code('lib/features/assistant/assistantopsmixin.dart');
  final view = _code('lib/features/assistant/assistant_widgets.dart');

  test('失败原因必须落进会话记录（只弹 2 秒 SnackBar 等于没留痕）', () {
    expect(ops, contains(r"'⚠ $liveError'"));
    expect(ops, contains("AgentMessage(id: -2, role: 'assistant'"));
    // 用户主动「停止」和真失败要分开写：停止不该按「发送失败」出声。
    expect(ops, contains('本轮已停止'));
  });

  test('实时气泡收场必须写在 finally 里（写在 try 尾部＝抛错路径永远卡在生成中）', () {
    final fi = ops.lastIndexOf('} finally {');
    expect(fi, greaterThan(-1), reason: '_send 必须有 finally');
    expect(ops.substring(fi, fi + 120), contains('_hasLive = false'));
  });

  test('会话记录读取失败必须留账面，不许 catch (_) 吞掉', () {
    final read = ops.indexOf('_messages = await _api.messages(id)');
    expect(read, greaterThan(-1), reason: '取数口必须在（否则下面这条判据是空的）');
    final after = ops.substring(read);
    final catchIdx = after.indexOf('} catch (');
    expect(catchIdx, greaterThan(-1));
    expect(after.substring(catchIdx, catchIdx + 200), contains('_messagesNotice ='));
    // 读失败要有可点的重探入口，不能只留一句话。
    expect(view, contains('_messagesNotice'));
    expect(view, contains('onRetry'));
  });

  test('正控：错误帧仍然当场出声 + 停止走带标记的 _stop', () {
    expect(ops, contains('_toast(liveError)'));
    expect(view, contains('onPressed: _stop'));
    expect(view, isNot(contains('onPressed: _api.cancel')));
  });
}
