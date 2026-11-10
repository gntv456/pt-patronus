// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 整理记录的取数窗口文案：单页封顶 500、默认 200，被截断必须说出来。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/transfer/transfer_account.dart';

void main() {
  test('被上限截断：说出已显示条数与总条数', () {
    expect(
      transferHistoryWindowLabel(shown: 200, total: 460, truncated: true),
      '仅显示最近 200 条整理记录（共 460 条，更早的未加载）',
    );
  });

  test('总数读不出：说「读取失败」，不写成共 0 条或共 null 条', () {
    final s = transferHistoryWindowLabel(shown: 200, truncated: true);
    expect(s, contains('读取失败'));
    expect(s, isNot(contains('共 0 条')));
    expect(s, isNot(contains('null')));
  });

  test('后端说截断却给出不大于已显示数的 total：不谎报「还有更多」', () {
    expect(
      transferHistoryWindowLabel(shown: 200, total: 200, truncated: true),
      isEmpty,
    );
    expect(
      transferHistoryWindowLabel(shown: 200, total: 150, truncated: true),
      isEmpty,
    );
  });

  test('正控：一次拿全时不出声', () {
    expect(
      transferHistoryWindowLabel(shown: 12, total: 1200, truncated: false),
      isEmpty,
    );
  });
}
