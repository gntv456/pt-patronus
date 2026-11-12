// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/backup.dart';
import 'package:ptpatronus/features/backup/backup_notice.dart';

// #256：后端 /backup/import 一次给全四份账（导入数 / 跳过数 / 失败数+明细 / 空备份标记），
// 移动端从前只取前两个数，然后写死一句「导入完成」——
// 用户以为恢复完了，实际同名站点被跳过、下载器创建失败，甚至这份文件压根没导进任何东西。
void main() {
  group('importOutcomeNotice（导入结果不能塌成一句「完成」）', () {
    test('全部干净 ⇒ 才配「导入完成」', () {
      final n = importOutcomeNotice(
        const ImportResult(sites: 5, downloaders: 2),
      );
      expect(n, '导入完成：站点 5 / 下载器 2');
      expect(n, isNot(contains('失败')));
    });

    test('有跳过 ⇒ 必须点名跳了几个，且不许再说「完成」', () {
      final n = importOutcomeNotice(
        const ImportResult(sites: 5, downloaders: 2, sitesSkipped: 3),
      );
      expect(n, contains('跳过 3 个站点'));
      expect(n, isNot(contains('导入完成')));
    });

    test('有下载器失败 ⇒ 点名数量并带出后端给的原因（前 3 条）', () {
      final n = importOutcomeNotice(
        const ImportResult(
          sites: 4,
          downloaders: 1,
          downloadersFailed: 4,
          failures: [
            '下载器 A: connection refused',
            '下载器 B: 401 unauthorized',
            '下载器 C: bad host',
            '下载器 D: timeout',
          ],
        ),
      );
      expect(n, contains('4 个下载器创建失败'));
      expect(n, contains('connection refused'));
      expect(n, contains('bad host'));
      expect(n, isNot(contains('timeout'))); // 只贴前 3 条
      expect(n, contains('共 4 条')); // 但总数要说全
      expect(n, isNot(contains('导入完成')));
    });

    test('空备份 ⇒ 说「没有可导入的内容」，不许报成功', () {
      final n = importOutcomeNotice(
        const ImportResult(sites: 0, downloaders: 0, empty: true),
      );
      expect(n, contains('没有可导入的内容'));
      expect(n, isNot(contains('导入完成')));
    });

    // #257：外部格式两条路径只导站点，后端不给下载器维度。
    test('站点单维路径不许写出「下载器 0」这种误导数字', () {
      final n = importOutcomeNotice(
        const ImportResult(sites: 3, downloaders: 0, sitesSkipped: 2),
        withDownloaders: false,
      );
      expect(n, contains('站点 3'));
      expect(n, isNot(contains('下载器')));
      expect(n, contains('跳过 2 个站点'));
      expect(n, isNot(contains('导入完成')));
    });
  });

  group('解析层：四份账都得从响应里取回来', () {
    test('完整响应 ⇒ 跳过数/失败数/明细/empty 都在', () {
      final r = ImportResult.fromJson(const {
        'empty': false,
        'imported_sites': 5,
        'imported_downloaders': 2,
        'sites_skipped': 3,
        'downloaders_failed': 1,
        'failures': ['下载器 A: boom'],
      });
      expect(r.sites, 5);
      expect(r.sitesSkipped, 3);
      expect(r.downloadersFailed, 1);
      expect(r.failures, ['下载器 A: boom']);
      expect(r.empty, isFalse);
    });

    test('旧后端/缺字段 ⇒ 全部按 0 / 空处理，不许抛出', () {
      final r = ImportResult.fromJson(const {'imported_sites': 2});
      expect(r.sites, 2);
      expect(r.downloaders, 0);
      expect(r.sitesSkipped, 0);
      expect(r.failures, isEmpty);
      expect(r.empty, isFalse);
    });
  });

  // 接线护栏：以上两层全绿也不代表页面真的用了这句文案（同 dashboard_banner_wiring_test 的教训）。
  group('页面接线', () {
    final src = File('lib/features/backup/backup_page.dart')
        .readAsLinesSync()
        .map((l) {
          final i = l.indexOf('//');
          return i >= 0 ? l.substring(0, i) : l;
        })
        .join('\n');

    test('备份页必须用这条账面文案，而不是自己拼「导入完成」', () {
      expect(src, contains('importOutcomeNotice(r)'));
      expect(src, isNot(contains("导入完成：站点")));
    });

    test('三条导入路径共用这一份文案（MP/.ptd 不许各自拼一句「已导入 N 个站点」）', () {
      expect('importOutcomeNotice(r'.allMatches(src).length, 3);
      expect(src, isNot(contains(r"已导入 $n 个站点")));
    });
  });
}
