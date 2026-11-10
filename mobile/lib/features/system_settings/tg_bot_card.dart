// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 system_settings_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/system_settings/system_settings_page.dart';

class _TgBotCard extends StatefulWidget {
  final TgBotApi api;
  final TgBotConfig initial;
  const _TgBotCard({required this.api, required this.initial});
  @override
  State<_TgBotCard> createState() => _TgBotCardState();
}

class _TgBotCardState extends State<_TgBotCard> {
  late bool _enabled = widget.initial.enabled;
  late bool _allowGroupChat = widget.initial.allowGroupChat;
  late final _tokenCtrl = TextEditingController();
  late final _idsCtrl = TextEditingController(
    text: widget.initial.allowedUserIds.join(', '),
  );
  late final _apiUrlCtrl = TextEditingController(text: widget.initial.apiUrl);
  bool _saving = false;

  @override
  void dispose() {
    _tokenCtrl.dispose();
    _idsCtrl.dispose();
    _apiUrlCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    // 逗号分隔 → int 白名单；非法片段静默丢弃。
    final ids = _idsCtrl.text
        .split(',')
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .map((s) => int.tryParse(s))
        .whereType<int>()
        .toList();
    final body = <String, dynamic>{
      'enabled': _enabled,
      'allowed_user_ids': ids,
      'allow_group_chat': _allowGroupChat,
      'api_url': _apiUrlCtrl.text.trim(),
      // bot_token 仅在用户输入新 token 时携带；留空 = 保留原 token。
      if (_tokenCtrl.text.trim().isNotEmpty)
        'bot_token': _tokenCtrl.text.trim(),
    };
    try {
      final updated = await widget.api.update(body);
      if (!mounted) return;
      _tokenCtrl.clear();
      setState(() {
        _enabled = updated.enabled;
        _allowGroupChat = updated.allowGroupChat;
        _idsCtrl.text = updated.allowedUserIds.join(', ');
        _apiUrlCtrl.text = updated.apiUrl;
      });
      _toast('Telegram Bot 配置已保存（部分改动需重启后端生效）');
    } catch (e) {
      _toast('保存失败：$e');
      await _reload();
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _reload() async {
    try {
      final c = await widget.api.get();
      if (!mounted) return;
      setState(() {
        _enabled = c.enabled;
        _allowGroupChat = c.allowGroupChat;
        _idsCtrl.text = c.allowedUserIds.join(', ');
        _apiUrlCtrl.text = c.apiUrl;
      });
      _tokenCtrl.clear();
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'message',
                from: Color(0xFF2193B0),
                to: Color(0xFF6DD5ED),
                size: 32,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('Telegram Bot',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '入站 bot：长轮询收消息 → 斜杠命令或自然语言 → 复用 AI Agent → 回帖。',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
          const SizedBox(height: 12),
          _field(
            'Bot Token',
            TextField(
              controller: _tokenCtrl,
              obscureText: true,
              decoration: InputDecoration(
                isDense: true,
                hintText: widget.initial.botTokenConfigured
                    ? '令牌已配置（留空 = 保留）'
                    : '留空 = 保留原 Token',
                border: const OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '允许用户 ID（逗号分隔）',
            TextField(
              controller: _idsCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                hintText: '如：123456789, 987654321',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _switchRow('允许群聊响应', _allowGroupChat,
              (v) => setState(() => _allowGroupChat = v)),
          _field(
            'API 地址（可选，国内反代；留空 = 官方）',
            TextField(
              controller: _apiUrlCtrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: 'https://api.telegram.org',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: _saving ? null : _save,
            icon: _saving
                ? const SizedBox(
                    width: 16, height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                : const Icon(Icons.save_outlined, size: 18),
            label: const Text('保存'),
          ),
        ],
      ),
    );
  }

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(value: value, onChanged: onChanged),
        ],
      ),
    );
  }
}
