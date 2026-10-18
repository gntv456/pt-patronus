// 移动端仪表盘并发区的失败口径：六路并发取数中某一路读失败时，界面必须说「加载失败」，
// 不能拿 0/空数组继续渲染成「你真的没有」。区名互不相同也要守住——重名会让一个区的失败
// 顶掉另一个区的账面。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/features/dashboard/dashboard_extras.dart';

void main() {
  group('extrasForbidden', () {
    test('403 与 admin_required 都是「没权限」，不是读失败', () {
      expect(extrasForbidden(ApiException(403, '需要管理员权限')), isTrue);
      expect(
        extrasForbidden(ApiException(0, 'x', code: 'admin_required')),
        isTrue,
      );
    });

    test('正控：其它状态码与网络异常仍按读失败记账', () {
      expect(extrasForbidden(ApiException(500, 'boom')), isFalse);
      expect(extrasForbidden(Exception('timeout')), isFalse);
    });
  });
  group('extrasAvailable', () {
    test('记了账的区不可用，未记账的区仍可用', () {
      final failed = [extrasSectionWorkflow, extrasSectionNotice];
      expect(extrasAvailable(failed, extrasSectionWorkflow), isFalse);
      expect(extrasAvailable(failed, extrasSectionBrush), isTrue);
    });

    test('空账本时全部可用', () {
      expect(extrasAvailable(const [], extrasSectionAgent), isTrue);
    });
  });

  group('extrasValue', () {
    test('失败的区给出「加载失败」而不是 0', () {
      expect(
        extrasValue(
          failed: const [extrasSectionContract],
          section: extrasSectionContract,
          value: '0',
        ),
        '加载失败',
      );
    });

    test('可用的区原样给出数值', () {
      expect(
        extrasValue(
          failed: const [extrasSectionContract],
          section: extrasSectionWorkflow,
          value: '3/5',
        ),
        '3/5',
      );
    });

    test('占位文案可点名具体动作', () {
      expect(
        extrasValue(
          failed: const [extrasSectionNotice],
          section: extrasSectionNotice,
          value: '暂无最近活动',
          unavailable: '通知读取失败，下拉刷新重试',
        ),
        '通知读取失败，下拉刷新重试',
      );
    });
  });

  group('extrasNotice', () {
    test('逐个点名失败的区', () {
      final notice =
          extrasNotice(const [extrasSectionWorkflow, extrasSectionMetrics]);
      expect(notice, contains('工作流'));
      expect(notice, contains('系统资源'));
      expect(notice, contains('不是全部'));
    });

    test('全部成功时不给提示', () {
      expect(extrasNotice(const []), isEmpty);
    });
  });

  group('区名常量', () {
    test('无权限与读失败是两回事，文案不能塌成同一条', () {
      expect(extrasAdminOnly, isNot('加载失败'));
      expect(extrasAdminOnly, contains('管理员'));
    });
    test('六个区互不重名', () {
      const names = [
        extrasSectionWorkflow,
        extrasSectionAgent,
        extrasSectionBrush,
        extrasSectionContract,
        extrasSectionNotice,
        extrasSectionMetrics,
      ];
      expect(names.toSet().length, names.length);
      for (final name in names) {
        expect(name, isNotEmpty);
      }
    });
  });

  // #255：每站统计那条查询失败时后端仍回 200，只在响应里带 stat_read_error。
  // 这条账面是「这次没读到」与「站点真的没数据」之间唯一的分界；Web 早就有横幅，
  // 移动端以前连字段都没解析，于是满屏 0 值没人解释。
  group('statReadNotice（读取失败 vs 真没数据）', () {
    test('没给原因 ⇒ 空串，页面不占位也不编告警', () {
      expect(statReadNotice(null), isEmpty);
      expect(statReadNotice(''), isEmpty);
    });

    test('有原因 ⇒ 点名是「读取失败」并原样带上后端那句', () {
      final n = statReadNotice('database is locked');
      expect(n, contains('每站统计读取失败'));
      expect(n, contains('database is locked'));
      // 不许塌成「无数据/没数据」——那是另一种结论。
      expect(n, isNot(contains('无数据')));
      expect(n, contains('0 值'));
    });
  });
}
