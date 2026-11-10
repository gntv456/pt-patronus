// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 设备趋势图的两个纯函数：取数窗口文案 + 按断口分段。
// 断口不补 0（否则掉线时段画成「CPU 跌到 0%」的假谷），截断必须说出口（否则「最近 60 次」被读成全量历史）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/monitor/trend_window.dart';

void main() {
  group('trendRuns', () {
    test('中间断口切成两段，且保留原始下标', () {
      final runs = trendRuns([40, 50, null, 60, 70]);
      expect(runs.length, 2);
      expect(runs[0].start, 0);
      expect(runs[0].values, [40, 50]);
      expect(runs[1].start, 3);
      expect(runs[1].values, [60, 70]);
    });

    test('连续断口只切一次，不留空段', () {
      final runs = trendRuns([10, null, null, 20]);
      expect(runs.length, 2);
      expect(runs.map((r) => r.values.length).toList(), [1, 1]);
    });

    test('孤立单点成为一段（画点而不是丢掉）', () {
      final runs = trendRuns([null, 50, null]);
      expect(runs.length, 1);
      expect(runs[0].start, 1);
      expect(runs[0].values, [50]);
    });

    test('全为 null 时没有段：界面走「没有可用读数」而不是画 0', () {
      expect(trendRuns([null, null, null]), isEmpty);
    });

    test('正控：无断口是一段连续线', () {
      final runs = trendRuns([30, 60, 90]);
      expect(runs.length, 1);
      expect(runs[0].values, [30, 60, 90]);
    });
  });

  group('trendWindowLabel', () {
    test('被上限截断：说出已画条数与采集总数', () {
      const w = TrendWindow(shown: 60, total: 860, truncated: true);
      expect(trendWindowLabel(w), '趋势图仅最近 60 次采集（该设备共 860 次）');
    });

    test('总数读不出：说「可能还有更多」，不写成 null 或 0', () {
      const w = TrendWindow(shown: 60, truncated: true);
      final label = trendWindowLabel(w);
      expect(label, contains('读取失败'));
      expect(label, isNot(contains('null')));
      expect(label, isNot(contains('共 0 次')));
    });

    test('正控：一次拿全时不出声', () {
      const w = TrendWindow(shown: 12, total: 12);
      expect(trendWindowLabel(w), isEmpty);
    });
  });
}
