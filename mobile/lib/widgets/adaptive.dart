import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';

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
          itemBuilder: itemBuilder,
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
        final cols = ptAutoColumns(usable, minItemWidth, minColumns: minColumns);
        final itemW = cols <= 1 ? usable : (usable - spacing * (cols - 1)) / cols;
        return SingleChildScrollView(
          padding: pad,
          child: Wrap(
            spacing: spacing,
            runSpacing: spacing,
            children: items
                .map((it) => SizedBox(width: itemW, child: itemBuilder(context, it)))
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

  const PTAdaptiveTable({
    super.key,
    required this.items,
    required this.columns,
    required this.narrowCard,
    this.onRowTap,
    this.minTableWidth,
    this.emptyText = '暂无数据',
  });

  @override
  Widget build(BuildContext context) {
    if (!isWideScreen(context)) {
      return items.isEmpty
          ? PTEmptyState(text: emptyText)
          : ListView.builder(
              padding: const EdgeInsets.all(12),
              itemCount: items.length,
              itemBuilder: (ctx, i) => Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: narrowCard(ctx, items[i]),
              ),
            );
    }
    if (items.isEmpty) return PTEmptyState(text: emptyText);
    return _WideTable(items: items, columns: columns, minTableWidth: minTableWidth, onRowTap: onRowTap);
  }
}

class _WideTable<T> extends StatelessWidget {
  final List<T> items;
  final List<PTColumn<T>> columns;
  final double? minTableWidth;
  final VoidCallback? onRowTap;
  const _WideTable({required this.items, required this.columns, this.minTableWidth, this.onRowTap});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final headerColor = const Color(0xFF6C5CE7).withValues(alpha: 0.10);
    final dividerColor = isDark ? Colors.white.withValues(alpha: 0.08) : Colors.black.withValues(alpha: 0.08);

    Widget headerRow = Row(
      children: columns
          .map((c) => Expanded(
                flex: c.flex,
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
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
              ))
          .toList(),
    );

    Widget row(T item, bool zebra) {
      final cells = Row(
        children: columns
            .map((c) => Expanded(
                  flex: c.flex,
                  child: Align(
                    alignment: c.numeric ? Alignment.centerRight : Alignment.centerLeft,
                    child: Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                      child: c.cell(context, item),
                    ),
                  ),
                ))
            .toList(),
      );
      Widget content = DecoratedBox(
        decoration: BoxDecoration(
          color: zebra ? (isDark ? Colors.white.withValues(alpha: 0.03) : Colors.black.withValues(alpha: 0.02)) : null,
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
        DecoratedBox(decoration: BoxDecoration(color: headerColor), child: headerRow),
        ...items.asMap().entries.map((e) => row(e.value, e.key.isOdd)),
      ],
    );

    Widget panel = GlassCard(
      padding: EdgeInsets.zero,
      child: minTableWidth != null
          ? SingleChildScrollView(scrollDirection: Axis.horizontal, child: ConstrainedBox(constraints: BoxConstraints(minWidth: minTableWidth!), child: table))
          : table,
    );

    return SingleChildScrollView(
      padding: const EdgeInsets.all(12),
      child: panel,
    );
  }
}

/// 统一空状态：图标 + 标题 + 说明 + 可选动作。
///
/// 取代各页散落的裸 `Center(Text(...))`（约 12 页），对齐 Web 的 `EmptyState` 组件。
/// 渲染为可滚动 ListView，保证作为 `RefreshIndicator` 子件时空列表仍可下拉刷新。
class PTEmptyState extends StatelessWidget {
  final IconData icon;
  final String? title;
  final String? text; // 兼容仅一行文案的旧用法
  final String? description;
  final String? actionLabel;
  final VoidCallback? onAction;
  final VoidCallback? onRetry;
  final IconData actionIcon; // actionLabel 按钮图标（默认 add；原硬编码 refresh 语义错——审计 P2）

