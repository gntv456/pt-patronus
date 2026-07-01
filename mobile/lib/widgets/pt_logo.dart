import 'package:flutter/material.dart';

/// PTPatronus 品牌吉祥物 —— 忠实复刻 Web [frontend/public/logo-mark.svg]（1024² 坐标系）。
///
/// 用 `CustomPainter` 而非 flutter_svg：SVG 用了 `feTurbulence`/`feDisplacementMap`/
/// `feGaussianBlur`，flutter_svg 不支持；这些液态/阴影在小尺寸不可见，故丢弃，只保留
/// 渐变 + 路径 + 文字（路径用 [path_parsing] 直接从 SVG d 串解析，保持可核对）。
///
/// - `detailed=true`（默认，in-app）：含笑脸；`false` 去脸（启动器图标源，小尺寸更清晰）。
/// - `background=false`（默认，in-app）：透明底，叠在玻璃上；`true`：彩色渐变圆角满铺
///   （对齐 Web logo.svg 的 `bg` 渐变 + 光斑），用作启动器图标源 PNG。
class PTLogo extends StatelessWidget {
  final double size;
  final bool detailed;
  final bool background;

  const PTLogo({
    super.key,
    this.size = 34,
    this.detailed = true,
    this.background = false,
  });

  @override
  Widget build(BuildContext context) => SizedBox(
        width: size,
        height: size,
        child: CustomPaint(
          painter: _PTLogoPainter(detailed: detailed, background: background),
        ),
      );
}// —— logo-mark.svg 路径（1024² 坐标），手写 ui.Path（路径仅 M/L/C/Q/Z，无需 SVG 解析器）。
//    与 SVG d 串逐段对应，便于核对：盾牌 / 微笑 / 尖帽。
final Path _shield = _buildShield();
final Path _smile = _buildSmile();
final Path _hat = _buildHat();

Path _buildShield() {
  // M180 290 L844 290 C884 380 896 488 868 580 C824 720 720 812 512 928
  // C304 812 200 720 156 580 C128 488 140 380 180 290 Z
  final p = Path();
  p.moveTo(180, 290);
  p.lineTo(844, 290);
  p.cubicTo(884, 380, 896, 488, 868, 580);
  p.cubicTo(824, 720, 720, 812, 512, 928);
  p.cubicTo(304, 812, 200, 720, 156, 580);
  p.cubicTo(128, 488, 140, 380, 180, 290);
  p.close();
  return p;
}

Path _buildSmile() {
  // M452 556 Q512 620 572 556
  final p = Path();
  p.moveTo(452, 556);
  p.quadraticBezierTo(512, 620, 572, 556);
  return p;
}

Path _buildHat() {
  // M318 300 Q350 220 452 162 Q512 118 572 162 Q674 220 706 300 Z
  final p = Path();
  p.moveTo(318, 300);
  p.quadraticBezierTo(350, 220, 452, 162);
  p.quadraticBezierTo(512, 118, 572, 162);
  p.quadraticBezierTo(674, 220, 706, 300);
  p.close();
  return p;
}

