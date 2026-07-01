import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/version.dart';
import 'package:ptpatronus/core/update_checker.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import '../login/login_page.dart';

/// 用户中心：登录态、版本/检查更新、性能模式（玻璃降级）、登出。
///
/// 其余功能页已统一迁入主菜单（抽屉 / 桌面侧栏），不再以列表入口形式堆叠在此——
/// 避免与导航容器重复（重构前 DrawerContent 底部「用户中心」的等价物）。
class AccountPage extends StatefulWidget {
  const AccountPage({super.key});
  @override
  State<AccountPage> createState() => _AccountPageState();
}

class _AccountPageState extends State<AccountPage> {
  bool _busy = false;
  String _version = appVersion; // 离线兜底；运行时优先读后端 /system/info

  @override
  void initState() {
    super.initState();
    // 版本号以后端为单一真源：运行时拉取，失败用本地兜底常量。
    ApiClient().get('/system/info').then((m) {
      final v = m['version'] as String?;
      if (v != null && v.isNotEmpty && mounted) setState(() => _version = v);
    }).catchError((_) {});
  }

  Future<void> _logout() async {
    setState(() => _busy = true);
    // 先断 WS：停掉用旧 token 的重连循环（disconnect 保留单例，重登后可恢复）。
    WsClient().disconnect();
    await ApiClient().logout();
    if (!mounted) return;
    Navigator.of(context).pushReplacement(
      MaterialPageRoute(builder: (_) => const LoginPage()),
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('用户中心'),
          backgroundColor: Colors.transparent,
          elevation: 0,
        ),
        body: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: GlassCard(
              padding: const EdgeInsets.all(24),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 360),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Row(children: [
                      Text('🛡️', style: TextStyle(fontSize: 28)),
                      SizedBox(width: 10),
                      Text('PTPatronus',
                          style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700)),
                    ]),
                    const SizedBox(height: 8),
                    Builder(builder: (_) {
                      final u = ApiClient().user;
                      final name = (u?['username'] as String?)?.isNotEmpty == true
                          ? (u!['username'] as String)
                          : ((u?['hxpt_username'] as String?) ?? '好学用户');
                      final isAdmin = u?['is_admin'] == true;
                      return Text(
                        isAdmin ? '管理员 · $name' : '已登录 · $name',
                        style: const TextStyle(color: Colors.grey),
                      );
                    }),
                    Row(mainAxisSize: MainAxisSize.min, children: [
                      Text('版本 $_version',
                          style: const TextStyle(color: Colors.grey, fontSize: 12)),
                      const SizedBox(width: 8),
                      GestureDetector(
                        behavior: HitTestBehavior.opaque,
                        onTap: () => checkAndPromptUpdate(context),
                        child: const Text('检查更新',
                            style: TextStyle(color: Colors.blue, fontSize: 12)),
                      ),
                    ]),
                    const SizedBox(height: 22),
                    ListenableBuilder(
                      listenable: glassReduced,
                      builder: (context, _) => SwitchListTile(
                        contentPadding: EdgeInsets.zero,
                        title: const Text('性能模式', style: TextStyle(fontWeight: FontWeight.w600)),
                        subtitle: const Text('关闭玻璃模糊，更省电/更流畅', style: TextStyle(fontSize: 12)),
                        value: glassReduced.value,
                        onChanged: (v) => setGlassReduced(v),
                      ),
                    ),
                    const SizedBox(height: 12),
                    ListenableBuilder(
                      listenable: themeModeNotifier,
                      builder: (context, _) => Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Padding(
                            padding: EdgeInsets.symmetric(horizontal: 16),
                            child: Text('主题模式', style: TextStyle(fontWeight: FontWeight.w600)),
                          ),
                          Padding(
                            padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 4),
                            child: SegmentedButton<ThemeMode>(
                              segments: const [
                                ButtonSegment(value: ThemeMode.system, label: Text('跟随系统')),
                                ButtonSegment(value: ThemeMode.light, label: Text('浅色')),
                                ButtonSegment(value: ThemeMode.dark, label: Text('深色')),
                              ],
                              selected: {themeModeNotifier.value},
                              onSelectionChanged: (s) => setThemeMode(s.first),
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 16),
                    FilledButton.icon(
                      onPressed: _busy ? null : _logout,
                      icon: const Icon(Icons.logout),
                      label: const Text('登出'),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
