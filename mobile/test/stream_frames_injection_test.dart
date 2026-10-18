// 四条流式页共用的帧源注入缝：证明 api 层「帧 → 条目 + 账面」真的接在 stream 上。
// 没有这个缝时，把 `ev.key == 'done'` 改错或整帧丢弃都不会让任何用例变红（只有编译层背书）。
import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/assessment.dart';
import 'package:ptpatronus/api/harem.dart';
import 'package:ptpatronus/api/medal_wall.dart';
import 'package:ptpatronus/api/recommend.dart';

typedef Frame = MapEntry<String, Map<String, dynamic>>;

/// 记下被请求路径的假帧源。
class FakeFrames {
  final List<Frame> frames;
  String? requestedPath;
  FakeFrames(this.frames);

  Stream<Frame> call(String path) {
    requestedPath = path;
    return Stream.fromIterable(frames);
  }
}

void main() {
  test('勋章墙：batch 出卡片、done 出清单账面，且打的是流式端点', () async {
    final src = FakeFrames([
      Frame('batch', {
        'results': [<String, dynamic>{}, <String, dynamic>{}],
      }),
      const Frame('done', {'list_error': '站点清单读取失败'}),
    ]);
    final events = await MedalWallApi(frames: src.call)
        .allStream(refresh: true)
        .toList();
    expect(src.requestedPath, '/medalwall/stream?refresh=1');
    expect(events.first.cards.length, 2);
    expect(events.first.listError, isEmpty);
    expect(events.last.listError, '站点清单读取失败');
    expect(events.last.cards, isEmpty);
  });

  test('考核：batch 计数与 done 账面各自落到 StreamBatch 的两个槽位', () async {
    final src = FakeFrames([
      Frame('batch', {
        'results': [<String, dynamic>{}, <String, dynamic>{}],
      }),
      const Frame('done', {'list_error': '站点清单读不到'}),
    ]);
    final batches = await AssessmentApi(frames: src.call).allStream().toList();
    expect(src.requestedPath, '/assessment/stream');
    expect(batches.first.items.length, 2);
    expect(batches.first.listError, isEmpty);
    expect(batches.last.listError, '站点清单读不到');
  });

  test('后宫：done 帧的 overview 与清单账面互不挤掉', () async {
    final src = FakeFrames([
      Frame('batch', {
        'results': [<String, dynamic>{}],
      }),
      Frame('done', {
        'overview': <String, dynamic>{},
        'list_error': '部分站点没跑成',
      }),
    ]);
    final events = await HaremApi(frames: src.call).allStream().toList();
    expect(src.requestedPath, '/harem/stream');
    expect(events.first.items.length, 1);
    expect(events.last.overview, isNotNull);
    expect(events.last.listError, '部分站点没跑成');
  });

  test('推荐：done 帧的源账面被换成一句人话，batch 条目不丢', () async {
    final src = FakeFrames([
      Frame('batch', {
        'results': [<String, dynamic>{}],
      }),
      const Frame('done', {'sources': 2, 'batches': 1, 'failed': ['douban']}),
    ]);
    final batches = await RecommendApi(frames: src.call)
        .feedStream(category: 'movie', page: 1)
        .toList();
    expect(src.requestedPath, contains('/media/recommend/stream'));
    expect(batches.first.items.length, 1);
    expect(batches.last.listError, contains('1/2'));
    expect(batches.last.listError, contains('douban'));
  });
}
