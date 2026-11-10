// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/trakt.dart';

// #280 移动端 Trakt：轮询要知道「再等也不会亮」，写操作不许猜死一个原因。

String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  test('终端态码判定：过期/没配/未授权都停，只有上游抖动继续等', () {
    expect(traktPollIsTerminal('trakt_code_expired'), isTrue);
    expect(traktPollIsTerminal('trakt_not_configured'), isTrue);
    expect(traktPollIsTerminal('trakt_not_authorized'), isTrue);
    expect(traktPollIsTerminal('trakt_upstream_failed'), isFalse);
    expect(traktPollIsTerminal(''), isFalse);
  });

  test('放弃轮询的措辞只能说「读不到」，不能说授权失败', () {
    final text = traktPollGiveUpText(traktPollGiveUpAfter);
    expect(text, contains('读不到'));
    expect(text, contains('$traktPollGiveUpAfter'));
    expect(text, isNot(contains('授权失败')));
  });

  group('接线', () {
    test('deviceStatus 必须把非 2xx 的 code/人话交回调用方（不再只回一个 bool）', () {
      final api = _code('lib/api/trakt.dart');
      expect(api, contains('Future<TraktDeviceStatus> deviceStatus'));
      expect(api, contains('on ApiException catch'));
      expect(api, contains('traktPollIsTerminal(e.code)'));
      expect(api, contains('message: e.userMessage'));
    });

    test('轮询不再无条件 keep polling：终端态要停并把原因说出来', () {
      final page = _code('lib/features/trakt/trakt_page.dart');
      expect(page, isNot(contains('/* keep polling */')));
      expect(page, contains('if (st.terminal)'));
      final terminal = page.substring(page.indexOf('if (st.terminal)'));
      expect(terminal.substring(0, 420), contains('_poll?.cancel()'));
      expect(terminal.substring(0, 420), contains('st.message'));
      expect(page, contains('traktPollGiveUpText(fails)'));
    });

    test('发起授权/同步不再猜死一个原因，带上后端那句', () {
      final page = _code('lib/features/trakt/trakt_page.dart');
      expect(page, isNot(contains('请先在配置填 client_id/secret')));
      expect(page, contains("_toast('发起授权失败：\$e')"));
      expect(page, contains("_toast('同步失败：\$e')"));
      expect(page, isNot(contains("'同步失败，请先完成授权'")));
    });
  });
}
