// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 流式 done 帧的清单账面（后宫/勋章墙共用同一判据）：`list_error` 一丢，
// 「站点清单读失败」就会被画成「没有药单 / 没有勋章」。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/common/stream_account.dart';

void main() {
  test('done 帧带 list_error：原样交出去', () {
    expect(streamListError({'list_error': '站点清单读取失败'}), '站点清单读取失败');
  });

  test('正控：没有 list_error 时是空串（不是 null，也不冒充「0 条」）', () {
    expect(streamListError({'overview': <String, dynamic>{}}), isEmpty);
    expect(streamListError({'list_error': null}), isEmpty);
    expect(streamListError({'results': <Object>[]}), isEmpty);
  });

  test('后端留空串时也不出声', () {
    expect(streamListError({'list_error': ''}), isEmpty);
  });

  test('StreamBatch 默认既没有条目也没有账面', () {
    const batch = StreamBatch<int>();
    expect(batch.items, isEmpty);
    expect(batch.listError, isEmpty);
  });

  test('正控：条目与账面互不遮蔽（有卡片时账面仍可另存）', () {
    const batch = StreamBatch<int>(items: [1, 2], listError: 'x');
    expect(batch.items, <int>[1, 2]);
    expect(batch.listError, 'x');
  });

  group('mapStreamFrame（三条流共用的帧映射）', () {
    int parsed = 0;
    int parse(Map<String, dynamic> j) {
      parsed++;
      return j['n'] as int;
    }

    test('batch 帧：逐条交解析器，账面留空', () {
      parsed = 0;
      final batch = mapStreamFrame(
        MapEntry('batch', {
          'results': [
            {'n': 1},
            {'n': 2},
          ]
        }),
        parse,
      );
      expect(batch.items, <int>[1, 2]);
      expect(parsed, 2);
      expect(batch.listError, isEmpty);
    });

    test('done 帧：只带清单账面，不吐条目', () {
      final batch = mapStreamFrame(
        MapEntry('done', {
          'overview': <String, dynamic>{},
          'list_error': '站点清单读取失败',
        }),
        parse,
      );
      expect(batch.items, isEmpty);
      expect(batch.listError, '站点清单读取失败');
    });

    test('正控：done 帧没报错时账面是空串', () {
      final batch = mapStreamFrame(MapEntry('done', <String, dynamic>{}), parse);
      expect(batch.listError, isEmpty);
    });

    test('batch 帧 results 不是数组时按空批过去，不抛', () {
      final batch = mapStreamFrame(MapEntry('batch', {'results': 'oops'}), parse);
      expect(batch.items, isEmpty);
      expect(batch.listError, isEmpty);
    });

      test('其它帧名既不产条目也不报账面', () {
      final batch = mapStreamFrame(
        MapEntry('heartbeat', {
          'results': [
            {'n': 9}
          ],
          'list_error': '不该被读',
        }),
        parse,
      );
      expect(batch.items, isEmpty);
      expect(batch.listError, isEmpty);
    });
  });

  group('recommendStreamNotice（推荐流源账面）', () {
    test('少跑一个源：说出比例与失败源', () {
      final s = recommendStreamNotice({
        'sources': 3,
        'batches': 2,
        'failed': ['douban'],
      });
      expect(s, contains('2/3'));
      expect(s, contains('douban'));
    });

    test('源数未知但有失败源：仍然说出来，不冒充全绿', () {
      final s = recommendStreamNotice({'batches': 1, 'failed': ['tmdb']});
      expect(s, contains('tmdb'));
    });

    test('正控：源数与批次一致且无失败，不出声', () {
      expect(
        recommendStreamNotice({'sources': 3, 'batches': 3, 'failed': []}),
        isEmpty,
      );
      expect(recommendStreamNotice(<String, dynamic>{}), isEmpty);
    });
  });
}
