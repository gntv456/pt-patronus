// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/theme/glass.dart';

part 'pt_motion_spinners.dart';

// Unified motion system for the modern tool-type UI (Minimalism & Swiss).
// 克制、无回弹、无过冲：状态过渡只做 120–240ms 的淡入；
// 所有入场动效都遵循系统「减少动效」偏好，默认不做列表 stagger。

/// Motion timing tokens shared app-wide.
class PTMotion {
  PTMotion._();

  /// 通用减速曲线（无过冲）。
  static const Curve soft = Curves.easeOut;

  /// 入场/退场曲线（无回弹）。
  static const Curve fluid = Curves.easeOut;

  /// 兼容旧调用点：不再使用 spring / overshoot。
  static const Curve spring = Curves.easeOut;

  static const Duration fast = Duration(milliseconds: 120);
  static const Duration normal = Duration(milliseconds: 180);
  static const Duration slow = Duration(milliseconds: 240);
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
    this.enabled = false,
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
    if (!widget.enabled || MediaQuery.disableAnimationsOf(context)) return widget.child;
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
    this.scaleDown = 1.0,
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
    _scale = Tween(begin: 1.0, end: 1.0).animate(curved);
    _ctrl.forward();
  }

  @override
  void dispose() {
    _ctrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (MediaQuery.disableAnimationsOf(context)) return widget.child;
    return FadeTransition(
      opacity: _fade,
      child: ScaleTransition(scale: _scale, child: widget.child),
    );
  }
}

/// 静态状态图标（原「呼吸」无限缩放/透明度循环已移除：
/// 后台工具不需要装饰性循环动效，skill 规定无限动画只用于加载指示器）。
class PTBreathIcon extends StatelessWidget {
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
  Widget build(BuildContext context) {
    return Icon(icon, size: size, color: color);
  }
}

