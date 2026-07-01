import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';
import 'glyphs.dart';

/// 液态玻璃风格应用图标 —— Flutter 端对齐 Web [frontend/src/components/GlassIcon.vue]：
/// 30% 圆角方块 + 135° 双色渐变底(from→to) + 顶部内高光 rim + 1px 内白边 + 柔和外阴影 +
/// 居中白色 Lucide 描边字形（字形与 Web GLYPHS 1:1，由 [glyphSvg] 包裹后 flutter_svg 渲染）。
///
/// 用法：导航项、品牌位、指标卡图标。`size` 默认 30（导航项），字形按 60% 居中（同 Web）。
class GlassIcon extends StatelessWidget {
  final String glyph;
  final Color from;
  final Color to;
  final double size;

  const GlassIcon({
    super.key,
    required this.glyph,
    required this.from,
    required this.to,
    this.size = 30,
  });

  @override
  Widget build(BuildContext context) {
    final radius = size * 0.30; // squircle 近似：iOS 连续圆角
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        gradient: LinearGradient(
          colors: [from, to],
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ),
        borderRadius: BorderRadius.circular(radius),
        // 内白边（inset 1px white .22）+ 柔和外阴影（rgba(31,38,135,.22)）
        border: Border.all(color: Colors.white.withValues(alpha: 0.22)),
        boxShadow: [
          BoxShadow(
            color: const Color(0xFF1F2687).withValues(alpha: 0.22),
            blurRadius: size * 0.40,
            offset: Offset(0, size * 0.17),
          ),
        ],
      ),
      child: Stack(
        children: [
          // 顶部内高光 rim：模拟液态玻璃边缘折射（白→透明，前 45%）。
          Positioned.fill(
            child: DecoratedBox(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(radius),
                gradient: LinearGradient(
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                  colors: [
                    Colors.white.withValues(alpha: 0.55),
                    Colors.white.withValues(alpha: 0.0),
                  ],
                  stops: const [0.0, 0.45],
                ),
              ),
            ),
          ),
          // 字形：60% 居中、白色描边（stroke 由 glyphSvg 烧死为 #fff）。
          Center(
            child: SvgPicture.string(
              glyphSvg(glyph),
              width: size * 0.60,
              height: size * 0.60,
            ),
          ),
        ],
      ),
    );
  }
}
