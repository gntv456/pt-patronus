// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 流式账面该落在哪里：一条数据都没到 → 整页错误态（带重试）；已有数据 → 顶部常驻横幅。
// done 帧的清单/源账面由四个流式页（后宫/勋章墙/考核/推荐）共用这一套判据与横幅。
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/common/stream_account.dart';

void main() {
  group('streamNoticeOf', () {
    test('没有账面：既不报错也不出横幅', () {
      final n = streamNoticeOf(arrived: 5, listError: '');
      expect(n.error, isNull);
      expect(n.banner, isNull);
    });

    test('一条都没到：走整页错误态，不叠横幅', () {
      final n = streamNoticeOf(arrived: 0, listError: '站点清单读取失败');
      expect(n.error, '站点清单读取失败');
      expect(n.banner, isNull);
    });

    test('已有数据：降级成横幅，残缺结果必须说出来', () {
      final n = streamNoticeOf(arrived: 3, listError: '只跑了 2/3 个推荐源');
      expect(n.banner, '只跑了 2/3 个推荐源');
      expect(n.error, isNull);
    });

    test('判据只此一份：arrived 为 0 与有数据必须走不同出口', () {
      const err = '清单读不到';
      expect(streamNoticeOf(arrived: 0, listError: err).error, err);
      expect(streamNoticeOf(arrived: 1, listError: err).error, isNull);
      expect(streamNoticeOf(arrived: 1, listError: err).banner, err);
      expect(streamNoticeOf(arrived: 0, listError: err).banner, isNull);
    });
  });

  group('StreamNoticeBanner 渲染层', () {
    Future<void> mount(WidgetTester tester, String? text) async {
      await tester.pumpWidget(
        MaterialApp(home: Scaffold(body: StreamNoticeBanner(text: text))),
      );
    }

    testWidgets('有账面文案时真的画在页面上', (tester) async {
      await mount(tester, '站点清单读取失败');
      expect(find.text('站点清单读取失败'), findsOneWidget);
      expect(find.byType(Row), findsOneWidget);
    });

    testWidgets('正控：文案为空时不画字也不占位', (tester) async {
      await mount(tester, null);
      expect(find.byType(Row), findsNothing);
      await mount(tester, '');
      expect(find.byType(Row), findsNothing);
    });
  });
}
