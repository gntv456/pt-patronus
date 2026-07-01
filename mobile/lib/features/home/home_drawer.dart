import 'package:flutter/material.dart';
import 'home_nav_panel.dart';

/// 侧滑主菜单抽屉（重构前交互复刻）：品牌头 + 全量目的地列表 + 底部分组（通知设置 / 用户中心）。
///
/// 由 HomeShell 的 Scaffold.drawer 承载；左缘拖拽与各页面顶栏 ☰ 均可打开。
/// 选中任意项即切换目的地并自动收起抽屉。
class HomeDrawer extends StatelessWidget {
  const HomeDrawer({super.key});

  @override
  Widget build(BuildContext context) {
    return Drawer(
      backgroundColor: Colors.transparent,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.horizontal(right: Radius.circular(24)),
      ),
      child: NavPanel(
        radius: const BorderRadius.horizontal(right: Radius.circular(24)),
        child: SizedBox(
          width: 264,
          child: SafeArea(
            right: false,
            child: Column(
              children: [
                NavBrandHeader(
                  trailing: IconButton(
                    icon: const Icon(Icons.close),
                    onPressed: () => Navigator.of(context).pop(),
                    tooltip: '关闭',
                  ),
                ),
                const Divider(height: 1),
                Expanded(child: NavGroupedList()),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
