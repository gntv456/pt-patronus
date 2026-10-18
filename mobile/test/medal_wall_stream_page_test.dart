import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/common/stream_account.dart';
import 'package:ptpatronus/features/medal_wall/medal_wall_page.dart';

// done 帧的 list_error 要一路走到页面态（_error / _streamBanner）才算真的接上。
// #189/#193 当时只证到 mapStreamFrame 这层纯函数，这里补上渲染层：
// 「读不到」既不能被画成「暂无勋章数据」，也不能在已有数据时彻底没声。
void main() {
  StreamFrames framesWith(List<MapEntry<String, Map<String, dynamic>>> evs) {
    return (path) => Stream.fromIterable(evs);
  }

  Future<void> pump(WidgetTester t, StreamFrames frames) async {
    await t.pumpWidget(MaterialApp(home: MedalWallPage(frames: frames)));
    await t.pump();
    await t.pump(const Duration(milliseconds: 50));
  }

  const err = '站点清单读取失败：savept.icu 无响应';

  testWidgets('一张卡都没到 + done 带 list_error：说读不到，不说「暂无勋章数据」', (t) async {
    await pump(t, framesWith([const MapEntry('done', {'list_error': err})]));
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('暂无勋章数据'), findsNothing);
  });

  testWidgets('已有数据 + done 带 list_error：顶部横幅要说可能不全，但不盖住卡片', (t) async {
    await pump(
      t,
      framesWith([
        const MapEntry(
          'batch',
          {
            'results': [
              {'site_id': 1, 'site_name': 'A', 'medal_id': 'm1', 'name': '测试勋章'},
            ],
          },
        ),
        const MapEntry('done', {'list_error': err}),
      ]),
    );
    expect(find.textContaining('测试勋章'), findsWidgets);
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('暂无勋章数据'), findsNothing);
  });

  testWidgets('正控：账面干净时既无横幅也无错误态', (t) async {
    await pump(
      t,
      framesWith([
        const MapEntry(
          'batch',
          {
            'results': [
              {'site_id': 1, 'site_name': 'A', 'medal_id': 'm1', 'name': '测试勋章'},
            ],
          },
        ),
        const MapEntry('done', <String, dynamic>{}),
      ]),
    );
    expect(find.textContaining('测试勋章'), findsWidgets);
    expect(find.textContaining(err), findsNothing);
  });

  testWidgets('空清单且后端确实报「零站点」：仍要说读不到（不能画成没有勋章）', (t) async {
    await pump(
      t,
      framesWith([
        const MapEntry('batch', {'results': <Map<String, dynamic>>[]}),
        const MapEntry('done', {'list_error': err}),
      ]),
    );
    expect(find.textContaining(err), findsWidgets);
    expect(find.textContaining('暂无勋章数据'), findsNothing);
  });
}
