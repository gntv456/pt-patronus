import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/core/api_client.dart';

/// PR-4：ApiException 的机器可读 code 通道。
/// 后端统一信封 {code, message, error}——code 供业务编程分支，
/// 不改变 message/userMessage 的既有文案优先级。
void main() {
  group('ApiException.code', () {
    test('统一信封 code 透传', () {
      final e = ApiException(400, '高危操作，需要先确认风险',
          code: 'risk_confirmation_required');
      expect(e.code, 'risk_confirmation_required');
      // 文案优先级不变：userMessage 仍走 friendlyMessage（后端消息优先）
      expect(e.userMessage, '高危操作，需要先确认风险');
    });

    test('无 code 时默认空串（老响应兼容）', () {
      final e = ApiException(404, '请求的资源不存在');
      expect(e.code, '');
    });

    test('code 不影响网络错误兜底文案', () {
      final e = ApiException(0, 'SocketException',
          userMessage: '网络连接失败，请检查网络或后端地址');
      expect(e.code, '');
      expect(e.userMessage, '网络连接失败，请检查网络或后端地址');
    });
  });

  group('friendlyMessage 优先级不变', () {
    test('后端消息非空时优先展示', () {
      expect(friendlyMessage(409, '重复支出，请刷新后重试'), '重复支出，请刷新后重试');
    });

    test('后端无消息按状态码兜底', () {
      expect(friendlyMessage(401, '请求失败'), '登录已失效，请重新登录');
      expect(friendlyMessage(429, '请求失败'), '操作过于频繁，请稍后再试');
      expect(friendlyMessage(503, '请求失败'), '服务暂时不可用，请稍后重试');
    });
  });

  group('sseErrCode', () {
    test('SSE 错误信封提取 code', () {
      expect(
          sseErrCode('{"code":"vip_expired","message":"好学 VIP 已过期"}'),
          'vip_expired');
    });

    test('非 JSON / 无 code 返回 null', () {
      expect(sseErrCode('not json'), isNull);
      expect(sseErrCode('{"message":"x"}'), isNull);
    });
  });
}
