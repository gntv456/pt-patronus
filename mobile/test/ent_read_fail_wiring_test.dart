import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/entertainment/ent_read_fail.dart';

// #279 移动端娱乐面板「读取失败」口径。
//
// 从前这 14 个视图把每个读请求都写成 `catch (_) {}`，另有 7 处写成
// `.catchError((_) => GameStats(balance: 0, …))`：站点凭据解不开（#277 刚修的那条）、
// 被墙、超时，在手机上和「这个号真没数据」一模一样——魔力值「—」、0 次、空列表。
// 后端这次已经把「该换游戏 / 去重新保存凭据 / 花钱失败先核对余额」写进了信封，
// 丢掉就等于什么都没告诉用户。

String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

const _credReason = '站点「幻月府」的 Cookie 无法解密（主密钥可能已更换）——请到「站点」里重新保存该站点凭据';

Widget _host(List<(String, Object)> pending) {
  return MaterialApp(
    home: Scaffold(
      body: Column(
        children: [
          for (final p in pending)
            Builder(
              builder: (context) => TextButton(
                onPressed: () => entReadFailed(context, p.$1, p.$2),
                child: Text('fire-${p.$1}'),
              ),
            ),
        ],
      ),
    ),
  );
}

void main() {
  setUp(resetEntReadFails);
  tearDown(resetEntReadFails);

  testWidgets('读取失败要出声：提示里同时有读数名和后端那句可照做的原因', (tester) async {
    await tester.pumpWidget(_host([
      ('farm', ApiException(503, 'entertainment_credential_unreadable', userMessage: _credReason)),
    ]));
    await tester.tap(find.text('fire-farm'));
    expect(find.byType(SnackBar), findsNothing); // 还在合并窗口内
    await tester.pump(const Duration(milliseconds: 1600));
    final text = tester.widget<SnackBar>(find.byType(SnackBar)).content;
    final label = (text as Text).data!;
    expect(label, contains('farm'));
    expect(label, contains('重新保存'), reason: '接口层给的人话必须抵达界面，否则 503 白修');
    expect(label, contains('不代表站点真实结果'));
  });

  testWidgets('一次全站故障只汇出一条提示，而不是堆 3 个 toast', (tester) async {
    await tester.pumpWidget(_host([
      ('prizes', ApiException(502, 'entertainment_data_unreadable', userMessage: '站点娱乐数据读不出来')),
      ('stats', ApiException(502, 'entertainment_data_unreadable', userMessage: '站点娱乐数据读不出来')),
      ('history', ApiException(404, 'site_not_found', userMessage: '站点不存在')),
    ]));
    for (final l in ['prizes', 'stats', 'history']) {
      await tester.tap(find.text('fire-$l'));
    }
    await tester.pump(const Duration(milliseconds: 1600));
    expect(find.byType(SnackBar), findsOneWidget);
    final label = (tester.widget<SnackBar>(find.byType(SnackBar)).content as Text).data!;
    for (final l in ['prizes', 'stats', 'history']) {
      expect(label, contains(l));
    }
  });

  testWidgets('正控：没有失败就一条提示都不发', (tester) async {
    await tester.pumpWidget(_host(const []));
    await tester.pump(const Duration(seconds: 5));
    expect(find.byType(SnackBar), findsNothing);
  });

  test('空清单回空串（调用方据此不出声）', () {
    expect(entReadNoticeText(const {}), '');
    expect(entReadNoticeText(const {'farm': ''}),
        '游戏数据未能读取：farm（显示的是空值，不代表站点真实结果）');
    expect(entReadNoticeText(const {'farm': '站点凭据解不开'}), contains('farm（站点凭据解不开）'));
    expect(entReadNoticeText({'a': 'x', 'b': 'y', 'c': 'z', 'd': 'w', 'e': 'v'}), contains('等 5 项'));
  });

  // 接线证明：helper 存在不等于有人调用（10-14 教训：解构出来却没人调，单测照样绿）。
  group('娱乐视图接线', () {
    final dir = Directory('lib/features/entertainment');
    final views = dir
        .listSync()
        .whereType<File>()
        .where((f) => f.path.endsWith('.dart') && !f.path.endsWith('ent_read_fail.dart'))
        .toList();

    test('静默兜底清零：只剩两处 WS 自愈面（都在 panda_flip_view）', () {
      var total = 0;
      final elsewhere = <String>[];
      for (final f in views) {
        final n = RegExp(r'} catch \(_\) \{\}').allMatches(_code(f.path)).length;
        if (n == 0) continue;
        total += n;
        if (!f.path.endsWith('panda_flip_view.dart')) elsewhere.add('${f.path}:$n');
      }
      expect(total, 2, reason: '28 处读取兜底里只许保留 panda_flip_view 的两处 WS（3s 自动重连）');
      expect(elsewhere, isEmpty, reason: '非 WS 的静默兜底又回来了：$elsewhere');
    });

    test('.catchError((_) => 造出来的假读数全部记下原因', () {
      var left = 0;
      for (final f in views) {
        left += RegExp(r'\.catchError\(\(_\) =>').allMatches(_code(f.path)).length;
      }
      expect(left, 0, reason: '把失败折成 balance:0 / 空列表 而不留痕 = 假读数');
    });

    test('每个用过兜底的视图都必须真的调用 entReadFailed', () {
      for (final f in views) {
        final code = _code(f.path);
        final calls = RegExp(r'entReadFailed\(context, ').allMatches(code).length;
        if (calls == 0) continue;
        expect(code, contains('if (mounted) entReadFailed'), reason: '${f.path} 用 context 没做 mounted 守卫');
      }
      final total = views.fold<int>(
          0, (sum, f) => sum + RegExp(r'entReadFailed\(context, ').allMatches(_code(f.path)).length);
      expect(total, greaterThanOrEqualTo(33), reason: '26 处外层 catch + 7 处内层 catchError');
    });
  });
}
