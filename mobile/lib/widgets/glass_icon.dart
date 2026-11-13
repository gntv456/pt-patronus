// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:flutter_svg/flutter_svg.dart';
import '../theme/glass.dart';
import 'glyphs.dart';

/// 单色线性应用图标 —— 与 Web 端 SptIcon / Lucide 同一图标族：
/// 浅主色软底 + 1px 边框 + 8px 圆角 + currentColor（语义主色）描边字形。
/// 无 135° 渐变 / 内高光 rim / 外发光 / squircle。
///
/// 用法：导航项、品牌位、指标卡图标。`size` 默认 30（导航项），字形按 60% 居中。
/// 兼容旧调用：保留 `from` / `to` 参数名（旧渐变两色），但不再参与渲染，
/// 避免 20+ 处调用点改动；字形颜色统一随主题主色。
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
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final glyphColor = Theme.of(context).colorScheme.primary;
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: isDark ? PTColors.primarySoftDark : PTColors.primarySoft,
        borderRadius: BorderRadius.circular(PTRadius.lg),
        border: Border.all(
          color: isDark ? PTColors.borderStrongDark : PTColors.border,
        ),
      ),
      child: SvgPicture.string(
        glyphSvg(glyph),
        width: size * 0.60,
        height: size * 0.60,
        // glyphSvg 把描边烧成 #fff，用 colorFilter 统一染成语义主色。
        colorFilter: ColorFilter.mode(glyphColor, BlendMode.srcIn),
      ),
    );
  }
}
