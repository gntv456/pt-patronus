// 玻璃降级纯函数测试：钉死 sigma 归零触发跳过 BackdropFilter + 降级时底色不透明度。
import 'package:flutter_test/flutter_test.dart';
import 'package:ptpatronus/theme/glass.dart';

void main() {
  group('effectiveGlassSigma', () {
    test('非降级时原样返回', () {
      expect(effectiveGlassSigma(18, false), 18);
    });
    test('降级时强制 0（触发跳过 BackdropFilter）', () {
      expect(effectiveGlassSigma(18, true), 0);
    });
    test('调用方传 0 也得 0', () {
      expect(effectiveGlassSigma(0, false), 0);
    });
  });

  group('glassCardAlpha', () {
    test('亮色非降级 0.55', () => expect(glassCardAlpha(false, false), 0.55));
    test('亮色降级 0.80（无模糊抬高保可读）', () => expect(glassCardAlpha(false, true), 0.80));
    test('暗色非降级 0.06', () => expect(glassCardAlpha(true, false), 0.06));
    test('暗色降级 0.14', () => expect(glassCardAlpha(true, true), 0.14));
  });
}
