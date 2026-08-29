import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/glass.dart';

// Unified motion system for the Liquid Glass design language.
// Curves and durations are tuned to feel "watery and fluid" (水润灵动):
// soft springs for interactive elements, gentle deceleration for transitions.

/// Motion timing tokens shared app-wide.
class PTMotion {
  PTMotion._();

  /// Snappy-but-soft curve for card taps, toggles, chips.
  static const Curve soft = Curves.easeOutCubic;

  /// Fluid entrance curve for page/list reveals.
  static const Curve fluid = Curves.easeOutQuart;

  /// Playful spring used for press/scale feedback (slight overshoot).
  static const Curve spring = Curves.easeOutBack;

  static const Duration fast = Duration(milliseconds: 180);
  static const Duration normal = Duration(milliseconds: 300);
  static const Duration slow = Duration(milliseconds: 480);
}

/// Staggered fade + slide-up entrance for list/grid items.
///
/// Wrap each item with [index] (its position). Items animate in sequence
/// with a small delay per row, capped so long lists don't over-delay.
/// It only plays once on first appearance — scroll-off/on won't re-trigger.
class PTListItemEntrance extends StatefulWidget {
  final int index;
  final Widget child;
  final bool enabled;

  const PTListItemEntrance({
    super.key,
    required this.index,
    required this.child,
    this.enabled = true,
  });

  @override
  State<PTListItemEntrance> createState() => _PTListItemEntranceState();
}

class _PTListItemEntranceState extends State<PTListItemEntrance>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;
  late final Animation<double> _fade;
  late final Animation<Offset> _slide;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: PTMotion.slow,
    );
    final delay = (widget.index.clamp(0, 8) * 0.05).clamp(0.0, 0.35);
    final curved = CurvedAnimation(
      parent: _ctrl,
      curve: Interval(delay, 1.0, curve: PTMotion.fluid),
    );
    _fade = Tween(begin: 0.0, end: 1.0).animate(curved);
    _slide = Tween<Offset>(
      begin: const Offset(0, 0.04),
      end: Offset.zero,
    ).animate(curved);
    if (widget.enabled) {
      _ctrl.forward();
    } else {
      _ctrl.value = 1.0;
    }
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!widget.enabled) return widget.child;
    return FadeTransition(
      opacity: _fade,
      child: SlideTransition(position: _slide, child: widget.child),
    );
  }
}

/// Press-scale feedback for cards/buttons that feel "tap into liquid".
/// Combines an InkWell ripple with a subtle scale-down (0.97) on press.
class PTPressScale extends StatefulWidget {
  final Widget child;
  final VoidCallback? onTap;
  final double scaleDown;

  const PTPressScale({
    super.key,
    required this.child,
    this.onTap,
    this.scaleDown = 0.97,
  });

  @override
  State<PTPressScale> createState() => _PTPressScaleState();
}

class _PTPressScaleState extends State<PTPressScale>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;
  late final Animation<double> _scale;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: PTMotion.fast,
      reverseDuration: PTMotion.normal,
    );
    _scale = Tween<double>(begin: 1.0, end: widget.scaleDown).animate(
      CurvedAnimation(parent: _ctrl, curve: PTMotion.soft),
    );
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.translucent,
      onTapDown: (_) => _ctrl.forward(),
      onTapUp: (_) {
        _ctrl.reverse();
        widget.onTap?.call();
      },
      onTapCancel: () => _ctrl.reverse(),
      child: ScaleTransition(scale: _scale, child: widget.child),
    );
  }
}

/// Animated entrance wrapper — fades + scales in once on mount.
/// Used for empty states, hero images, and centered messaging.
class PTEntrance extends StatefulWidget {
  final Widget child;
  final Duration duration;

  const PTEntrance({
    super.key,
    required this.child,
    this.duration = PTMotion.slow,
  });

  @override
  State<PTEntrance> createState() => _PTEntranceState();
}

class _PTEntranceState extends State<PTEntrance>
    with SingleTickerProviderStateMixin {
  late final AnimationController _ctrl;
  late final Animation<double> _fade;
  late final Animation<double> _scale;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(vsync: this, duration: widget.duration);
    final curved =
        CurvedAnimation(parent: _ctrl, curve: PTMotion.fluid);
    _fade = Tween(begin: 0.0, end: 1.0).animate(curved);
    _scale = Tween(begin: 0.94, end: 1.0).animate(curved);
    _ctrl.forward();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return FadeTransition(
      opacity: _fade,
      child: ScaleTransition(scale: _scale, child: widget.child),
    );
  }
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


/// 柔和的呼吸图标：无限循环的轻微缩放（1.0 ↔ 1.06）+ 透明度脉动。
/// 用于空状态、引导提示等，让静态画面有「呼吸感」而非死板。
class PTBreathIcon extends StatefulWidget {
  final IconData icon;
  final double size;
  final Color? color;
  const PTBreathIcon({
    super.key,
    required this.icon,
    required this.size,
    this.color,
  });
  @override
  State<PTBreathIcon> createState() => _PTBreathIconState();
}

class _PTBreathIconState extends State<PTBreathIcon>
    with TickerProviderStateMixin {
  late final AnimationController _ctrl;

  @override
  void initState() {
    super.initState();
    _ctrl = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 2400),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _ctrl,
      builder: (context, child) {
        final t = Curves.easeInOut.transform(_ctrl.value);
        return Opacity(
          opacity: 0.55 + 0.45 * t,
          child: Transform.scale(
            scale: 1.0 + 0.06 * t,
            child: child,
          ),
        );
      },
      child: Icon(
        widget.icon,
        size: widget.size,
        color: widget.color,
      ),
    );
  }
}


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