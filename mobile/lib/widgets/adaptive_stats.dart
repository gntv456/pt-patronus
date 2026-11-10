// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 adaptive.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

/// [PTStatStrip] 的一项。可选 [onTap]：点击触发筛选；[selected]：高亮表示当前激活。
class PTStat {
  final String value;
  final String label;
  final Color? color;
  final VoidCallback? onTap;
  final bool selected;
  const PTStat({
    required this.value,
    required this.label,
    this.color,
    this.onTap,
    this.selected = false,
  });
}

/// 单行指标条（纯展示，或每格可点筛选）。替代监护室 vitals / 后宫概览的折行双行瓦片。
///
/// 每格 `value(bold,color) + label(grey,xs)` 紧凑排列：
/// - 宽屏（≥1000）：[Row] 均匀分布 + 可选竖分隔线（监护仪风格），填满宽度不滚动；
/// - 窄屏：横向滚动，永不折行（取代旧 Wrap 折 2-4 行）。
/// - 某项 [PTStat.onTap] 非空时该格可点（InkWell）；[PTStat.selected] 时主色高亮底。
class PTStatStrip extends StatelessWidget {
  final List<PTStat> items;
  final double height;
  final bool dividers; // 宽屏时格间加竖分隔线（监护仪 spt-vital__divider 风格）
  final EdgeInsets padding;

  const PTStatStrip({
    super.key,
    required this.items,
    this.height = 40,
    this.dividers = false,
    this.padding = const EdgeInsets.symmetric(horizontal: 4),
  });

  Widget _cell(BuildContext context, PTStat s) {
    final valueColor = s.selected ? PTColors.primary : s.color;
    final labelColor = s.selected ? PTColors.primary : Theme.of(context).colorScheme.onSurfaceVariant;
    Widget cell = Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(
            s.value,
            style: TextStyle(
              fontSize: PTFs.lg,
              fontWeight: FontWeight.w700,
              color: valueColor,
            ),
          ),
          const SizedBox(height: 2),
          Text(
            s.label,
            style: TextStyle(fontSize: PTFs.xs, color: labelColor),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
    if (s.selected) {
      cell = Container(
        decoration: BoxDecoration(
          color: PTColors.primary.withValues(alpha: 0.12),
          borderRadius: BorderRadius.circular(8),
        ),
        child: cell,
      );
    }
    if (s.onTap != null) {
      return InkWell(
        onTap: s.onTap,
        borderRadius: BorderRadius.circular(8),
        child: cell,
      );
    }
    return cell;
  }

  @override
  Widget build(BuildContext context) {
    final cells = <Widget>[];
    for (var i = 0; i < items.length; i++) {
      if (i > 0 && dividers) {
        cells.add(
          Container(
            width: 1,
            height: height * 0.6,
            margin: const EdgeInsets.symmetric(horizontal: 4),
            color: Theme.of(context).dividerColor.withValues(alpha: 0.25),
          ),
        );
      }
      cells.add(_cell(context, items[i]));
    }
    // 宽屏：Row 均匀分布（填满宽度）+ 可选竖分隔；窄屏：横向滚动不折行。
    if (isWideScreen(context)) {
      return SizedBox(
        height: height,
        child: Padding(
          padding: padding,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceAround,
            children: cells,
          ),
        ),
      );
    }
    return SizedBox(
      height: height,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: padding,
        children: cells,
      ),
    );
  }
}
