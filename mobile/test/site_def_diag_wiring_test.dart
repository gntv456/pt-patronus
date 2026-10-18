import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/site.dart';

// #333（承接 #331）：后端已经把四类「站点定义被静默忽略」收成账面，
// 移动端必须真读到并说出来，且不能把「读失败」讲成「没有配置问题」，
// 也不能因为这一路失败把站点列表整页拖没（本仓同族缺陷 #206/#279 的形状）。
// 沿用仓里的读真源码护栏做法（test/site_checkin_listerr_wiring_test.dart）。

String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  group('DefinitionDiagnostic 模型', () {
    test('缺键一律落成空串，不炸（后端字段是 omitempty）', () {
      final d = DefinitionDiagnostic.fromJson(const {'kind': 'unknown_key'});
      expect(d.kind, 'unknown_key');
      expect(d.site, '');
      expect(d.file, '');
      expect(d.detail, '');
    });

    test('定位对象优先站点 id，其次文件名', () {
      expect(const DefinitionDiagnostic(kind: 'duplicate_id', site: 'sameweb', file: 'b.yaml').target, 'sameweb');
      expect(const DefinitionDiagnostic(kind: 'unknown_key', file: 'myself.yaml').target, 'myself.yaml');
      expect(const DefinitionDiagnostic(kind: 'skipped_subdir').target, '');
    });

    test('DefinitionDiagnostics 的空默认是「items 空 + 无读失败」两个维度', () {
      const empty = DefinitionDiagnostics();
      expect(empty.items, isEmpty);
      expect(empty.readError, '');
      const failed = DefinitionDiagnostics(readError: '网络错误');
      expect(failed.items, isEmpty);
      expect(failed.readError, '网络错误');
    });
  });

  final api = _code('lib/api/site.dart');
  final page = _code('lib/features/sites/sites_page.dart');

  test('api 层必须打独立诊断端点，并把失败原因带回而不是抛出或吞成空', () {
    expect(api, contains("'/sites/definition-diagnostics'"));
    expect(api, contains("r['diagnostics']"));
    expect(api, contains('readError: e.toString()'));
    // 正控：同文件里既有的 definitions() 不能被这次改动弄丢（添加站点表单还在读它）。
    expect(api, contains("r['definitions']"));
  });

  test('诊断这一路不进 Future.wait：它失败不许拖走站点列表', () {
    final wait = RegExp(r'Future\.wait\(\[[\s\S]*?\]\);').firstMatch(page);
    expect(wait, isNotNull, reason: '页面主加载应仍是 Future.wait（正控：结构没被改坏）');
    expect(wait!.group(0), isNot(contains('_loadDefDiags')), reason: '诊断必须独立一路读');
    expect(page, contains('unawaited(_loadDefDiags())'));
  });

  test('页面必须区分「读失败」与「没有配置问题」两种说法', () {
    expect(page, contains('站点定义诊断读取失败'));
    expect(page, contains('条配置没有生效'));
    expect(page, contains('_defDiagsErr.isEmpty'), reason: '两者都空时不画条（不能画成「0 条问题」的假安心）');
  });

  test('后端新增 kind 时不许露出裸英文，要有「其它配置问题」兜底', () {
    expect(page, contains("_ => '其它配置问题'"));
    for (final k in ['bad_regexp', 'unknown_key', 'duplicate_id', 'skipped_subdir']) {
      expect(page, contains("'$k'"), reason: '四类都要有自己的中文标签');
    }
  });

  test('诊断条必须真挂进 body（只定义不挂载＝用户还是看不见）', () {
    expect(page, contains('Widget _defDiagBanner()'));
    expect(page, contains('_defDiagBanner(),'), reason: '必须出现在 body children 里，否则只是定义了个方法');
  });
}
