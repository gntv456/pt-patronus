// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// RSS 检查/条目账面：失败与「只查了窗口」都不能被渲染成「一切正常」。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/rss.dart';
import 'package:ptpatronus/features/rss/rss_account.dart';

void main() {
  group('rssCheckSummary', () {
    test('ok=false：说得出后端给的中文原因，绝不回退成「新增 0 条」', () {
      const r = RssCheckResult(ok: false, error: '订阅源抓取失败：连接超时');
      expect(rssCheckSummary(r), '检查失败：订阅源抓取失败：连接超时');
    });

    test('ok=false 且原因为空：也要承认失败，不给安心文案', () {
      const r = RssCheckResult(ok: false);
      expect(rssCheckSummary(r), contains('检查失败'));
    });

    test('被窗口截断：说出源总数与本轮已查条数', () {
      const r = RssCheckResult(
        ok: true,
        newCount: 3,
        scanned: 200,
        total: 250,
        truncated: true,
      );
      expect(rssCheckSummary(r), contains('源共 250 条'));
      expect(rssCheckSummary(r), contains('只检查了最近 200 条'));
    });

    test('总数读不出：说「读取失败」，不写成 0 条', () {
      const r = RssCheckResult(
        ok: true,
        newCount: 3,
        scanned: 200,
        truncated: true,
      );
      final s = rssCheckSummary(r);
      expect(s, contains('总数读取失败'));
      expect(s, isNot(contains('源共 0 条')));
    });

    test('正控：整个源查完时只报新增数', () {
      const r = RssCheckResult(ok: true, newCount: 5, scanned: 40, total: 40);
      expect(rssCheckSummary(r), '新增 5 条');
    });
  });

  group('rssItemWindowLabel', () {
    test('一页装不下：说出已显示条数与未显示条数', () {
      final p = RssItemPage(
        items: List.filled(100, const RssItem(title: 'x')),
        total: 260,
        limit: 100,
        truncated: true,
      );
      expect(rssItemWindowLabel(p), '仅显示最近 100 条，该订阅共 260 条（还有 160 条未显示）');
    });

    test('总数读不出：不编 total，也不说「这就是全部」', () {
      final p = RssItemPage(
        items: List.filled(100, const RssItem(title: 'x')),
        truncated: true,
      );
      expect(rssItemWindowLabel(p), contains('总数读取失败'));
    });

    test('后端说截断却给出不大于已显示数的 total：不谎报「还有 N 条」', () {
      final p = RssItemPage(
        items: List.filled(3, const RssItem(title: 'x')),
        total: 3,
        truncated: true,
      );
      expect(rssItemWindowLabel(p), isEmpty);
    });

    test('正控：一次拿全时不出声', () {
      final p = RssItemPage(
        items: List.filled(12, const RssItem(title: 'x')),
        total: 12,
      );
      expect(rssItemWindowLabel(p), isEmpty);
    });
  });

  group('响应缺字段时的取值方向', () {
    test('check 响应没有 ok：按失败处理，不把缺字段当成功', () {
      final r = RssCheckResult.fromJson({});
      expect(r.ok, false);
      expect(rssCheckSummary(r), contains('检查失败'));
    });

    test('items 响应没有 truncated：按「可能被截断」处理，不说「这就是全部」', () {
      final p = RssItemPage.fromJson({
        'items': [
          {'title': 'a'},
        ],
      });
      expect(p.truncated, true);
      expect(p.total, isNull);
      expect(rssItemWindowLabel(p), contains('总数读取失败'));
    });

    test('正控：items 明确未截断时不回退成截断文案', () {
      final p = RssItemPage.fromJson({
        'items': [
          {'title': 'a'},
        ],
        'total': 1,
        'limit': 100,
        'truncated': false,
      });
      expect(rssItemWindowLabel(p), isEmpty);
    });
  });
}
