// LoginPage 的展示层扩展（无 setState 段；从 login_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/login/login_page.dart';

extension on _LoginPageState {
  // OIDC/SSO：移动端 deep-link（ptpatronus://）回调尚未接通——直接拉起系统浏览器会令牌丢失，
  // 用户卡在浏览器无法返回 App。改为弹说明引导用账号/Cookie 登录（保留「仍打开浏览器」兜底）。
  Future<void> _oidcLogin() async {
    if (!mounted) return;
    await showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text('$_oidcName 暂不可用'),
        content: const Text(
          '移动端 SSO 自动回调（deep-link）尚未接通：在浏览器完成授权后，令牌无法自动回到 App。\n\n'
          '请改用「好学账号」或「Cookie」标签页登录。',
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


  List<Widget> _accountFields() {
    final needCap = _captcha?['need_captcha'] == true;
    return [
      TextField(
        controller: _user,
        decoration: const InputDecoration(labelText: '好学用户名'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _pass,
        obscureText: true,
        decoration: const InputDecoration(labelText: '好学密码'),
      ),
      const SizedBox(height: 12),
      TextField(
        controller: _twoStep,
        decoration: const InputDecoration(labelText: '二级验证码（开了 2FA 才填，6 位）'),
      ),
      if (needCap) ...[
        const SizedBox(height: 12),
        Row(
          children: [
            if (_captchaBytes != null)
              GestureDetector(
                onTap: () => _fetchCaptcha(),
                child: Image.memory(
                  _captchaBytes!,
                  height: 40,
                  gaplessPlayback: true,
                ),
              ),
            const SizedBox(width: 8),
            Expanded(
              child: TextField(
                controller: _imgStr,
                decoration: const InputDecoration(labelText: '验证码'),
              ),
            ),
          ],
        ),
      ],
      const SizedBox(height: 22),
      FilledButton(
        onPressed: _loading ? null : _accountLogin,
        child: _loading
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Text('登录'),
      ),
      if (_captcha?['challenge_mode'] == true)
        Padding(
          padding: const EdgeInsets.only(top: 8),
          child: Text(
            '好学已开启挑战响应认证，后端会自动处理，直接登录即可；如登录失败可改用 Cookie 登录',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
        ),
    ];
  }
}
