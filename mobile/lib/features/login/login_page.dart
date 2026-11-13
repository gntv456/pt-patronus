// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/pt_logo.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:url_launcher/url_launcher.dart';
import '../home/home_shell.dart';
import 'server_config_page.dart';

part 'loginopsmixin.dart';
part 'login_widgets.dart';

/// 登录页：本地密码登录为主（好学站点关闭后的主路径，#issue）。
/// 旧好学账号/Cookie 双模已随站点改版（FluxTorrent）下线删除。
class LoginPage extends StatefulWidget {
  const LoginPage({super.key});
  @override
  State<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends State<LoginPage> with LoginOpsMixin {

  @override
  Future<void> _onLoginOk(Map<String, dynamic> res) async {
    await ApiClient().setToken(res['token'] as String?);
    final u = res['user'];
    if (u is Map<String, dynamic>) {
      await ApiClient().setUser(u); // 持久化身份（用户名/admin 徽章），重启后恢复
    }
    if (!mounted) return;
    Navigator.of(
      context,
    ).pushReplacement(MaterialPageRoute(builder: (_) => const HomeShell()));
  }
  @override
  void initState() {
    super.initState();
    Future.microtask(_fetchAuthStatus);
  }

  @override
  void dispose() {
    _user.dispose();
    _pass.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: GlassBackground(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child: PTEntrance(
              child: GlassCard(
              padding: const EdgeInsets.all(20),
              child: ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 360),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const Row(
                      children: [
                        PTLogo(size: 30),
                        SizedBox(width: 10),
                        Text(
                          'PTPatronus',
                          style: TextStyle(
                            fontSize: 22,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      'PT 守护神 · 自托管 PT 管理面板',
                      style: TextStyle(color: context.dimText),
                    ),
                    const SizedBox(height: 20),
                    ..._localFields(),
                    if (_demoMode) ...[
                      const SizedBox(height: 10),
                      OutlinedButton.icon(
                        onPressed: _loading ? null : _demoLogin,
                        icon: const Icon(Icons.visibility_outlined, size: 18),
                        label: const Text('浏览演示环境'),
                      ),
                    ],
                    if (_oidcEnabled) ...[
                      const SizedBox(height: 10),
                      OutlinedButton.icon(
                        onPressed: _loading ? null : _oidcLogin,
                        icon: const Icon(Icons.login_outlined, size: 18),
                        label: Text('$_oidcName 登录'),
                      ),
                    ],
                    if (_err != null)
                      Padding(
                        padding: const EdgeInsets.only(top: 10),
                        child: Text(
                          _err!,
                          style: const TextStyle(
                            color: PTColors.danger,
                            fontSize: 14,
                          ),
                        ),
                      ),
                    const SizedBox(height: 10),
                    // 切换后端地址（分发场景：每个用户后端不同）
                    Center(
                      child: TextButton.icon(
                        onPressed: () => Navigator.of(context).push(
                          MaterialPageRoute(
                            builder: (_) => const ServerConfigPage(),
                          ),
                        ),
                        icon: const Icon(Icons.dns_outlined, size: 16),
                        label: Text(
                          '后端：${ApiClient().baseUrl}',
                          style: TextStyle(
                            fontSize: 14,
                            color: context.dimText,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ),
            ),
          ),
        ),
      ),
    );
  }

}
