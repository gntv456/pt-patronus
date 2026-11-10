// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/recommend/recommend_page.dart';

// 推荐流的 done 帧带 sources/batches/failed（#123/#192），
// 这里证的是它有没有一路走到页面态：少跑一个源时，界面既不能说成「今天没推荐」，
// 也不能在已经出片的情况下彻底没声。
void main() {
  const partial = MapEntry<String, Map<String, dynamic>>(
    'done',
    {'sources': 2, 'batches': 1, 'failed': ['douban']},
  );
  const allOk = MapEntry<String, Map<String, dynamic>>(
    'done',
    {'sources': 2, 'batches': 2},
  );
  const oneItem = MapEntry<String, Map<String, dynamic>>(
    'batch',
    {
      'results': [
        {'title': '测试影片', 'type': 'movie', 'year': 2026},
      ],
    },
  );

  Future<void> pump(
    WidgetTester t,
    List<MapEntry<String, Map<String, dynamic>>> evs,
  ) async {
    await t.pumpWidget(
      MaterialApp(home: RecommendPage(frames: (path) => Stream.fromIterable(evs))),
    );
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  testWidgets('一个源没跑成 + 零条目：说「源跑不全」，不说「暂无推荐」', (t) async {
    await pump(t, [partial]);
    expect(find.textContaining('douban'), findsWidgets);
    expect(find.textContaining('暂无推荐'), findsNothing);
  });

  testWidgets('已出片 + 一个源没跑成：横幅常驻说明，不盖住条目', (t) async {
    await pump(t, [oneItem, partial]);
    expect(find.textContaining('测试影片'), findsWidgets);
    expect(find.textContaining('douban'), findsWidgets);
    expect(find.textContaining('暂无推荐'), findsNothing);
  });

  testWidgets('正控：源都跑成了，就不编造告警', (t) async {
    await pump(t, [oneItem, allOk]);
    expect(find.textContaining('测试影片'), findsWidgets);
    expect(find.textContaining('douban'), findsNothing);
  });
}
