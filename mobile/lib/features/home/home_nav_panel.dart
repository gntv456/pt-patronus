import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../../theme/glass.dart';
import '../../widgets/glass_icon.dart';
import '../../widgets/pt_logo.dart';
import 'home_controller.dart';
import 'home_nav_items.dart';

/// 抽屉与侧栏共用的玻璃面板：半透明 + 模糊，尊重 [glassReduced]（降级时跳过 BackdropFilter，
/// 抬高底色保证可读）。与 [GlassCard] 视觉一致，但去掉内边距/阴影，适配通栏导航容器。
class NavPanel extends StatelessWidget {
  final Widget child;
  final BorderRadius? radius;
  const NavPanel({super.key, required this.child, this.radius});

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final r = radius ?? BorderRadius.zero;
    return ValueListenableBuilder<bool>(
      valueListenable: glassReduced,
      builder: (context, reduced, _) {
        final panel = Container(
          decoration: BoxDecoration(
            color: isDark
                ? Colors.white.withValues(alpha: reduced ? 0.14 : 0.08)
                : Colors.white.withValues(alpha: reduced ? 0.82 : 0.64),
            borderRadius: r,
            border: Border.all(
              color: Colors.white.withValues(alpha: isDark ? 0.08 : 0.6),
            ),
          ),
          child: child,
        );
        if (reduced) return panel;
        return ClipRRect(
          borderRadius: r,
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 18, sigmaY: 18),
            child: panel,
          ),
        );
      },
    );
  }
}

/// 抽屉与侧栏共用的分组导航列表 —— 渲染 [navGroups] + 底部 [navFooter]。
/// P3：分组折叠记忆 + 收藏置顶（SharedPreferences，对齐 Web useNavPrefs）。
class NavGroupedList extends StatefulWidget {
  final EdgeInsets padding;
  const NavGroupedList({
    super.key,
    this.padding = const EdgeInsets.symmetric(vertical: 8),
  });

  @override
  State<NavGroupedList> createState() => _NavGroupedListState();
}

class _NavGroupedListState extends State<NavGroupedList> {
  static const _kCollapsed = 'nav.collapsedGroups';
  static const _kFavorites = 'nav.favorites';

  final Set<String> _collapsed = {};
  final List<String> _favorites = []; // label 列表
  bool _ready = false;

  @override
  void initState() {
    super.initState();
    _loadPrefs();
  }

  Future<void> _loadPrefs() async {
    final prefs = await SharedPreferences.getInstance();
    final collapsed = prefs.getStringList(_kCollapsed) ?? const [];
    final favs = prefs.getStringList(_kFavorites) ?? const [];
    if (!mounted) return;
    setState(() {
      _collapsed
        ..clear()
        ..addAll(collapsed);
      _favorites
        ..clear()
        ..addAll(favs);
      _ready = true;
    });
  }

