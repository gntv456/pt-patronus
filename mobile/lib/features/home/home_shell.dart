import 'dart:async';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/notice.dart';
import 'package:ptpatronus/core/update_checker.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/features/account/account_page.dart';
import 'package:ptpatronus/features/notice/notice_page.dart';
import 'package:ptpatronus/features/dashboard/dashboard_page.dart';
import 'package:ptpatronus/features/search/search_page.dart';
import 'package:ptpatronus/features/sites/sites_page.dart';
import 'package:ptpatronus/features/downloaders/downloaders_page.dart';
import 'package:ptpatronus/features/entertainment/entertainment_page.dart';
import 'package:ptpatronus/features/media/media_page.dart';
import 'package:ptpatronus/theme/glass.dart';

// 登录后的主壳：底部导航在 仪表盘/搜索/站点/下载器 间切换。
class HomeShell extends StatefulWidget {
  const HomeShell({super.key});
  @override
  State<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends State<HomeShell> {
  int _index = 0;
  StreamSubscription<Notice>? _noticeSub;

  @override
  void initState() {
    super.initState();
    refreshNoticeUnread();
    _noticeSub = WsClient().notices.listen((_) => refreshNoticeUnread());
    // 启动后台检查更新（silent：仅在有新版时弹对话框，不打扰已是最新版本的用户）
    checkAndPromptUpdate(context, silent: true);
  }

  @override
  void dispose() {
    _noticeSub?.cancel();
    super.dispose();
  }

  static const _pages = <Widget>[
    DashboardPage(),
    SearchPage(),
    SitesPage(),
    DownloadersPage(),
    MediaPage(),
    EntertainmentPage(),
    AccountPage(),
  ];

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        body: IndexedStack(index: _index, children: _pages),
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
        bottomNavigationBar: Container(
          margin: const EdgeInsets.fromLTRB(12, 0, 12, 12),
          decoration: BoxDecoration(
            color: isDark ? Colors.white.withValues(alpha: 0.08) : Colors.white.withValues(alpha: 0.6),
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: Colors.white.withValues(alpha: isDark ? 0.08 : 0.6)),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: isDark ? 0.4 : 0.10),
                blurRadius: 24,
                offset: const Offset(0, 6),
              ),
            ],
          ),
          child: NavigationBar(
            backgroundColor: Colors.transparent,
            elevation: 0,
            selectedIndex: _index,
            onDestinationSelected: (i) => setState(() => _index = i),
            destinations: const [
              NavigationDestination(icon: Icon(Icons.dashboard_outlined), selectedIcon: Icon(Icons.dashboard), label: '仪表盘'),
              NavigationDestination(icon: Icon(Icons.search_outlined), selectedIcon: Icon(Icons.search), label: '搜索'),
              NavigationDestination(icon: Icon(Icons.language_outlined), selectedIcon: Icon(Icons.language), label: '站点'),
              NavigationDestination(icon: Icon(Icons.download_outlined), selectedIcon: Icon(Icons.download), label: '下载器'),
              NavigationDestination(icon: Icon(Icons.movie_outlined), selectedIcon: Icon(Icons.movie), label: '媒体'),
              NavigationDestination(icon: Icon(Icons.sports_esports_outlined), selectedIcon: Icon(Icons.sports_esports), label: '娱乐'),
              NavigationDestination(icon: Icon(Icons.person_outline), selectedIcon: Icon(Icons.person), label: '账户'),
            ],
          ),
        ),
      ),
    );
  }
}
