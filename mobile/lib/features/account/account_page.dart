import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/version.dart';
import 'package:ptpatronus/core/update_checker.dart';
import 'package:ptpatronus/core/ws_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import '../announcements/announcements_page.dart';
import '../assessment/assessment_page.dart';
import '../medal_wall/medal_wall_page.dart';
import '../backup/backup_page.dart';
import '../file_manage/file_manage_page.dart';
import '../freeleech/freeleech_page.dart';
import '../harem/harem_page.dart';
import '../login/login_page.dart';
import '../messages/messages_page.dart';
import '../monitor/monitor_page.dart';
import '../notification/notification_settings_page.dart';
import '../recommend/recommend_page.dart';
import '../reseed/reseed_page.dart';
import '../rss/rss_page.dart';
import '../savept/savept_page.dart';
import '../seedbrowse/seedbrowse_page.dart';
import '../subscriptions/subscriptions_page.dart';
import '../workflow/workflow_list_page.dart';

/// 账户页：展示登录态，提供登出（bump TokenVersion + 清本地 token，回登录页）。
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
          title: const Text('账户'),
          backgroundColor: Colors.transparent,
          elevation: 0,
        ),
        body: Center(
          child: GlassCard(
            padding: const EdgeInsets.all(24),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 320),
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
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const BackupPage()),
                            ),
                    icon: const Icon(Icons.backup_table),
                    label: const Text('备份'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const FileManagePage()),
                            ),
                    icon: const Icon(Icons.folder_open),
                    label: const Text('文件管理'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const MonitorPage()),
                            ),
                    icon: const Icon(Icons.monitor_heart),
                    label: const Text('设备监控'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const ReseedPage()),
                            ),
                    icon: const Icon(Icons.eco_outlined),
                    label: const Text('辅种'),
                  ),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const FreeleechPage()),
                            ),
                    icon: const Icon(Icons.card_giftcard),
                    label: const Text('站免'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const AnnouncementsPage()),
                            ),
                    icon: const Icon(Icons.campaign),
                    label: const Text('公告'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const SaveptPage()),
                            ),
                    icon: const Icon(Icons.monitor_heart_outlined),
                    label: const Text('PT监护室'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const HaremPage()),
                            ),
                    icon: const Icon(Icons.groups),
                    label: const Text('后宫'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const AssessmentPage()),
                            ),
                    icon: const Icon(Icons.fact_check),
                    label: const Text('站点考核'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const MedalWallPage()),
                            ),
                    icon: const Icon(Icons.workspace_premium),
                    label: const Text('勋章墙'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const MessagesPage()),
                            ),
                    icon: const Icon(Icons.mail),
                    label: const Text('消息'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const SeedBrowsePage()),
                            ),
                    icon: const Icon(Icons.article),
                    label: const Text('种子浏览'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const RecommendPage()),
                            ),
                    icon: const Icon(Icons.local_fire_department),
                    label: const Text('推荐'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const SubscriptionsPage()),
                            ),
                    icon: const Icon(Icons.subscriptions_outlined),
                    label: const Text('订阅'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const WorkflowListPage()),
                            ),
                    icon: const Icon(Icons.account_tree_outlined),
                    label: const Text('工作流'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const RssPage()),
                            ),
                    icon: const Icon(Icons.rss_feed),
                    label: const Text('RSS'),
                  ),
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
                    onPressed: _busy
                        ? null
                        : () => Navigator.of(context).push(
                              MaterialPageRoute(builder: (_) => const NotificationSettingsPage()),
                            ),
                    icon: const Icon(Icons.notifications_active_outlined),
                    label: const Text('通知设置'),
                  ),
                  const SizedBox(height: 10),
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
                  const SizedBox(height: 10),
                  FilledButton.tonalIcon(
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
