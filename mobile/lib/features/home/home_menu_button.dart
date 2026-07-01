import 'package:flutter/material.dart';
import 'home_controller.dart';

/// 各导航页顶栏的 ☰ 按钮：点击侧滑出主菜单抽屉（重构前交互）。
///
/// 抽屉挂在 HomeShell 的 Scaffold 上；此处通过 [HomeShellScope] 拿到控制器调用 openDrawer。
/// 用作页面 AppBar 的 leading（页面自带返回键的子页不使用本按钮）。
class HomeMenuButton extends StatelessWidget {
  final String? tooltip;
  const HomeMenuButton({super.key, this.tooltip});

  @override
  Widget build(BuildContext context) {
    return IconButton(
      tooltip: tooltip ?? '主菜单',
      icon: const Icon(Icons.menu),
      onPressed: () => HomeShellScope.of(context).openDrawer(),
    );
  }
}