/// 把吉祥物画到 [canvas]（坐标按 1024² 缩放到 [size]）。widget 与启动器图标渲染脚本共用此函数，
/// 保证 in-app logo 与启动器图标视觉一致。
///
/// [withPT] 控制是否画「PT」字标：in-app 用系统字体渲染（默认 true）；启动器图标源渲染
/// 在 flutter test（Ahem 字体会把字渲染成方块），且 48px 下 PT 字不可读，故渲染脚本传 false。
void paintPTLogo(Canvas canvas, Size size,
    {bool detailed = true, bool background = false, bool withPT = true}) {
  const dim = 1024.0;
  canvas.scale(size.width / dim, size.height / dim);

  // —— 彩色背景（仅启动器图标源用）——
  if (background) {
    canvas.clipRRect(RRect.fromRectAndRadius(
      const Rect.fromLTWH(0, 0, dim, dim),
      const Radius.circular(205),
    ));
    final full = Rect.fromLTWH(0, 0, dim, dim);
    canvas.drawRect(
      full,
      Paint()
        ..shader = const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [
            Color(0xFFFF9EC7), Color(0xFFC08CEE), Color(0xFF7B7FF0),
            Color(0xFF5AA8F5), Color(0xFF4AD4EF),
          ],
          stops: [0, 0.26, 0.56, 0.8, 1],
        ).createShader(full),
    );
    final pink = Rect.fromCenter(center: const Offset(300, 300), width: 880, height: 760);
    canvas.drawOval(
      pink,
      Paint()
        ..shader = const RadialGradient(
          colors: [Color(0xFFFFD6EC), Color(0x00FFD6EC)],
        ).createShader(pink),
    );
  }

  final shieldBounds = _shield.getBounds();

  // —— 盾牌玻璃体：白玻璃渐变 + 紫蓝折射 + 双描边 ——
  canvas.drawPath(
    _shield,
    Paint()
      ..shader = LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [
              Colors.white.withValues(alpha: 0.64),
              Colors.white.withValues(alpha: 0.26),
              Colors.white.withValues(alpha: 0.08),
            ],
            stops: const [0, 0.5, 1],
          ).createShader(shieldBounds),
  );
  canvas.drawPath(
    _shield,
    Paint()
      ..shader = LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [
              const Color(0xFF9D8BFF).withValues(alpha: 0.42),
              const Color(0xFF5BB6FF).withValues(alpha: 0.38),
            ],
          ).createShader(shieldBounds),
  );
  canvas.drawPath(
    _shield,
    Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4.5
      ..color = Colors.white.withValues(alpha: 0.82),
  );
  canvas.drawPath(
    _shield,
    Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..color = const Color(0xFF6C8CF5).withValues(alpha: 0.5),
  );

  // —— 脸 + PT（clip 到盾内）——
  canvas.save();
  canvas.clipPath(_shield);
  if (detailed) {
    canvas.drawCircle(const Offset(408, 458), 42, Paint()..color = const Color(0xFF2A2342));
    canvas.drawCircle(const Offset(616, 458), 42, Paint()..color = const Color(0xFF2A2342));
    canvas.drawCircle(const Offset(396, 444), 14, Paint()..color = Colors.white);
    canvas.drawCircle(const Offset(604, 444), 14, Paint()..color = Colors.white);
    canvas.drawPath(
      _smile,
      Paint()
        ..style = PaintingStyle.stroke
        ..strokeWidth = 24
        ..strokeCap = StrokeCap.round
        ..color = const Color(0xFFE85D7E),
    );
  }
  if (withPT) _drawPT(canvas);
  canvas.restore();

  // —— 彩色尖帽 + 帽带 + 顶部球 ——
  canvas.drawPath(
    _hat,
    Paint()
      ..shader = const LinearGradient(
            begin: Alignment.centerLeft,
            end: Alignment.centerRight,
            colors: [
              Color(0xFFFFB24D), Color(0xFFFF6FA5),
              Color(0xFFC46BF0), Color(0xFF9D6CF6),
            ],
            stops: [0, 0.35, 0.7, 1],
          ).createShader(_hat.getBounds()),
  );
  canvas.drawOval(
    Rect.fromCenter(center: const Offset(512, 302), width: 392, height: 56),
    Paint()..color = const Color(0xFF7D52C4),
  );
  canvas.drawOval(
    Rect.fromCenter(center: const Offset(512, 295), width: 368, height: 14),
    Paint()..color = Colors.white.withValues(alpha: 0.32),
  );
  final ball = Rect.fromCircle(center: const Offset(512, 110), radius: 36);
  canvas.drawCircle(
    ball.center,
    36,
    Paint()
      ..shader = const RadialGradient(
            center: Alignment(-0.3, -0.4),
            radius: 0.7,
            colors: [Color(0xFF9AF6FF), Color(0xFF46D4F0), Color(0xFF2BB8D8)],
            stops: [0, 0.6, 1],
          ).createShader(ball),
  );
  canvas.drawCircle(const Offset(500, 97), 13, Paint()..color = Colors.white.withValues(alpha: 0.85));
}

/// PT 渐变字标：SVG baseline 在 (514,798)，字号 188，字重 800，字距 -10。
void _drawPT(Canvas canvas) {
  final tp = TextPainter(
    text: TextSpan(
      text: 'PT',
      style: TextStyle(
        fontSize: 188,
        fontWeight: FontWeight.w800,
        letterSpacing: -10,
        foreground: Paint()
          ..shader = const LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [Color(0xFF6C5CE7), Color(0xFF3A2DB0)],
              ).createShader(const Rect.fromLTWH(150, 600, 728, 300)),
      ),
    ),
    textAlign: TextAlign.center,
    textDirection: TextDirection.ltr,
  );
  tp.layout(maxWidth: 800);
  final baseline = tp.computeLineMetrics().first.baseline;
  tp.paint(canvas, Offset(514 - tp.width / 2, 798 - baseline));
  tp.dispose();
}

class _PTLogoPainter extends CustomPainter {
  final bool detailed;
  final bool background;
  const _PTLogoPainter({required this.detailed, required this.background});

  @override
  void paint(Canvas canvas, Size size) =>
      paintPTLogo(canvas, size, detailed: detailed, background: background);

  @override
  bool shouldRepaint(_PTLogoPainter old) =>
      old.detailed != detailed || old.background != background;
}