  Future<void> _saveCollapsed() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(_kCollapsed, _collapsed.toList());
  }

  Future<void> _saveFavorites() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setStringList(_kFavorites, _favorites);
  }

  void _toggleGroup(String title) {
    setState(() {
      if (_collapsed.contains(title)) {
        _collapsed.remove(title);
      } else {
        _collapsed.add(title);
      }
    });
    _saveCollapsed();
  }

  void _toggleFavorite(String label) {
    setState(() {
      if (_favorites.contains(label)) {
        _favorites.remove(label);
      } else {
        _favorites.add(label);
      }
    });
    _saveFavorites();
  }

  @override
  Widget build(BuildContext context) {
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    final children = <Widget>[];

    // 收藏置顶
    if (_ready && _favorites.isNotEmpty) {
      children.add(
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 14, 16, 2),
          child: Text(
            '收藏',
            style: TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w600,
              color: muted.withValues(alpha: 0.6),
            ),
          ),
        ),
      );
      for (final label in _favorites) {
        final idx = navItems.indexWhere((n) => n.label == label && !n.hidden);
        if (idx < 0) continue;
        children.add(NavTile(index: idx, favorite: true, onToggleFavorite: () => _toggleFavorite(label)));
      }
    }

    for (final g in navGroups) {
      final visible = g.items.where((it) => !it.hidden).toList();
      if (visible.isEmpty) continue;
      final collapsed = _collapsed.contains(g.title);
      children.add(
        InkWell(
          onTap: () => _toggleGroup(g.title),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 14, 12, 2),
            child: Row(
              children: [
                Expanded(
                  child: Text(
                    g.title,
                    style: TextStyle(
                      fontSize: 11,
                      fontWeight: FontWeight.w600,
                      color: muted.withValues(alpha: 0.6),
                    ),
                  ),
                ),
                Icon(
                  collapsed ? Icons.chevron_right : Icons.expand_more,
                  size: 16,
                  color: muted.withValues(alpha: 0.5),
                ),
              ],
            ),
          ),
        ),
      );
      if (collapsed) continue;
      for (final item in visible) {
        final idx = navItems.indexOf(item);
        if (idx < 0) continue;
        children.add(NavTile(
          index: idx,
          favorite: _favorites.contains(item.label),
          onToggleFavorite: () => _toggleFavorite(item.label),
        ));
      }
    }
    final footerVisible = navFooter.where((it) => !it.hidden).toList();
    if (footerVisible.isNotEmpty) {
      children.add(
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 16, vertical: 6),
          child: Divider(height: 1),
        ),
      );
      for (final item in footerVisible) {
        final idx = navItems.indexOf(item);
        if (idx < 0) continue;
        children.add(NavTile(
          index: idx,
          favorite: _favorites.contains(item.label),
          onToggleFavorite: () => _toggleFavorite(item.label),
        ));
      }
    }
    children.add(const SizedBox(height: 12));
    return ListView(padding: widget.padding, children: children);
  }
}

/// 单条菜单项：抽屉与侧栏通用。监听 [HomeController] 自行刷新选中态，
/// 避免父级整树重建。
class NavTile extends StatelessWidget {
  final int index;
  final bool favorite;
  final VoidCallback? onToggleFavorite;
  const NavTile({
    super.key,
    required this.index,
    this.favorite = false,
    this.onToggleFavorite,
  });

  @override
  Widget build(BuildContext context) {
    final c = HomeShellScope.of(context);
    final item = navItems[index];
    return ListenableBuilder(
      listenable: c,
      builder: (context, _) {
        final sel = c.index == index;
        return Container(
          margin: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
          decoration: sel
              ? BoxDecoration(
                  color: PTColors.primary.withValues(alpha: 0.18),
                  borderRadius: BorderRadius.circular(12),
                )
              : null,
          child: ListTile(
            leading: GlassIcon(
              glyph: item.glyph,
              from: item.from,
              to: item.to,
              size: 30,
            ),
            title: Text(
              item.label,
              style: TextStyle(
                fontWeight: sel ? FontWeight.w700 : FontWeight.w500,
                color: sel ? PTColors.primary : null,
              ),
            ),
            subtitle: item.desc == null
                ? null
                : Text(
                    item.desc!,
                    style: const TextStyle(fontSize: 11, color: Colors.grey),
                  ),
            trailing: onToggleFavorite == null
                ? null
                : IconButton(
                    tooltip: favorite ? '取消收藏' : '收藏',
                    icon: Icon(
                      favorite ? Icons.star : Icons.star_border,
                      size: 18,
                      color: favorite ? Colors.amber : Colors.grey,
                    ),
                    onPressed: onToggleFavorite,
                  ),
            selected: sel,
            shape: RoundedRectangleBorder(
              borderRadius: BorderRadius.circular(12),
            ),
            onTap: () => c.select(index),
          ),
        );
      },
    );
  }
}

/// 侧栏 / 抽屉顶部品牌条（logo + 名称），可选右侧操作（如侧栏收起按钮）。
class NavBrandHeader extends StatelessWidget {
  final Widget? trailing;
  const NavBrandHeader({super.key, this.trailing});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(18, 10, 8, 8),
      child: Row(
        children: [
          const PTLogo(size: 34),
          const SizedBox(width: 10),
          const Text(
            'PTPatronus',
            style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
          ),
          const Spacer(),
          ?trailing,
        ],
      ),
    );
  }
}
