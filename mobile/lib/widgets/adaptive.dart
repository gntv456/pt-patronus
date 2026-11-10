// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

part 'adaptive_dialogs.dart';
part 'adaptive_chips.dart';
part 'adaptive_table.dart';
part 'adaptive_stats.dart';
part 'adaptive_headers.dart';
part 'adaptive_empty.dart';

// ─────────────────────────────────────────────────────────────────────────
// 自适应布局原子组件：让 Flutter 在宽屏呈现 Web 范式（表格 / 对话框 / 多列网格），
// 窄屏回退为卡片 / 底栏。所有「桌面端像 Web」的差异都通过这几个组件收敛。
// 对齐前端 MainLayout.vue + 各页 CSS auto-fit 网格 / el-table / el-dialog / el-drawer。
// ─────────────────────────────────────────────────────────────────────────

/// 自动填充网格（CSS `repeat(auto-fit, minmax(N,1fr))` 语义）。
///
/// 列数 = floor(可用宽 / [minItemWidth])，随窗口缩放自动重排：宽屏多列、窄屏单列。
/// 替代各页固定 `crossAxisCount` 的 `GridView.count`（无法重排、宽屏浪费空间）。
///
/// 作主滚动区时默认占满并滚动；作内嵌区段（如详情页推荐）传 `shrinkWrap: true`
/// + `physics: NeverScrollableScrollPhysics()` 即可。
class PTAutoGrid extends StatelessWidget {
  final double minItemWidth;
  final int minColumns;
  final double spacing;
  final double runSpacing;
  final double childAspectRatio;
  final int itemCount;
  final Widget Function(BuildContext, int) itemBuilder;
  final EdgeInsets? padding;
  final bool shrinkWrap;
  final ScrollPhysics? physics;

  const PTAutoGrid({
    super.key,
    required this.minItemWidth,
    required this.itemCount,
    required this.itemBuilder,
    this.minColumns = 1,
    this.spacing = 12,
    this.runSpacing = 12,
    this.childAspectRatio = 1,
    this.padding,
    this.shrinkWrap = false,
    this.physics,
  });

  @override
  Widget build(BuildContext context) {
    final EdgeInsets pad = padding ?? EdgeInsets.zero;
    return LayoutBuilder(
      builder: (context, c) {
        final cols = ptAutoColumns(
          c.maxWidth - pad.horizontal,
          minItemWidth,
          minColumns: minColumns,
        );
        return GridView.builder(
          padding: pad,
          shrinkWrap: shrinkWrap,
          physics: physics,
          gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
            crossAxisCount: cols,
            mainAxisSpacing: runSpacing,
            crossAxisSpacing: spacing,
            childAspectRatio: childAspectRatio,
          ),
          itemCount: itemCount,
          itemBuilder: (ctx, i) => PTListItemEntrance(
            index: i,
            child: itemBuilder(ctx, i),
          ),
        );
      },
    );
  }
}

/// 可变高卡片的多列瀑布流（CSS `auto-fill minmax` + 自然行高 语义）。
///
/// 与 [PTAutoGrid] 区别：子项高度不固定（如考核卡含可变 metrics 列表、站免多公告），
/// 固定纵横比的 GridView 会裁剪或留白。[PTMasonry] 用 `Wrap` + 宽度受限子项，
/// 列数随窗口缩放，每行高度取该行最高项，整体作为单一滚动区。
/// 适合 Assessment / Freeleech / Harem / Savept 公告区等「Web 多列卡片、Flutter 高度可变」的页。
class PTMasonry<T> extends StatelessWidget {
  final List<T> items;
  final double minItemWidth;
  final int minColumns;
  final double spacing;
  final EdgeInsets? padding;
  final Widget Function(BuildContext, T) itemBuilder;

  const PTMasonry({
    super.key,
    required this.items,
    required this.minItemWidth,
    required this.itemBuilder,
    this.minColumns = 1,
    this.spacing = 12,
    this.padding,
  });

  @override
  Widget build(BuildContext context) {
    final EdgeInsets pad = padding ?? const EdgeInsets.all(12);
    return LayoutBuilder(
      builder: (context, c) {
        final usable = c.maxWidth - pad.horizontal;
        final cols = ptAutoColumns(
          usable,
          minItemWidth,
          minColumns: minColumns,
        );
        final itemW = cols <= 1
            ? usable
            : (usable - spacing * (cols - 1)) / cols;
        return SingleChildScrollView(
          padding: pad,
          child: Wrap(
            spacing: spacing,
            runSpacing: spacing,
            children: items
                .asMap()
                .entries
                .map(
                  (e) => PTListItemEntrance(
                    index: e.key,
                    child: SizedBox(
                      width: itemW,
                      child: itemBuilder(context, e.value),
                    ),
                  ),
                )
                .toList(),
          ),
        );
      },
    );
  }
}

/// 自适应表格的一列定义。
class PTColumn<T> {
  final String label;
  final int flex;
  final bool numeric;
  final Widget Function(BuildContext, T) cell;
  const PTColumn({
    required this.label,
    required this.cell,
    this.flex = 1,
    this.numeric = false,
  });
}



