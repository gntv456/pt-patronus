import 'dart:ui';
import 'package:flutter/material.dart';
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
            border: Border.all(color: Colors.white.withValues(alpha: isDark ? 0.08 : 0.6)),
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

/// 抽屉与侧栏共用的分组导航列表 —— 渲染 [navGroups]（4 组，每组带小标题）+ 底部 [navFooter]。
/// 全局 index 与扁平 [navItems] 对齐（组展开顺序 + footer），故 [NavTile.index] 可直接驱动
/// [HomeController.select]。组标题样式对齐 Web `.nav-group-title`（小号 muted 文本）。
class NavGroupedList extends StatelessWidget {
  final EdgeInsets padding;
  const NavGroupedList({super.key, this.padding = const EdgeInsets.symmetric(vertical: 8)});

  @override
  Widget build(BuildContext context) {
    final muted = Theme.of(context).colorScheme.onSurfaceVariant;
    final children = <Widget>[];
    var idx = 0;
    for (final g in navGroups) {
      children.add(Padding(
        padding: const EdgeInsets.fromLTRB(16, 14, 16, 2),
        child: Text(
          g.title,
          style: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: muted.withValues(alpha: 0.6)),
        ),
      ));
      for (final _ in g.items) {
        children.add(NavTile(index: idx++));
      }
    }
    children.add(const Padding(
      padding: EdgeInsets.symmetric(horizontal: 16, vertical: 6),
      child: Divider(height: 1),
    ));
    for (final _ in navFooter) {
      children.add(NavTile(index: idx++));
    }
    children.add(const SizedBox(height: 12));
    return ListView(padding: padding, children: children);
  }
}

/// 单条菜单项：抽屉与侧栏通用。监听 [HomeController] 自行刷新选中态，
/// 避免父级整树重建。
class NavTile extends StatelessWidget {
  final int index;
  const NavTile({super.key, required this.index});

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
                : Text(item.desc!, style: const TextStyle(fontSize: 11, color: Colors.grey)),
            selected: sel,
            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
            onTap: () => c.select(index),
          ),
        );
      },
    );
  }
}

/// 抽屉 / 侧栏顶部品牌区。
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
          const Text('PTPatronus', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
          const Spacer(),
          ?trailing,
        ],
      ),
    );
  }
}
