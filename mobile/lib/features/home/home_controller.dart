import 'package:flutter/material.dart';

/// 宽度阈值：≥ 此值显示常驻侧栏（桌面 / 大平板横屏），否则仅抽屉（手机布局）。
/// 与 Docker 端 Web「侧栏常驻 + 窄屏堆叠」的响应式语义一致。
const double kHomeWideBreakpoint = 1000;

/// 主壳控制器：当前页索引、桌面侧栏可见性、抽屉开关。
///
/// 通过 [HomeShellScope] 暴露给页面顶栏的 ☰、抽屉、侧栏。变更通过 [notifyListeners]
/// 驱动 HomeShell 内的 ListenableBuilder 重建（侧栏显隐 / 选中态）。
class HomeController extends ChangeNotifier {
  int _index = 0;
  bool _sidebarVisible = true; // 桌面端默认显示常驻侧栏（对齐 Docker）
  final GlobalKey<ScaffoldState> scaffoldKey = GlobalKey<ScaffoldState>();

  /// AI 助手在 [navItems] 中的索引（由 HomeShell 在 init 时按 glyph `'bot'` 解析注入）。
  /// 顶栏常驻按钮据此 select；该 NavItem 已 `hidden`，不在抽屉/侧栏渲染。
  /// 放控制器而非 widget 层解析，避免 widgets/pt_page_scaffold → features/home/home_nav_items
  /// 的循环导入（nav_items 反向导入各页面 → 页面又用 PTPageScaffold）。
  int? assistantIndex;

  int get index => _index;
  bool get sidebarVisible => _sidebarVisible;

  /// 切换主菜单目的地。重复点击仅关闭抽屉。选中后自动收起抽屉（窄屏）。
  void select(int i) {
    final changed = _index != i;
    _index = i;
    if (changed) notifyListeners();
    _closeDrawer();
  }

  void setSidebarVisible(bool v) {
    if (_sidebarVisible == v) return;
    _sidebarVisible = v;
    notifyListeners();
  }

  /// ☰ 入口：打开侧滑抽屉（所有客户端统一的「点击侧滑出来」）。
  void openDrawer() => scaffoldKey.currentState?.openDrawer();

  void _closeDrawer() {
    final st = scaffoldKey.currentState;
    if (st != null && st.isDrawerOpen) st.closeDrawer();
  }
}

/// 把 [HomeController] 注入子树。需要随控制器重建的组件用 ListenableBuilder 监听；
/// 只读引用（如顶栏 ☰ 调用 openDrawer）直接 [HomeShellScope.of] 即可。
class HomeShellScope extends InheritedWidget {
  final HomeController controller;
  const HomeShellScope({
    super.key,
    required this.controller,
    required super.child,
  });

  static HomeController of(BuildContext context) {
    final scope = context.dependOnInheritedWidgetOfExactType<HomeShellScope>();
    assert(scope != null, 'HomeShellScope not found — 仅在 HomeShell 子树内可用');
    return scope!.controller;
  }

  @override
  bool updateShouldNotify(covariant HomeShellScope oldWidget) =>
      controller != oldWidget.controller;
}