/// 自适应浮层：宽屏 `showDialog`（居中玻璃面板，等价 Web el-dialog），
/// 窄屏 `showModalBottomSheet`（等价 Web 窄屏 / 移动端底栏）。
///
/// [builder] 收到的 widget 直接作为内容（不含外层面板）；本函数套上对应容器。
/// [wideWidth] 限制宽屏对话框最大宽度（默认 560，对齐 Web 常用 el-dialog 宽）。
Future<T?> showAdaptiveSheet<T>(
  BuildContext context, {
  required WidgetBuilder builder,
  double wideWidth = 560,
  String? title,
  bool dismissible = true,
}) {
  if (isWideScreen(context)) {
    return showDialog<T>(
      context: context,
      barrierDismissible: dismissible,
      builder: (ctx) => _PTWideDialog(
        title: title,
        maxWidth: wideWidth,
        dismissible: dismissible,
        child: builder(ctx),
      ),
    );
  }
  return showModalBottomSheet<T>(
    context: context,
    isScrollControlled: true,
    useSafeArea: true,
    showDragHandle: true,
    backgroundColor: Colors.transparent,
    builder: (ctx) => _PTBottomSheet(title: title, child: builder(ctx)),
  );
}

// ─────────────────────────────────────────────────────────────────────────
// 紧凑头部原子：单行横向滚动 chip 条 / 指标条。
// 替代各页散落的 `SizedBox(h:38/40)+ListView+GestureDetector+Container` chip 模板，
// 以及监护室 vitals / 后宫概览那种「大数值+小标签」放进 Wrap 折行的双行瓦片。
// 共同点：永不换行（横向滚动），把窄屏首屏被「标题 + 筛选」吃掉的竖向空间压回来。
// ─────────────────────────────────────────────────────────────────────────

/// [PTChipBar] 的一项。
class PTChipItem<T> {
  final T value;
  final String label;
  final Color? color; // 选中态着色（底 alpha.18 + 边 alpha.4）；空则用 PTColors.primary
  const PTChipItem({required this.value, required this.label, this.color});
}

/// 单行横向滚动筛选条（单选）。收敛各页重复手写的 chip 模板。
///
/// 高度默认 34（比旧散落实现的 38/40 更紧），pill 样式（radius 999），
/// 选中项用 [PTChipItem.color] 着色。横向滚动永不换行——窄屏省竖向空间。
/// 用于站免 / 考核 / 推荐(源·分类) / 勋章墙(状态) / 消息(邮箱 tab) 等筛选条。
class PTChipBar<T> extends StatelessWidget {
  final List<PTChipItem<T>> items;
  final T selected;
  final ValueChanged<T> onSelected;
  final double height;
  final EdgeInsets padding;

  const PTChipBar({
    super.key,
    required this.items,
    required this.selected,
    required this.onSelected,
    this.height = 34,
    this.padding = const EdgeInsets.symmetric(horizontal: 12),
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: height,
      child: ListView(
        scrollDirection: Axis.horizontal,
        padding: padding,
        children: items.map((it) {
          final on = it.value == selected;
          final c = it.color ?? PTColors.primary;
          return Padding(
            padding: const EdgeInsets.only(right: 8),
            child: GestureDetector(
              onTap: () => onSelected(it.value),
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 6,
                ),
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: on ? c.withValues(alpha: 0.18) : Colors.transparent,
                  border: Border.all(
                    color: on
                        ? c.withValues(alpha: 0.4)
                        : Theme.of(context).colorScheme.onSurfaceVariant.withValues(alpha: 0.35),
                  ),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  it.label,
                  style: TextStyle(
                    fontSize: PTFs.body,
                    fontWeight: on ? FontWeight.w600 : FontWeight.w500,
                    color: on ? c : Theme.of(context).colorScheme.onSurfaceVariant,
                  ),
                ),
              ),
            ),
          );
        }).toList(),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────
// 筛选弹窗化原子：很小的「药丸」触发器 + 候选列表弹窗。
// 把常驻筛选 chip 行收进弹窗——顶部只留很小的 PTFilterPill，点按弹 showFilterSheet
// 候选列表。比 PTChipBar 更省竖向空间（一行小药丸），符合移动端「筛选藏进弹窗」。
// ─────────────────────────────────────────────────────────────────────────

/// [showFilterSheet] 的一个候选项。
class PTFilterOption<T> {
  final T value;
  final String label;
  final int? count; // 非空时右侧显示计数（如某状态条数）
  const PTFilterOption({required this.value, required this.label, this.count});
}

/// 单选候选列表弹窗：复用 [showAdaptiveSheet]（窄屏底栏 / 宽屏对话框）。
/// 一列 ListTile，选中项打勾，点按即选即关。
/// 用回调式（而非返回值）：[onSelected] 仅在用户点选某项时触发——即便该项 value 为 null
/// （如「全部」）也会回调；点外部关闭则不回调。从而把「选了『全部』(null)」与「取消」
/// 彻底区分开，避免 null 双义。选项多（站点/年份动态）时由外层 SingleChildScrollView 滚动。
void showFilterSheet<T>(
  BuildContext context, {
  required String title,
  required List<PTFilterOption<T>> options,
  required T selected,
  required ValueChanged<T> onSelected,
  double wideWidth = 420,
}) {
  showAdaptiveSheet(
    context,
    title: title,
    wideWidth: wideWidth,
    builder: (ctx) => Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final o in options)
          ListTile(
            dense: true,
            contentPadding: const EdgeInsets.symmetric(horizontal: 8),
            title: Text(o.label, style: const TextStyle(fontSize: 14)),
            trailing: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (o.count != null)
                  Text(
                    '${o.count}',
                    style: TextStyle(fontSize: 12, color: context.dimText),
                  ),
                if (o.value == selected) ...[
                  if (o.count != null) const SizedBox(width: 8),
                  const Icon(Icons.check, color: PTColors.primary, size: 20),
                ],
              ],
            ),
            onTap: () {
              Navigator.pop(ctx);
              onSelected(o.value);
            },
          ),
      ],
    ),
  );
}
