// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 adaptive.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/widgets/adaptive.dart';

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
/// 与全局 appBarTheme.toolbarHeight(48) 对齐——44×44 触摸区落在 48 高工具栏内，留 2px 上下余量。
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
