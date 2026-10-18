import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/api/subscribe.dart';

// 卡片里剥掉 // 注释再扫，免得我自己写的说明文字造出假红（同仓内护栏的做法）。
String _code(String path) {
  final lines = File(path).readAsLinesSync().map((l) {
    final i = l.indexOf('//');
    return i >= 0 ? l.substring(0, i) : l;
  }).toList();
  return lines.join('\n');
}

void main() {
  group('订阅卡片的「上次检查有问题」账面', () {
    test('fromJson 要把 last_check_error 带出来，缺字段时是空串（=整轮干净）', () {
      final dirty = Subscription.fromJson({
        'id': 1,
        'title': '某电影',
        'type': 'movie',
        'last_checked_at': '2026-10-14T00:00:00Z',
        'last_check_error': '1/2 个站本轮读不出来：bad（该站点登录态已失效）',
      });
      expect(dirty.lastCheckError, contains('bad'));
      final clean = Subscription.fromJson({
        'id': 2,
        'title': '某剧',
        'type': 'tv',
        'last_checked_at': '2026-10-14T00:00:00Z',
      });
      expect(clean.lastCheckError, isEmpty);
    });

    // 负控实测：把订阅卡片里那条分支删掉，上面这条模型用例一个字都不会红
    // ——所以接线要单独钉（字段进了接口类型 ≠ 上了屏幕）。
    test('订阅卡片必须真的把它渲染出来（接线不许断在最后一米）', () {
      final src = _code(
        'lib/features/subscriptions/subscriptions_widgets.dart',
      );
      expect(src.contains('s.lastCheckError'), isTrue);
      expect(src.contains("'上次检查有问题：\${s.lastCheckError}'"), isTrue);
      // 只在有原因时才出现（不许把干净轮次也标成有问题）。
      expect(src.contains('if (s.lastCheckError.isNotEmpty)'), isTrue);
    });
  });
}
