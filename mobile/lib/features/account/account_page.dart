import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_inappwebview/flutter_inappwebview.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/version.dart';
import 'package:ptpatronus/core/update_checker.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'package:url_launcher/url_launcher.dart';
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
  String _clientVersion = appVersion; // 本机安装版本（package_info_plus，兜底 appVersion）
  String _backendVersion = '-'; // 所连后端版本（/system/info）

  @override
  void initState() {
    super.initState();
    // 客户端版本：本机安装版本（package_info_plus），与所连后端无关。
    getClientVersion().then((v) {
      if (mounted) setState(() => _clientVersion = v);
    });
    // 后端版本：所连后端 /system/info 的 version（失败保留 '-'）。
    ApiClient()
        .get('/system/info')
        .then((m) {
          final v = m['version'] as String?;
          if (v != null && v.isNotEmpty && mounted) {
            setState(() => _backendVersion = v);
          }
        })
        .catchError((_) {});
  }

  Future<void> _logout() async {
    // 二次确认（审计 P1）：避免误点立即登出、断会话
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('登出'),
        content: const Text('将断开当前会话与实时推送，确认登出？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('登出'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    setState(() => _busy = true);
    // 先断 WS：停掉用旧 token 的重连循环（disconnect 保留单例，重登后可恢复）。
    WsClient().disconnect();
    // 清 WebView cookie/缓存：防登出后仍能从 WebView 取回上游好学/站点会话（审计 P1）
    try {
      await CookieManager.instance().deleteAllCookies();
      await WebStorageManager.instance().deleteAllData();
    } catch (_) {}
    await ApiClient().logout();
    if (!mounted) return;
    // 清栈跳登录：pushReplacement 会留 HomeShell 在栈底、Back 键回到已登出骨架，改 pushAndRemoveUntil（审计 P1）
    Navigator.of(context).pushAndRemoveUntil(
      MaterialPageRoute(builder: (_) => const LoginPage()),
      (_) => false,
    );
  }

  void _showHelp(BuildContext context) {
    showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('使用帮助'),
        content: const SingleChildScrollView(
          child: Text(
            'PTPatronus（PT 守护神）核心价值：账号保号防封。\n\n'
            '快速上手三步：\n'
            '1. 站点 → 添加 PT 站点（粘贴 cookie 或 WebView 登录）\n'
            '2. 下载器 → 添加 qBittorrent / Transmission\n'
            '3. 仪表盘 → 一键同步统计；签到 / 辅种 / H&R 告警自动运行\n\n'
            '常用术语：\n'
            '· 辅种：跨站辅助种子\n'
            '· 监护室：站点健康监控\n'
            '· 后宫：邀请管理\n'
            '· 站免：全站免费促销\n'
            '· H&R：未达标种子告警',
            style: TextStyle(height: 1.5),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
  }

  Future<void> _openIssues() async {
    final uri = Uri.parse('https://github.com/gntv456/pt-patronus/issues');
    if (!await launchUrl(uri, mode: LaunchMode.externalApplication)) {
      if (!mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('无法打开浏览器')));
    }
  }

  Future<void> _copyDiagnostics() async {
    final u = ApiClient().user;
    final text = [
      'PTPatronus mobile',
      'client: $_clientVersion',
      'backend: $_backendVersion (${ApiClient().baseUrl})',
      'user: ${u?['username'] ?? u?['hxpt_username'] ?? '-'}',
      'admin: ${u?['is_admin'] == true}',
    ].join('\n');
    await Clipboard.setData(ClipboardData(text: text));
    if (!mounted) return;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(const SnackBar(content: Text('诊断信息已复制')));
  }

  void _showFeedback(BuildContext context) {
    showDialog<void>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('问题反馈'),
        content: const Text('如遇问题或建议，可以打开 GitHub Issues 提交，也可以先复制诊断信息附在反馈里。'),
        actions: [
          TextButton(onPressed: _copyDiagnostics, child: const Text('复制诊断信息')),
          FilledButton.icon(
            onPressed: _openIssues,
            icon: const Icon(Icons.open_in_new, size: 18),
            label: const Text('打开 Issues'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '用户中心',
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: GlassCard(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 520),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  const Row(
                    children: [
                      Icon(
                        Icons.shield_outlined,
                        size: 28,
                        color: PTColors.primary,
                      ),
                      SizedBox(width: 10),
                      Text(
                        'PTPatronus',
                        style: TextStyle(
                          fontSize: 20,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Builder(
                    builder: (_) {
                      final u = ApiClient().user;
                      final name =
                          (u?['username'] as String?)?.isNotEmpty == true
                          ? (u!['username'] as String)
                          : ((u?['hxpt_username'] as String?) ?? '好学用户');
                      final isAdmin = u?['is_admin'] == true;
                      return Text(
                        isAdmin ? '管理员 · $name' : '已登录 · $name',
                        style: TextStyle(color: context.dimText),
                      );
                    },
                  ),
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '客户端 v$_clientVersion · 后端 v$_backendVersion',
                        style: TextStyle(
                          color: context.dimText,
                          fontSize: 12,
                        ),
                      ),
                      const SizedBox(height: 4),
                      Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          GestureDetector(
                            behavior: HitTestBehavior.opaque,
                            onTap: () => checkAndPromptUpdate(context),
                            child: const Text(
                              '检查更新',
                              style: TextStyle(color: Colors.blue, fontSize: 12),
                            ),
                          ),
                          const SizedBox(width: 16),
                          GestureDetector(
                            behavior: HitTestBehavior.opaque,
                            onTap: () => _showHelp(context),
                            child: const Text(
                              '使用帮助',
                              style: TextStyle(color: Colors.blue, fontSize: 12),
                            ),
                          ),
                          const SizedBox(width: 16),
                          GestureDetector(
                            behavior: HitTestBehavior.opaque,
                            onTap: () => _showFeedback(context),
                            child: const Text(
                              '问题反馈',
                              style: TextStyle(color: Colors.blue, fontSize: 12),
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                  const SizedBox(height: 22),
                  ListenableBuilder(
                    listenable: glassReduced,
                    builder: (context, _) => SwitchListTile(
                      contentPadding: EdgeInsets.zero,
                      title: const Text(
                        '性能模式',
                        style: TextStyle(fontWeight: FontWeight.w600),
                      ),
                      subtitle: const Text(
                        '关闭玻璃模糊，更省电/更流畅',
                        style: TextStyle(fontSize: 12),
                      ),
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
                          child: Text(
                            '主题模式',
                            style: TextStyle(fontWeight: FontWeight.w600),
                          ),
                        ),
                        Padding(
                          padding: const EdgeInsets.symmetric(
                            horizontal: 4,
                            vertical: 4,
                          ),
                          child: SegmentedButton<ThemeMode>(
                            segments: const [
                              ButtonSegment(
                                value: ThemeMode.system,
                                label: Text('跟随系统'),
                              ),
                              ButtonSegment(
                                value: ThemeMode.light,
                                label: Text('浅色'),
                              ),
                              ButtonSegment(
                                value: ThemeMode.dark,
                                label: Text('深色'),
                              ),
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
    );
  }
}
