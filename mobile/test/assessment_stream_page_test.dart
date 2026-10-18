import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/assessment/assessment_page.dart';

// 考核流的 done 账面（#74/#190）必须在页面上分得清「读不到」与「没有考核」。
void main() {
  const err = '站点清单读取失败：3 站无响应';
  const oneCard = MapEntry<String, Map<String, dynamic>>(
    'batch',
    {
      'results': [
        {'site_id': 1, 'site_name': 'A站', 'status': 'HEALTHY'},
      ],
    },
  );

  Future<void> pump(
    WidgetTester t,
    List<MapEntry<String, Map<String, dynamic>>> evs,
  ) async {
    await t.pumpWidget(
      MaterialApp(home: AssessmentPage(frames: (path) => Stream.fromIterable(evs))),
    );
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  testWidgets('零站点卡 + done 带 list_error：说读不到，不说「该筛选下无站点」', (t) async {
    await pump(t, [const MapEntry('done', {'list_error': err})]);
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('该筛选下无站点'), findsNothing);
  });

  testWidgets('已有考核卡 + done 带 list_error：横幅说明，不挡卡片', (t) async {
    await pump(t, [oneCard, const MapEntry('done', {'list_error': err})]);
    expect(find.textContaining('A站'), findsWidgets);
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('该筛选下无站点'), findsNothing);
  });

  testWidgets('正控：账面干净就不编造告警', (t) async {
    await pump(t, [oneCard, const MapEntry('done', <String, dynamic>{})]);
    expect(find.textContaining('A站'), findsWidgets);
    expect(find.textContaining(err), findsNothing);
  });
}
