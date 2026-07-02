import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/core/update_checker.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/features/notice/notice_page.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'home_controller.dart';
import 'home_drawer.dart';
import 'home_nav_items.dart';
import 'home_sidebar.dart';

/// 登录后的主壳：响应式导航容器（对齐 Docker 端 Web 布局）。
///
/// - 宽屏（≥ [kHomeWideBreakpoint]，桌面/大平板）：常驻左侧栏 + 内容区；侧栏可隐藏，
///   隐藏后左缘出现「展开」按钮——主菜单可隐藏、可重新钉住。
/// - 窄屏（手机）：仅内容区；任意页面顶栏 ☰ 或左缘拖拽侧滑出主菜单抽屉（重构前交互复刻）。
///
/// 主菜单目的地统一来自 [navItems]（与 Docker Web 侧栏顺序对齐），抽屉与侧栏共用。
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});
  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> with WidgetsBindingObserver {
  final HomeController _controller = HomeController();
  StreamSubscription<Notice>? _noticeSub;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this); // 监听前后台：后台断 WS 省电省流量
    refreshNoticeUnread();
    _noticeSub = WsClient().notices.listen((_) => refreshNoticeUnread());
    // 启动后台检查更新（silent：仅在有新版时弹对话框，不打扰已是最新版本的用户）
    checkAndPromptUpdate(context, silent: true);
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _noticeSub?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    // 后台断 WS（停指数退避重连循环）；回前台重连。各页轮询 timer 由页面自管（审计 P1-6）
    if (state == AppLifecycleState.paused || state == AppLifecycleState.hidden) {
      WsClient().disconnect();
    } else if (state == AppLifecycleState.resumed) {
      WsClient().connect();
    }
  }

  @override
  Widget build(BuildContext context) {
    return HomeShellScope(
      controller: _controller,
      child: GlassBackground(
        child: ListenableBuilder(
          listenable: _controller,
          builder: (context, _) {
            final wide = MediaQuery.widthOf(context) >= kHomeWideBreakpoint;
            final page = navItems[_controller.index].builder(context);
            // 窄屏使用最大宽度限制的内容卡片化体验（页面自带 Scaffold/AppBar）。
            final Widget body;
            if (wide) {
              if (_controller.sidebarVisible) {
                body = Row(
                  children: [const HomeSidebar(), Expanded(child: page)],
                );
              } else {
                body = Row(
                  children: [const HomeSidebarReveal(), Expanded(child: page)],
                );
              }
            } else {
              body = page;
            }
            return Scaffold(
              key: _controller.scaffoldKey,
              backgroundColor: Colors.transparent,
              drawer: const HomeDrawer(),
              drawerEdgeDragWidth: 48,
              body: body,
              floatingActionButton: ValueListenableBuilder<int>(
                valueListenable: noticeUnread,
                builder: (_, n, _) => FloatingActionButton(
                  mini: true,
                  backgroundColor: Theme.of(context).colorScheme.primaryContainer,
                  onPressed: () => Navigator.push(
                    context,
                    MaterialPageRoute(builder: (_) => const NoticePage()),
                  ),
                  child: Badge(
                    isLabelVisible: n > 0,
                    label: Text(n > 99 ? '99+' : '$n',
                        style: const TextStyle(fontSize: 10, fontWeight: FontWeight.w700)),
                    child: const Icon(Icons.notifications),
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }
}
