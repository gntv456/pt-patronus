import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/err_text.dart';

// #403 移动端 catch (_) 把后端写的可操作原因整个丢掉。
//
// 最典型的一处：仪表盘「单站刷新」调 dashboard.syncOne，后端那个出口在「站点凭据
// 解不开」时回的是
//
//	503 site_credential_unreadable「这个站点的凭据读不出来…请到「站点」里重新保存该站点的凭据」
//
// （后端 #398）。而移动端写成 catch (_) { _toast(站点刷新失败); } ——
// ApiException 的 userMessage 连同后端那句一起被丢掉。
// 用户只知道刷新失败，不知道该去站点页重登 Cookie，于是反复点刷新。
//
// 另一处同类：连通状态读失败时静默回空清单，界面上与「这些站真没检测过」完全同形，
// 而这一页正是用户判断站点是否掉线的依据。

/// 后端 #398 在凭据解不开时给的那句（真数据，从响应信封里取）。
const _credReason = '这个站点的凭据读不出来（可能更换过主密钥 SECRET_KEY）：本次没有向站点发请求，站点那边没有发生任何事。请到「站点」里重新保存该站点的凭据后再试。原因见下方。';

ApiException _credErr() => ApiException(503, _credReason, code: 'site_credential_unreadable');

void main() {
  test('#403 errTextOf 取 ApiException 的用户文案（后端那句话要能到达界面）', () {
    expect(errTextOf(_credErr()), _credReason);
  });

  test('#403 非 ApiException 不编一个原因（与后端「未知码原样带出」同一原则）', () {
    // 拿不到真原因就返回 null，调用方继续用它自己那句兜底文案 ——
    // 绝不凭空造一句「网络异常」之类的猜测盖住真实故障。
    expect(errTextOf(Exception('boom')), isNull);
    expect(errTextOf(null), isNull);
    expect(errTextOf('一个字符串'), isNull);
  });

  test('#403 userMessage 为空时也回 null（不许回空串冒充有原因）', () {
    final e = ApiException(500, '', userMessage: '');
    expect(errTextOf(e), isNull);
  });
  /// 去掉整行注释，免得把判据本身当成被测代码。
  String readSource(String path) {
    final lines = File(path).readAsLinesSync().map((l) {
      final i = l.indexOf('//');
      return i >= 0 ? l.substring(0, i) : l;
    }).toList();
    return lines.join('\n');
  }

  /// 取某个方法的函数体（按大括号配平）。
  String bodyOf(String src, String marker) {
    final at = src.indexOf(marker);
    if (at < 0) return '';
    var depth = 0;
    var started = false;
    for (var i = at; i < src.length; i++) {
      final c = src[i];
      if (c == '{') {
        depth++;
        started = true;
      } else if (c == '}') {
        depth--;
        if (started && depth == 0) return src.substring(at, i + 1);
      }
    }
    return src.substring(at);
  }

  test('#403 单站刷新/全站同步不许再用 catch (_) 丢掉原因', () {
    final src = readSource('lib/features/dashboard/dashboard_data.dart');
    for (final fn in ['_syncOne(', '_syncAll(']) {
      final body = bodyOf(src, fn);
      expect(body, isNotEmpty, reason: '$fn 的函数体没截到');
      expect(body, contains('errTextOf('),
          reason: '$fn 的 catch 必须用 errTextOf(e) 取出后端那句可操作文案');
      expect(body, isNot(contains('catch (_)')),
          reason: '$fn 又变回 catch (_)（后端原因会被整个丢掉）');
    }
  });

  // 仪表盘与站点页是**同一形状的两份拷贝**（各自的 part 都有 _loadConnectivity + 徽章）。
  // #403 修了仪表盘那份、#404 补上站点页那份 —— 所以这条判据一次核两页，
  // 下次再加第三份拷贝时会立刻红。
  for (final page in const [
    ('仪表盘', 'lib/features/dashboard/dashboard_data.dart', 'lib/features/dashboard/dashboard_site_cards.dart'),
    ('站点页', 'lib/features/sites/sites_checkin.dart', 'lib/features/sites/sites_rows.dart'),
  ]) {
    test('#404 $page 连通状态读失败必须被记下来并可点开看原因', () {
      final data = readSource(page.$2);
      final rows = readSource(page.$3);
      // 读失败必须记下原因（供徽章与对话框消费）。
      expect(data, contains('_connectivityError = errTextOf(e)'),
          reason: 'readSource 没认出 $page 的 _loadConnectivity');
      // 徽章必须区分「没读到」与「未检测」。
      expect(rows, contains('未读到'));
      expect(rows, contains('未检测'));
      // 读失败那一档必须可点开看原因（此前 status == null 直接 return，不可点）。
      expect(rows, contains('_showConnectivityReadError'));
    });
  }
}
