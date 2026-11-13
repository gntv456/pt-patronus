// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 pt_page_scaffold.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// 统一顶栏（放进 PTPageScaffold 的 body 顶部 Column，非 Scaffold.appBar）。
/// 这样可在顶栏下方「向下弹出」搜索行。子件 = Column[AppBar, AnimatedSize(搜索行)]。
class PTPageAppBar extends StatefulWidget {
  final Object title; // String（自动套 appBarTheme.titleTextStyle + 省略号）| Widget
  final Widget? leading; // null→自动：wide 隐藏 / narrow HomeMenuButton
  final List<PTBarAction> actions;
  final List<PTFilterDimension> filterDimensions; // 空→无筛选按钮
  final PTSearchConfig? search; // null→无搜索按钮
  final List<Widget>? trailing; // 操作簇之前的额外控件（如 search 页 SegmentedButton）
  final VoidCallback? filterOnClearAll; // 筛选弹窗「清除全部」；不传则用维度兜底重置
  final PTSiteFilterConfig? siteFilter; // 站点筛选（单独按钮，紧凑弹窗，不与 filterDimensions 混）
  final double toolbarHeight;

  const PTPageAppBar({
    super.key,
    required this.title,
    required this.actions,
    this.leading,
    this.filterDimensions = const [],
    this.search,
    this.trailing,
    this.filterOnClearAll,
    this.siteFilter,
    this.toolbarHeight = 48,
  });

  @override
  State<PTPageAppBar> createState() => _PTPageAppBarState();
}

