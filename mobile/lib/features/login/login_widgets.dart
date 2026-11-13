// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// LoginPage 的展示层扩展（无 setState 段；从 login_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/login/login_page.dart';

extension on _LoginPageState {
  // OIDC/SSO：移动端 deep-link（ptpatronus://）回调尚未接通——直接拉起系统浏览器会令牌丢失，
  // 用户卡在浏览器无法返回 App。改为弹说明引导用本地账号密码登录（保留「仍打开浏览器」兜底）。
  Future<void> _oidcLogin() async {
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('$_oidcName 暂不可用'),
        content: const Text(
          '移动端 SSO 自动回调（deep-link）尚未接通：在浏览器完成授权后，令牌无法自动回到 App。\n\n'
          '请改用本地账号密码登录。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('知道了'),
          ),
          TextButton(
            onPressed: () {
              Navigator.of(ctx).pop();
              final uri = Uri.tryParse(
                '${ApiClient().baseUrl}/api/v1/auth/oidc/start',
              );
              if (uri != null) {
                launchUrl(uri, mode: LaunchMode.externalApplication);
              }
            },
            child: const Text('仍打开浏览器'),
          ),
        ],
      ),
    );
  }


  // 本地密码登录表单（好学站点关闭后的主路径，#<issue>）：用户名 + 密码，
  // 首次登录自动建号。
  List<Widget> _localFields() {
    return [
      TextField(
        controller: _user,
        autofillHints: const [AutofillHints.username],
        textInputAction: TextInputAction.next,
        style: const TextStyle(fontSize: 16),
        decoration: const InputDecoration(labelText: '用户名'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _pass,
        autofillHints: const [AutofillHints.password],
        // 密码可见切换 + 回车提交（输完密码不必手点登录按钮）。
        obscureText: !_showPass,
        style: const TextStyle(fontSize: 16),
        onSubmitted: (_) => _loading ? null : _localLogin(),
        decoration: InputDecoration(
          labelText: '密码',
          suffixIcon: IconButton(
            icon: Icon(_showPass ? Icons.visibility_off : Icons.visibility),
            onPressed: togglePassVisible,
            tooltip: _showPass ? '隐藏密码' : '显示密码',
          ),
        ),
      ),
      const SizedBox(height: 8),
      Text(
        '首次登录自动创建账号（成为管理员）；已有账号请用原密码登录',
        style: TextStyle(fontSize: 14, color: context.dimText),
      ),
      const SizedBox(height: 22),
      FilledButton(
        onPressed: _loading ? null : _localLogin,
        child: _loading
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('登录'),
      ),
    ];
  }

}
