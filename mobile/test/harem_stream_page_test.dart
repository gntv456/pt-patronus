// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/harem/harem_page.dart';

// 后宫流的 done 账面（#188）：读不到站点清单时不能画成「无数据」，
// 那是「这一轮没抓到成员」和「压根没读到」之间唯一的分界。
void main() {
  const err = '站点清单读取失败：savept.icu 无响应';

  Future<void> pump(
    WidgetTester t,
    List<MapEntry<String, Map<String, dynamic>>> evs,
  ) async {
    await t.pumpWidget(
      MaterialApp(home: HaremPage(frames: (path) => Stream.fromIterable(evs))),
    );
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  testWidgets('done 带 list_error 且零成员：说读不到，不说「无数据」', (t) async {
    await pump(t, [const MapEntry('done', {'list_error': err})]);
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('无数据'), findsNothing);
  });

  testWidgets('正控：账面干净的空结果仍说「无数据」，不编造告警', (t) async {
    await pump(t, [const MapEntry('done', <String, dynamic>{})]);
    expect(find.textContaining('无数据'), findsWidgets);
    expect(find.textContaining(err), findsNothing);
  });
}
