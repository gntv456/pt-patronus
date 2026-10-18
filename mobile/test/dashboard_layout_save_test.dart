import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/features/dashboard/dashboard_page.dart';

// 「保存仪表盘布局」的成败口径：写本机偏好失败时说「已保存」并把面板切成新布局，
// 用户要到下次启动才发现设置回到了旧的那套。判定/文案在 layoutSaveFailure，
// 「失败绝不应用」的编排在 saveDashboardConfig（写入走注入缝，shared_preferences
// 的 mock 永远返回 true，造不出失败）。
void main() {
  group('layoutSaveFailure', () {
    test('写入失败必须给出中文原因（而不是 null 让调用方照常应用）', () {
      final reason = layoutSaveFailure(false);
      expect(reason, isNotNull);
      expect(reason, contains('保存失败'));
      expect(reason, contains('下次启动'));
      expect(reason, isNot(contains('SharedPreferences')));
    });

    test('写入成功返回 null，调用方据此才应用新配置', () {
      expect(layoutSaveFailure(true), isNull);
    });
  });

  group('saveDashboardConfig', () {
    test('写失败：出声且一次都不应用', () async {
      String? heard;
      var applied = 0;
      await saveDashboardConfig(
        key: 'k',
        payload: '{}',
        write: (k, v) async => false,
        onFail: (reason) => heard = reason,
        apply: () => applied++,
      );
      expect(heard, isNotNull, reason: '写失败必须把原因交给界面');
      expect(heard, contains('保存失败'));
      expect(applied, 0, reason: '没写进本机偏好就不该切内存态');
    });

    test('写成功：应用新配置且不吭声', () async {
      String? heard;
      var applied = 0;
      await saveDashboardConfig(
        key: 'k',
        payload: '{}',
        write: (k, v) async => true,
        onFail: (reason) => heard = reason,
        apply: () => applied++,
      );
      expect(heard, isNull);
      expect(applied, 1);
    });

    test('写入本身抛错时不静默应用（异常透给调用方）', () async {
      var applied = 0;
      expect(
        saveDashboardConfig(
          key: 'k',
          payload: '{}',
          write: (k, v) async => throw StateError('平台通道断了'),
          onFail: (_) {},
          apply: () => applied++,
        ),
        throwsStateError,
      );
      expect(applied, 0);
    });
  });
}
