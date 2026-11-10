// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 「有单页上限的清单」的统一判定：移动端多处清单共用它（种子史等）。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/common/capped_list_notice.dart';

void main() {
  String unk(int n) => '已显示最近 $n 条，但总数读取失败——可能还有更多';
  String kn(int n, int total) => '仅显示最近 $n 条（共 $total 条）';

  test('被上限截断：用总数句式', () {
    expect(
      cappedListNotice(
        shown: 500,
        total: 820,
        truncated: true,
        unknownTotal: unk,
        knownTotal: kn,
      ),
      '仅显示最近 500 条（共 820 条）',
    );
  });

  test('总数读不出：走 unknownTotal，绝不渲染 0 或 null', () {
    final s = cappedListNotice(
      shown: 500,
      truncated: true,
      unknownTotal: unk,
      knownTotal: kn,
    );
    expect(s, contains('读取失败'));
    expect(s, isNot(contains('共 0')));
    expect(s, isNot(contains('null')));
  });

  test('后端说截断却给出不大于已显示数的 total：不谎报还有更多', () {
    expect(
      cappedListNotice(
        shown: 500,
        total: 500,
        truncated: true,
        unknownTotal: unk,
        knownTotal: kn,
      ),
      isEmpty,
    );
    expect(
      cappedListNotice(
        shown: 500,
        total: 120,
        truncated: true,
        unknownTotal: unk,
        knownTotal: kn,
      ),
      isEmpty,
    );
  });

  test('正控：未截断时即使 total 更大也不出声', () {
    expect(
      cappedListNotice(
        shown: 8,
        total: 900,
        truncated: false,
        unknownTotal: unk,
        knownTotal: kn,
      ),
      isEmpty,
    );
  });
}
