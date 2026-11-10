// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 自适应表格：宽屏多列表格 / 窄屏卡片（从 adaptive.dart 拆出，part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

class _WideTable<T> extends StatelessWidget {
  final List<T> items;
  final List<PTColumn<T>> columns;
  final double? minTableWidth;
  final VoidCallback? onRowTap;
  const _WideTable({
    required this.items,
    required this.columns,
    this.minTableWidth,
    this.onRowTap,
  });

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final headerColor = const Color(0xFF6C5CE7).withValues(alpha: 0.10);
    final dividerColor = isDark
        ? Colors.white.withValues(alpha: 0.08)
        : Colors.black.withValues(alpha: 0.08);

    Widget headerRow = Row(
      children: columns
          .map(
            (c) => Expanded(
              flex: c.flex,
              child: Padding(
                padding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 10,
                ),
                child: Text(
                  c.label,
                  style: TextStyle(
                    fontSize: PTFs.sm,
                    fontWeight: FontWeight.w700,
                    color: const Color(0xFF6C5CE7),
                  ),
                  textAlign: c.numeric ? TextAlign.right : TextAlign.left,
                ),
              ),
            ),
          )
          .toList(),
    );

    Widget row(T item, bool zebra) {
      final cells = Row(
        children: columns
            .map(
              (c) => Expanded(
                flex: c.flex,
                child: Align(
                  alignment: c.numeric
                      ? Alignment.centerRight
                      : Alignment.centerLeft,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 10,
                      vertical: 8,
                    ),
                    child: c.cell(context, item),
                  ),
                ),
              ),
            )
            .toList(),
      );
      Widget content = DecoratedBox(
        decoration: BoxDecoration(
          color: zebra
              ? (isDark
                    ? Colors.white.withValues(alpha: 0.03)
                    : Colors.black.withValues(alpha: 0.02))
              : null,
          border: Border(bottom: BorderSide(color: dividerColor, width: 0.6)),
        ),
        child: cells,
      );
      if (onRowTap != null) {
        content = InkWell(onTap: onRowTap, child: content);
      }
      return content;
    }

    final table = Column(
      children: [
        DecoratedBox(
          decoration: BoxDecoration(color: headerColor),
          child: headerRow,
        ),
        ...items.asMap().entries.map((e) => row(e.value, e.key.isOdd)),
      ],
    );

    Widget panel = GlassCard(
      padding: EdgeInsets.zero,
      child: minTableWidth != null
          ? SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: ConstrainedBox(
                constraints: BoxConstraints(minWidth: minTableWidth!),
                child: table,
              ),
            )
          : table,
    );

    return SingleChildScrollView(
      padding: const EdgeInsets.all(12),
      child: panel,
    );
  }
}

/// 自适应列表：宽屏渲染对齐的数据表格（Web el-table 范式），窄屏渲染卡片列表。
///
/// 收敛 Sites / Search 种子 / Dashboard 站点明细 / Reseed / Harem / Downloaders 等
/// 「Web=表格 / Flutter=纵向卡片」的最大视觉差异。表格行按 [columns] 的 flex 比例对齐；
/// 当 [minTableWidth] 非空且容器更窄时，整表横向滚动（等价 el-table 溢出滚动）。
class PTAdaptiveTable<T> extends StatelessWidget {
  final List<T> items;
  final List<PTColumn<T>> columns;
  final Widget Function(BuildContext, T) narrowCard;
  final VoidCallback? onRowTap;
  final double? minTableWidth;
  final String emptyText;

  /// 内嵌于外层滚动视图时置 true：窄屏 `ListView.builder` 改 `shrinkWrap` + 禁用滚动 +
  /// 零 padding，作为区段由外层 ListView 统一滚动。避免同向 ListView 嵌套导致的无界高度塌缩
  /// （dashboard 站点明细曾因此空白）。作滚动根时（sites/harem）保持默认 false。
  final bool embedded;

  const PTAdaptiveTable({
    super.key,
    required this.items,
    required this.columns,
    required this.narrowCard,
    this.onRowTap,
    this.minTableWidth,
    this.emptyText = '暂无数据',
    this.embedded = false,
  });

  @override
  Widget build(BuildContext context) {
    if (!isWideScreen(context)) {
      return items.isEmpty
          ? PTEmptyState(text: emptyText)
          : ListView.builder(
              shrinkWrap: embedded,
              physics: embedded ? const NeverScrollableScrollPhysics() : null,
              padding: embedded ? EdgeInsets.zero : const EdgeInsets.all(12),
              itemCount: items.length,
              itemBuilder: (ctx, i) => PTListItemEntrance(
                index: i,
                child: Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: narrowCard(ctx, items[i]),
                ),
              ),
            );
    }
    if (items.isEmpty) return PTEmptyState(text: emptyText);
    return _WideTable(
      items: items,
      columns: columns,
      minTableWidth: minTableWidth,
      onRowTap: onRowTap,
    );
  }
}