class _PTPageAppBarState extends State<PTPageAppBar> {
  bool _searchOpen = false;

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    final h = widget.toolbarHeight;
    final Widget? leading =
        widget.leading ?? (wide ? null : const HomeMenuButton());
    final showSearch = _searchOpen && widget.search != null;

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        AppBar(
          toolbarHeight: h,
          backgroundColor: Colors.transparent,
          elevation: 0,
          scrolledUnderElevation: 0,
          leading: leading,
          title: _buildTitle(),
          actions: _buildActions(wide),
        ),
        // 搜索行：从顶栏下方「向下弹出」（AnimatedSize 滑出/收起）。
        AnimatedSize(
          alignment: Alignment.topCenter,
          duration: const Duration(milliseconds: 180),
          curve: Curves.easeOut,
          child: showSearch
              ? _searchRow(widget.search!)
              : const SizedBox(width: double.infinity, height: 0),
        ),
      ],
    );
  }

  Widget _buildTitle() {
    if (widget.title is Widget) return widget.title as Widget;
    return Text(
      '${widget.title}',
      overflow: TextOverflow.ellipsis,
      maxLines: 1,
    );
  }

  Widget _searchRow(PTSearchConfig s) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 0, 4, 8),
      child: Row(
        children: [
          Expanded(
            child: TextField(
              controller: s.controller,
              autofocus: s.autofocusOnOpen,
              textInputAction: TextInputAction.search,
              onChanged: s.onChanged,
              onSubmitted: s.onSubmitted,
              style: const TextStyle(fontSize: 15),
              decoration: InputDecoration(
                isDense: true,
                hintText: s.hintText,
                prefixIcon: const Icon(Icons.search, size: 20),
                prefixIconConstraints: const BoxConstraints(minWidth: 36),
                contentPadding: const EdgeInsets.symmetric(vertical: 13),
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(12),
                  borderSide: BorderSide.none,
                ),
                filled: true,
                fillColor: PTColors.primary.withValues(alpha: 0.06),
                suffixIcon: s.controller.text.isNotEmpty
                    ? IconButton(
                        icon: const Icon(Icons.clear, size: 18),
                        tooltip: '清空',
                        onPressed: () {
                          s.controller.clear();
                          s.onChanged('');
                        },
                      )
                    : null,
              ),
            ),
          ),
          IconButton(
            icon: const Icon(Icons.close, size: 20),
            tooltip: '收起',
            onPressed: () => setState(() => _searchOpen = false),
          ),
        ],
      ),
    );
  }

  List<Widget> _buildActions(bool wide) {
    // 1. 拆 inline / overflow；narrow inline 超 2 则尾部进 ⋮。
    List<PTBarAction> inline = widget.actions
        .where((a) => !a.overflow)
        .toList();
    List<PTBarAction> menu = widget.actions.where((a) => a.overflow).toList();
    if (!wide && inline.length > _kMaxInlineActionsNarrow) {
      final pushed = inline.sublist(_kMaxInlineActionsNarrow);
      inline = inline.sublist(0, _kMaxInlineActionsNarrow);
      menu = [...pushed, ...menu];
    }

    final out = <Widget>[];

    // 2. trailing（如 search 页 SegmentedButton）。
    if (widget.trailing != null) out.addAll(widget.trailing!);

    // 3. inline 操作：wide=tonal 文字按钮 / narrow=PTIconAction(44×44)。
    for (final a in inline) {
      out.add(
        wide
            ? FilledButton.tonalIcon(
                onPressed: a.loading ? null : a.onPressed,
                icon: Icon(a.icon, size: 18),
                label: Text(a.label),
              )
            : PTIconAction(
                icon: a.icon,
                tooltip: a.label,
                loading: a.loading,
                onPressed: a.onPressed,
              ),
      );
    }

    // 4. ⋮ 溢出菜单（含 overflow 标记 + narrow 挤进来的）。
    if (menu.isNotEmpty) {
      out.add(_overflowMenu(menu, wide));
    }

    // 5. 站点筛选按钮（单独按钮 + 紧凑弹窗，不与 filterDimensions 混）。
    if (widget.siteFilter != null) {
      out.add(_siteButton(context, widget.siteFilter!));
    }

    // 6. 通用筛选按钮（filterDimensions 非空才渲染）。
    if (widget.filterDimensions.isNotEmpty) {
      out.add(_filterButton());
    }

    // 7. 搜索按钮（宽窄屏统一）：点按 → 顶栏下方弹出搜索行。
    if (widget.search != null) {
      final active = widget.search!.controller.text.isNotEmpty;
      out.add(
        IconButton(
          tooltip: '搜索',
          iconSize: 20,
          padding: EdgeInsets.zero,
          constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
          onPressed: () => setState(() => _searchOpen = !_searchOpen),
          icon: Badge(
            isLabelVisible: active,
            backgroundColor: PTColors.primary,
            child: Icon(
              Icons.search,
              color: _searchOpen ? PTColors.primary : null,
            ),
          ),
        ),
      );
    }

    // 8. AI 助手常驻入口：侧栏/抽屉已隐藏该项（NavItem.hidden），统一进顶栏（对齐
    //    Web MainLayout 顶栏 🤖）。当前已在 AI 页则不重复显示（该页工具栏自有新对话/设置）。
    //    通过 HomeShellScope（leaf，仅 import material）拿控制器，避免 widget 层反向导入
    //    home_nav_items 造成循环（nav_items → 各页面 → PTPageScaffold）。
    final scope = context.dependOnInheritedWidgetOfExactType<HomeShellScope>();
    if (scope != null) {
      final ai = scope.controller.assistantIndex;
      if (ai != null && ai >= 0 && scope.controller.index != ai) {
        out.add(
          IconButton(
            tooltip: 'AI 助手',
            icon: const Icon(Icons.smart_toy),
            iconSize: 22,
            padding: EdgeInsets.zero,
            constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
            onPressed: () => scope.controller.select(ai),
          ),
        );
      }
    }

    return out;
  }

  Widget _overflowMenu(List<PTBarAction> menu, bool wide) {
    return PopupMenuButton<int>(
      icon: const Icon(Icons.more_vert),
      iconSize: 20,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
      tooltip: '更多',
      onSelected: (i) => menu[i].onPressed?.call(),
      itemBuilder: (_) => [
        for (int i = 0; i < menu.length; i++)
          PopupMenuItem<int>(
            value: i,
            enabled: menu[i].onPressed != null,
            child: Row(
              children: [
                Icon(menu[i].icon, size: 18),
                const SizedBox(width: 8),
                Text(menu[i].label),
              ],
            ),
          ),
      ],
    );
  }

  Widget _siteButton(BuildContext context, PTSiteFilterConfig s) {
    final c = PTColors.primary;
    final on = s.active;
    final subtle = Theme.of(context).colorScheme.onSurfaceVariant;
    final dim = on ? c : subtle;
    return Tooltip(
      message: '${s.label}：${s.selectedLabel}',
      child: GestureDetector(
        onTap: () => showSitePickerSheet(
          context,
          title: s.label,
          selected: s.selected,
          options: s.options,
          onChanged: s.onChanged,
        ),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
          constraints: const BoxConstraints(minHeight: 36),
          decoration: BoxDecoration(
            color: on ? c.withValues(alpha: 0.14) : Colors.transparent,
            border: Border.all(
              color: on
                  ? c.withValues(alpha: 0.5)
                  : context.dimText.withValues(alpha: 0.35),
            ),
            borderRadius: BorderRadius.circular(999),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(s.icon, size: 15, color: dim),
              const SizedBox(width: 4),
              ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 92),
                child: Text(
                  s.selectedLabel,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: on ? FontWeight.w700 : FontWeight.w500,
                    color: on ? c : context.mainText,
                  ),
                ),
              ),
              Icon(Icons.arrow_drop_down, size: 18, color: dim),
            ],
          ),
        ),
      ),
    );
  }

  Widget _filterButton() {
    final dims = widget.filterDimensions;
    final active = dims.where((d) => d.contributesToBadge).length;
    return IconButton(
      tooltip: active > 0 ? '筛选（$active 项）' : '筛选',
      icon: Badge(
        isLabelVisible: active > 0,
        label: Text('$active'),
        backgroundColor: PTColors.primary,
        textColor: Colors.white,
        child: const Icon(Icons.tune),
      ),
      iconSize: 20,
      padding: EdgeInsets.zero,
      constraints: const BoxConstraints(minWidth: 44, minHeight: 44),
      onPressed: () => showMultiFilterSheet(
        context,
        title: '筛选',
        dimensions: dims,
        onClearAll: widget.filterOnClearAll ?? _clearAllFallback(dims),
      ),
    );
  }

  // 默认「清除全部」：把每个维度拨回其默认值（choice→null「全部」，toggle→false）。
  // 各页若有专门的 _clearFilters（含重置搜索框等），应自己包一层 onClearAll 传入
  // PTPageScaffold 的 filterOnClearAll（见下）。这里只兜底纯维度重置。
  VoidCallback _clearAllFallback(List<PTFilterDimension> dims) {
    return () {
      for (final d in dims) {
        if (d.kind == PTFilterKind.toggle) {
          d.onChanged(false);
        } else {
          // choice：把第一个「全部」选项的值（通常 null）作为默认；找不到则 null。
          final all = d.options.isEmpty ? null : d.options.first.value;
          d.onChanged(all);
        }
      }
    };
  }
}
