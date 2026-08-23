import 'package:flutter/material.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/features/home/home_controller.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_filter_sheet.dart';

// ─────────────────────────────────────────────────────────────────────────
// 统一的每页顶栏 + 页壳。收敛全站 23 个导航页两套头部范式（手写 Row / 真 AppBar）
// 为同一根紧凑顶栏：leading 菜单（窄屏）/ 标题 / [站点按钮] / [筛选按钮] / [搜索按钮]
// / 操作图标按钮（>2 自动进 ⋮ 溢出）。
//
// 设计要点（见 plans/piped-booping-marshmallow.md Path B）：
//  - 不走 Scaffold.appBar：PTPageAppBar 放进 body 顶部 Column（AppBar + 可选搜索行），
//    这样搜索能从顶栏「向下弹出」一行（AnimatedSize 滑出），而非挤进标题位。
//  - wide 屏侧栏常驻，leading 菜单按钮自动隐藏（不再冗余）。
//  - 操作 narrow=PTIconAction(44×44) / wide=tonal 按钮；overflow=true 或 narrow>2 进 ⋮。
//  - 搜索：点顶栏搜索图标 → 顶栏下方弹出一行 TextField（✕ 收起），宽窄屏统一。
//    _searchOpen 是本 StatefulWidget 自持，仅 tap/✕ 触发，不 in-build。
//  - ⋮ 一律 44×44（修 sites/logs 现有 30×30 的 HIG 触摸违规）。
// ─────────────────────────────────────────────────────────────────────────

const int _kMaxInlineActionsNarrow = 2;

/// 顶栏一个操作。narrow→PTIconAction(44×44)；wide→tonal 文字按钮。
/// `overflow:true` 恒进尾部 ⋮ 菜单；narrow 下 inline 超过 [_kMaxInlineActionsNarrow]
/// 时尾部自动溢出进 ⋮。取代 PTHeaderAction 与各页散落 PopupMenuButton 项。
class PTBarAction {
  final IconData icon;
  final String label;
  final VoidCallback? onPressed;
  final bool loading; // narrow→PTIconAction(loading:) 显 spinner
  final bool overflow; // true→恒进 ⋮
  const PTBarAction({
    required this.icon,
    required this.label,
    this.onPressed,
    this.loading = false,
    this.overflow = false,
  });
}

/// 顶栏向下弹出搜索配置。点顶栏搜索图标 → 顶栏下方弹出一行 TextField（✕ 收起）。
/// 既用于「搜索是辅助筛选」的页（SavePT 搜站点名，onChanged 即时过滤），
/// 也用于搜索主页（Search 页，onSubmitted 回车搜索、删 body 搜索框避免重复）。
class PTSearchConfig {
  final TextEditingController controller;
  final ValueChanged<String> onChanged; // 即时过滤（SavePT onChanged）
  final ValueChanged<String>? onSubmitted; // 可选：回车搜索
  final String hintText;
  final bool autofocusOnOpen; // 弹出时自动聚焦
  const PTSearchConfig({
    required this.controller,
    required this.onChanged,
    this.onSubmitted,
    this.hintText = '搜索…',
    this.autofocusOnOpen = true,
  });
}

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
    this.toolbarHeight = 44,
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

/// 页壳：糊样板——透明 Scaffold + body 顶部 Column[PTPageAppBar, Expanded(body)]
/// + 页 FAB。收敛 A 营手写 Row 头 + B 营真 AppBar 两套范式为同一根顶栏。
/// PTPageAppBar 放 body 顶部（非 appBar 槽），以支持搜索「向下弹出」。
///
/// [filterOnClearAll] 透传给筛选弹窗的「清除全部」；不传则 PTPageAppBar 用维度兜底重置。
class PTPageScaffold extends StatelessWidget {
  final Object title;
  final Widget? leading;
  final List<PTBarAction> actions;
  final List<PTFilterDimension> filterDimensions;
  final PTSearchConfig? search;
  final List<Widget>? trailing;
  final Widget body;
  final Widget? floatingActionButton;
  final FloatingActionButtonLocation? floatingActionButtonLocation;
  final VoidCallback? filterOnClearAll;
  final PTSiteFilterConfig? siteFilter;
  final double toolbarHeight;

  const PTPageScaffold({
    super.key,
    required this.title,
    required this.body,
    this.actions = const [],
    this.leading,
    this.filterDimensions = const [],
    this.search,
    this.trailing,
    this.floatingActionButton,
    this.floatingActionButtonLocation,
    this.filterOnClearAll,
    this.siteFilter,
    this.toolbarHeight = 44,
  });

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        body: Column(
          children: [
            PTPageAppBar(
              title: title,
              leading: leading,
              actions: actions,
              filterDimensions: filterDimensions,
              filterOnClearAll: filterOnClearAll,
              siteFilter: siteFilter,
              search: search,
              trailing: trailing,
              toolbarHeight: toolbarHeight,
            ),
            Expanded(child: body),
          ],
        ),
        floatingActionButton: floatingActionButton,
        floatingActionButtonLocation: floatingActionButtonLocation,
      ),
    );
  }
}
