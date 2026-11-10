// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 pt_motion.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/pt_motion.dart';

/// 统一的居中加载视图：柔和的脉动指示器 + 淡入。
/// 替代各页面散落的裸 Center(child: CircularProgressIndicator())。
/// 比默认旋转圈更贴合液态风格——用渐变弧 + 缓动转速。
class PTLoadingView extends StatefulWidget {
  final double size;
  final Color? color;
  const PTLoadingView({super.key, this.size = 32, this.color});
  @override
  State<PTLoadingView> createState() => _PTLoadingViewState();
}

class _PTLoadingViewState extends State<PTLoadingView>
    with TickerProviderStateMixin {
  late final AnimationController _spin;
  late final AnimationController _fade;

  @override
  void initState() {
    super.initState();
    _spin = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1100),
    )..repeat();
    _fade = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 300),
      value: 0,
    )..forward();
  }

  @override
  void dispose() {
    _spin.dispose();
    _fade.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = widget.color ?? Theme.of(context).colorScheme.primary;
    return Semantics(
      label: '加载中',
      excludeSemantics: true,
      child: FadeTransition(
        opacity: _fade,
        child: Center(
          child: SizedBox(
            width: widget.size,
            height: widget.size,
            child: AnimatedBuilder(
              animation: _spin,
              builder: (context, _) {
                return CustomPaint(
                painter: _LiquidSpinnerPainter(
                  progress: _spin.value,
                  color: c,
                ),
              );
            },
          ),
        ),
      ),
      ),
    );
  }
}

/// 液态加载弧：带渐变拖尾的圆弧，比平笔画的旋转圈更柔和。
class _LiquidSpinnerPainter extends CustomPainter {
  final double progress;
  final Color color;
  _LiquidSpinnerPainter({required this.progress, required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2 - 3;
    // 270 度弧，带渐变
    final startAngle = progress * 6.2832;
    final sweep = 4.712; // ~270 度
    final rect = Rect.fromCircle(center: center, radius: radius);
    final paint = Paint()
      ..shader = SweepGradient(
        startAngle: startAngle,
        endAngle: startAngle + sweep,
        colors: [
          color.withValues(alpha: 0),
          color.withValues(alpha: 0.4),
          color,
        ],
        stops: const [0.0, 0.6, 1.0],
        transform: GradientRotation(startAngle),
      ).createShader(rect)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2.5
      ..strokeCap = StrokeCap.round;
    canvas.drawArc(rect, startAngle, sweep, false, paint);
  }

  @override
  bool shouldRepaint(covariant _LiquidSpinnerPainter old) =>
      old.progress != progress || old.color != color;
}

/// A lightweight skeleton shimmer block for loading placeholders.
/// Pulses with a soft gradient sweep instead of a flat grey box.
class PTShimmerBlock extends StatefulWidget {
  final double width;
  final double height;
  final double radius;

  const PTShimmerBlock({
    super.key,
    this.width = double.infinity,
    required this.height,
    this.radius = PTRadius.sm,
  });

  @override
  State<PTShimmerBlock> createState() => _PTShimmerBlockState();
}

class _PTShimmerBlockState extends State<PTShimmerBlock>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    )..repeat();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final base = isDark
        ? Colors.white.withValues(alpha: 0.06)
        : Colors.black.withValues(alpha: 0.06);
    final highlight = isDark
        ? Colors.white.withValues(alpha: 0.14)
        : Colors.white.withValues(alpha: 0.70);
    return AnimatedBuilder(
      animation: _ctrl,
      builder: (context, child) {
        return Container(
          width: widget.width,
          height: widget.height,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(widget.radius),
            gradient: LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: [base, highlight, base],
              stops: [
                (_ctrl.value - 0.3).clamp(0.0, 1.0),
                _ctrl.value,
                (_ctrl.value + 0.3).clamp(0.0, 1.0),
              ],
            ),
          ),
        );
      },
    );
  }
}
