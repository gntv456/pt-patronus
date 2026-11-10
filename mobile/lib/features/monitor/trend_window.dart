// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// trend_window.dart —— 设备趋势图的两个纯函数：取数窗口文案、按断口分段。
//
// 后端 /monitors/:id/metrics 一次最多回 limit 条，并把 total/truncated 一起带回来；
// 只画「最近 N 次」却不说，用户会把它读成这台设备的全部历史。
// 无读数的采样（离线 / 采集失败 / 只带 online 的手动探活）在序列里是 null，
// 补 0 会把掉线时段画成「CPU 跌到 0%」的假断崖。
import 'package:flutter/foundation.dart';

/// 趋势图取数窗口：已画条数、该设备采集总数（读不出为 null）、是否被上限截断。
@immutable
class TrendWindow {
  const TrendWindow({required this.shown, this.total, this.truncated = false});

  final int shown;
  final int? total;
  final bool truncated;
}

/// 一句话说明这张图看的是多少条采集；总数读不出时不能写成「共 null 次」。
String trendWindowLabel(TrendWindow w) {
  if (!w.truncated) return '';
  final total = w.total;
  if (total == null) {
    return '趋势图仅最近 ${w.shown} 次采集（采集总数读取失败，可能还有更多）';
  }
  return '趋势图仅最近 ${w.shown} 次采集（该设备共 $total 次）';
}

/// 一段连续有读数的采样：[start] 是它在原序列里的下标，[values] 是段内取值。
@immutable
class TrendRun {
  const TrendRun(this.start, this.values);

  final int start;
  final List<double> values;
}

/// 按 null 断口把序列切成连续段：段内保留原始横坐标，段间留白，
/// 于是掉线时段在图上是一段空白而不是 0% 的假谷。
List<TrendRun> trendRuns(List<double?> data) {
  final runs = <TrendRun>[];
  var cur = <double>[];
  var start = 0;
  for (var i = 0; i < data.length; i++) {
    final v = data[i];
    if (v == null) {
      if (cur.isNotEmpty) {
        runs.add(TrendRun(start, cur));
        cur = <double>[];
      }
      continue;
    }
    if (cur.isEmpty) start = i;
    cur.add(v);
  }
  if (cur.isNotEmpty) runs.add(TrendRun(start, cur));
  return runs;
}
