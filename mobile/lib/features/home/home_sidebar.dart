// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'home_controller.dart';
import 'home_nav_panel.dart';

/// 桌面 / 大屏常驻侧栏（对齐 Docker 端 Web 左侧栏）：品牌头（含收起按钮）+ 全量目的地。
///
/// 仅在宽度 ≥ [kHomeWideBreakpoint] 且 [HomeController.sidebarVisible] 时由 HomeShell 渲染。
/// 收起后 HomeShell 在左缘显示一个「展开」小按钮，点击重新钉住侧栏——即「主菜单可隐藏」。
class HomeSidebar extends StatelessWidget {
  const HomeSidebar({super.key});

  @override
  Widget build(BuildContext context) {
    final c = HomeShellScope.of(context);
    return Container(
      width: 232,
      margin: const EdgeInsets.fromLTRB(12, 12, 0, 12),
      child: NavPanel(
        radius: BorderRadius.circular(20),
        child: SafeArea(
          right: false,
          child: Column(
            children: [
              NavBrandHeader(
                trailing: IconButton(
                  tooltip: '隐藏侧栏',
                  icon: const Icon(Icons.chevron_left),
                  onPressed: () => c.setSidebarVisible(false),
                ),
              ),
              const Divider(height: 1),
              Expanded(
                child: NavGroupedList(),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// 侧栏被隐藏后，左缘悬浮的「展开」按钮（仅桌面宽屏出现）。
/// 固定窄宽度，作为 Row 中独立的一列，不抢占内容区。
class HomeSidebarReveal extends StatelessWidget {
  const HomeSidebarReveal({super.key});

  @override
  Widget build(BuildContext context) {
    final c = HomeShellScope.of(context);
    return Container(
      width: 24,
      margin: const EdgeInsets.only(left: 6),
      alignment: Alignment.center,
      child: Material(
        color: Theme.of(context).colorScheme.primary.withValues(alpha: 0.85),
        borderRadius: const BorderRadius.horizontal(right: Radius.circular(12)),
        child: Semantics(
          tooltip: '展开侧栏',
          button: true,
          child: InkWell(
            customBorder: const RoundedRectangleBorder(
              borderRadius: BorderRadius.horizontal(right: Radius.circular(12)),
            ),
            onTap: () => c.setSidebarVisible(true),
            child: const Padding(
              padding: EdgeInsets.symmetric(vertical: 18, horizontal: 2),
              child: Icon(Icons.chevron_right, color: Colors.white, size: 20),
            ),
          ),
        ),
      ),
    );
  }
}
