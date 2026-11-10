// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 system_settings_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/system_settings/system_settings_page.dart';

class _CookieCloudCard extends StatefulWidget {
  final CookieCloudApi api;
  final CookieCloudConfig initial;
  const _CookieCloudCard({required this.api, required this.initial});
  @override
  State<_CookieCloudCard> createState() => _CookieCloudCardState();
}

class _CookieCloudCardState extends State<_CookieCloudCard> {
  late bool _enabled = widget.initial.enabled;
  late final _serverCtrl = TextEditingController(text: widget.initial.server);
  late final _uuidCtrl = TextEditingController(text: widget.initial.uuid);
  late final _passCtrl = TextEditingController();
  late final _intervalCtrl =
      TextEditingController(text: '${widget.initial.intervalMin}');
  bool _saving = false;
  bool _syncing = false;

  @override
  void dispose() {
    _serverCtrl.dispose();
    _uuidCtrl.dispose();
    _passCtrl.dispose();
    _intervalCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    final interval = int.tryParse(_intervalCtrl.text.trim()) ?? 120;
    final body = <String, dynamic>{
      'enabled': _enabled,
      'server': _serverCtrl.text.trim(),
      'uuid': _uuidCtrl.text.trim(),
      'interval_min': interval,
      if (_passCtrl.text.trim().isNotEmpty) 'password': _passCtrl.text.trim(),
    };
    try {
      await widget.api.update(body);
      if (!mounted) return;
      _passCtrl.clear();
      _toast('CookieCloud 配置已保存');
    } catch (e) {
      _toast('保存失败：$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      final r = await widget.api.sync();
      _toast('同步完成：更新 ${r.updated}，新增 ${r.added}，跳过 ${r.skipped}');
    } catch (e) {
      _toast('同步失败：$e');
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  String _fmtTime(int unix) {
    final d = DateTime.fromMillisecondsSinceEpoch(unix * 1000);
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
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
              const Icon(Icons.cloud_sync, size: 28, color: Color(0xFF0EA5E9)),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('CookieCloud 同步',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '浏览器装 CookieCloud 扩展并填服务器/UUID/密码后，定时拉取最新 cookie，'
            '按域名覆盖本地已失效站点 + 自动新增匹配内置定义的站点。',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
          if (widget.initial.lastError.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text('上次错误：${widget.initial.lastError}',
                  style: const TextStyle(fontSize: 11, color: PTColors.danger)),
            )
          else if (widget.initial.lastSyncUnix > 0)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text('上次同步：${_fmtTime(widget.initial.lastSyncUnix)}',
                  style: TextStyle(fontSize: 11, color: context.dimText)),
            ),
          const SizedBox(height: 12),
          _field(
            '服务器',
            TextField(
              controller: _serverCtrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: 'https://cc.example.com',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            'UUID / Key',
            TextField(
              controller: _uuidCtrl,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '端到端密码',
            TextField(
              controller: _passCtrl,
              obscureText: true,
              decoration: InputDecoration(
                isDense: true,
                hintText: widget.initial.passwordConfigured ? '已配置（留空=不修改）' : '密码',
                border: const OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '同步间隔（分钟）',
            TextField(
              controller: _intervalCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              FilledButton.tonalIcon(
                onPressed: _syncing ? null : _sync,
                icon: _syncing
                    ? const SizedBox(
                        width: 16, height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.sync, size: 18),
                label: const Text('立即同步'),
              ),
              const SizedBox(width: 8),
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
        ],
      ),
    );
  }

  Widget _field(String label, Widget child, {String? hint}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          if (hint != null)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child:
                  Text(hint, style: TextStyle(fontSize: 11, color: context.dimText)),
            ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}
