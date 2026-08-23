import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';

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
          child: PTEntrance(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  PTBreathIcon(
                    icon: icon,
                    size: 56,
                    color: Theme.of(context).colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
                  ),
                const SizedBox(height: 14),
                Text(
                  main,
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: PTFs.lg,
                    fontWeight: FontWeight.w600,
                    color: isDark ? Colors.white70 : context.dimText,
                  ),
                ),
                if (description != null && description!.isNotEmpty) ...[
                  const SizedBox(height: 6),
                  Text(
                    description!,
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: PTFs.body,
                      color: isDark ? Colors.white38 : Colors.black38,
                    ),
                  ),
                ],
                if (actionLabel != null && onAction != null) ...[
                  const SizedBox(height: 18),
                  FilledButton.icon(
                    onPressed: onAction,
                    icon: Icon(actionIcon),
                    label: Text(actionLabel!),
                  ),
                ] else if (onRetry != null) ...[
                  const SizedBox(height: 18),
                  OutlinedButton(onPressed: onRetry, child: const Text('重试')),
                ],
              ],
            ),
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
  const _PTWideDialog({
    this.title,
    required this.maxWidth,
    required this.dismissible,
    required this.child,
  });

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
                child: Row(
                  children: [
                    Expanded(
                      child: Text(
                        title!,
                        style: TextStyle(
                          fontSize: PTFs.xl,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ),
                    IconButton(
                      visualDensity: VisualDensity.compact,
                      icon: const Icon(Icons.close),
                      onPressed: () => Navigator.of(context).maybePop(),
                    ),
                  ],
                ),
              ),
            Flexible(child: SingleChildScrollView(child: child)),
          ],
        ),
      ),
    );
    return Center(
      child: Padding(padding: const EdgeInsets.all(24), child: panel),
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
        padding: EdgeInsets.fromLTRB(
          16,
          8,
          16,
          16 + MediaQuery.viewInsetsOf(context).bottom,
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (title != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 8, top: 4),
                child: Text(
                  title!,
                  style: TextStyle(
                    fontSize: PTFs.xl,
                    fontWeight: FontWeight.w700,
                  ),
                ),
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
///
/// 已废弃：全站 23 个导航页统一改用 [PTPageScaffold] + [PTBarAction]
/// （widgets/pt_page_scaffold.dart）。保留过渡期兼容，后续删除。
@Deprecated('已由 PTBarAction + PTPageScaffold 取代')
class PTPageHeaderActions extends StatelessWidget {
  final List<PTHeaderAction> actions;
  const PTPageHeaderActions({super.key, required this.actions});

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: actions
          .map(
            (a) => wide
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
                    iconSize: 20,
                    padding: EdgeInsets.zero,
                    constraints: const BoxConstraints(
                      minWidth: 44,
                      minHeight: 44,
                    ),
                  ),
          )
          .toList(),
    );
  }
}

@Deprecated('已由 PTBarAction 取代')
class PTHeaderAction {
  final IconData icon;
  final String label;
  final VoidCallback? onPressed;
  const PTHeaderAction({
    required this.icon,
    required this.label,
    this.onPressed,
  });
}

/// 顶栏图标按钮：44×44 触摸区（Material/Apple HIG 最小红线）+ 20px 图标，替代散落的裸 `IconButton`。
/// 用于各导航页 header 的 actions（刷新 / 添加 / 导出 / 更多等）。`loading` 时显示小 spinner
/// （替代各页手写的 `CircularProgressIndicator(strokeWidth:2)` 刷新态），loading 态也占 44×44 防跳动。
/// 与全局 appBarTheme.toolbarHeight(44) 对齐——44×44 正好填满 44 高工具栏的 actions 区。
class PTIconAction extends StatelessWidget {
  final IconData icon;
  final VoidCallback? onPressed;
  final String? tooltip;
  final bool loading;
  const PTIconAction({
    super.key,
    required this.icon,
    this.onPressed,
    this.tooltip,
    this.loading = false,
  });

  @override
  Widget build(BuildContext context) {
    if (loading) {
      // loading 态也占满 44×44，与图标态等高，避免切换时工具栏跳动。
      return const SizedBox(
        width: 44,
        height: 44,
        child: Center(
          child: SizedBox(
            width: 20,
            height: 20,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
        ),
      );
    }
    return IconButton(
      icon: Icon(icon),
      iconSize: 20,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
      tooltip: tooltip,
      onPressed: onPressed,
    );
  }
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

// ─────────────────────────────────────────────────────────────────────────
// 筛选弹窗化原子：很小的「药丸」触发器 + 候选列表弹窗。
// 把常驻筛选 chip 行收进弹窗——顶部只留很小的 PTFilterPill，点按弹 showFilterSheet
// 候选列表。比 PTChipBar 更省竖向空间（一行小药丸），符合移动端「筛选藏进弹窗」。
// ─────────────────────────────────────────────────────────────────────────

/// 很小的筛选触发器药丸。两种用法：
/// - 选择型：[onTap] 由调用方打开 `showFilterSheet`，显示 `label value ▾`。
/// - 开关型：[onTap] 直接切换（二元），[active] 控制高亮（如「近期站庆」）。
class PTFilterPill extends StatelessWidget {
  final String value; // 当前值文本（如 "正在站免"）
  final String? label; // 维度名前缀（如 "状态"），可空
  final VoidCallback onTap;
  final bool active; // 非默认值时 true → 主色高亮
  final IconData? icon;

  const PTFilterPill({
    super.key,
    required this.value,
    required this.onTap,
    this.label,
    this.active = false,
    this.icon,
  });

  @override
  Widget build(BuildContext context) {
    final c = PTColors.primary;
    final dim = active ? c : Theme.of(context).colorScheme.onSurfaceVariant;
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        constraints: const BoxConstraints(minHeight: 36),
        decoration: BoxDecoration(
          color: active ? c.withValues(alpha: 0.14) : Colors.transparent,
          border: Border.all(
            color: active
                ? c.withValues(alpha: 0.5)
                : Theme.of(context).colorScheme.onSurfaceVariant.withValues(alpha: 0.4),
          ),
          borderRadius: BorderRadius.circular(999),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (icon != null) ...[
              Icon(icon, size: 15, color: dim),
              const SizedBox(width: 4),
            ],
            if (label != null) ...[
              Text(label!, style: TextStyle(fontSize: 12, color: dim)),
              const SizedBox(width: 4),
            ],
            Text(
              value,
              style: TextStyle(
                fontSize: 13,
                fontWeight: active ? FontWeight.w700 : FontWeight.w500,
                color: active ? c : Theme.of(context).colorScheme.onSurface,
              ),
            ),
            Icon(Icons.arrow_drop_down, size: 18, color: dim),
          ],
        ),
      ),
    );
  }
}

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