  const PTEmptyState({
    super.key,
    this.icon = Icons.inbox_outlined,
    this.title,
    this.text,
    this.description,
    this.actionLabel,
    this.onAction,
    this.onRetry,
    this.actionIcon = Icons.add,
  });

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final main = text ?? title ?? '暂无数据';
    return ListView(
      children: [
        const SizedBox(height: 96),
        Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 56, color: (isDark ? Colors.white : Colors.black54).withValues(alpha: 0.35)),
                const SizedBox(height: 14),
                Text(main, textAlign: TextAlign.center, style: TextStyle(fontSize: PTFs.lg, fontWeight: FontWeight.w600, color: isDark ? Colors.white70 : Colors.black54)),
                if (description != null && description!.isNotEmpty) ...[
                  const SizedBox(height: 6),
                  Text(description!, textAlign: TextAlign.center, style: TextStyle(fontSize: PTFs.body, color: isDark ? Colors.white38 : Colors.black38)),
                ],
                if (actionLabel != null && onAction != null) ...[
                  const SizedBox(height: 18),
                  FilledButton.icon(onPressed: onAction, icon: Icon(actionIcon), label: Text(actionLabel!)),
                ] else if (onRetry != null) ...[
                  const SizedBox(height: 18),
                  OutlinedButton(onPressed: onRetry, child: const Text('重试')),
                ],
              ],
            ),
          ),
        ),
      ],
    );
  }
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

class _PTWideDialog extends StatelessWidget {
  final String? title;
  final double maxWidth;
  final bool dismissible;
  final Widget child;
  const _PTWideDialog({this.title, required this.maxWidth, required this.dismissible, required this.child});

  @override
  Widget build(BuildContext context) {
    Widget panel = GlassCard(
      padding: const EdgeInsets.all(20),
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Row(children: [
                  Expanded(child: Text(title!, style: TextStyle(fontSize: PTFs.xl, fontWeight: FontWeight.w700))),
                  IconButton(
                    visualDensity: VisualDensity.compact,
                    icon: const Icon(Icons.close),
                    onPressed: () => Navigator.of(context).maybePop(),
                  ),
                ]),
              ),
            Flexible(child: SingleChildScrollView(child: child)),
          ],
        ),
      ),
    );
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: panel,
      ),
    );
  }
}

class _PTBottomSheet extends StatelessWidget {
  final String? title;
  final Widget child;
  const _PTBottomSheet({this.title, required this.child});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return Container(
      decoration: BoxDecoration(
        color: isDark ? const Color(0xFF1E2024) : Colors.white,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(20)),
      ),
      child: Padding(
        padding: EdgeInsets.fromLTRB(16, 8, 16, 16 + MediaQuery.viewInsetsOf(context).bottom),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 8, top: 4),
                child: Text(title!, style: TextStyle(fontSize: PTFs.xl, fontWeight: FontWeight.w700)),
              ),
            Flexible(child: SingleChildScrollView(child: child)),
          ],
        ),
      ),
    );
  }
}

/// 顶栏页面标题行 + 操作区：宽屏带标签按钮（对齐 Web .head），窄屏图标按钮。
/// 让各页头部在桌面端不再是「纯图标」，与 Web 的「导出图片 / 刷新统计」文本按钮一致。
class PTPageHeaderActions extends StatelessWidget {
  final List<PTHeaderAction> actions;
  const PTPageHeaderActions({super.key, required this.actions});

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: actions
          .map((a) => wide
              ? Padding(
                  padding: const EdgeInsets.only(left: 8),
                  child: FilledButton.tonalIcon(
                    onPressed: a.onPressed,
                    icon: Icon(a.icon, size: 18),
                    label: Text(a.label),
                  ),
                )
              : IconButton(
                  tooltip: a.label,
                  onPressed: a.onPressed,
                  icon: Icon(a.icon),
                ))
          .toList(),
    );
  }
}

class PTHeaderAction {
  final IconData icon;
  final String label;
  final VoidCallback? onPressed;
  const PTHeaderAction({required this.icon, required this.label, this.onPressed});
}
