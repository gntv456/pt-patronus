// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 media_server_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/media_server/media_server_page.dart';

/// 添加 / 编辑表单。
class _MediaServerForm extends StatefulWidget {
  final MediaServer? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _MediaServerForm({this.initial, required this.onSubmit});
  @override
  State<_MediaServerForm> createState() => _MediaServerFormState();
}

class _MediaServerFormState extends State<_MediaServerForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _urlCtrl = TextEditingController(text: widget.initial?.url ?? '');
  late final _tokenCtrl = TextEditingController(); // 编辑时不回显秘密
  late final _userCtrl = TextEditingController(text: widget.initial?.username ?? '');
  late final _directLinkBaseCtrl =
      TextEditingController(text: widget.initial?.directLinkBase ?? '');
  // late 字段惰性求值，可在初始化式引用 widget.initial（同 brushflow 表单范式）。
  late String _type = widget.initial?.type ?? 'emby';
  late bool _isDefault = widget.initial?.isDefault ?? false;
  late bool _refreshOnDownload = widget.initial?.refreshOnDownload ?? true;
  late bool _notifyOnPlay = widget.initial?.notifyOnPlay ?? false;
  late bool _directLinkEnabled = widget.initial?.directLinkEnabled ?? false;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _urlCtrl.dispose();
    _tokenCtrl.dispose();
    _userCtrl.dispose();
    _directLinkBaseCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) =>
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入名称');
      return;
    }
    if (_urlCtrl.text.trim().isEmpty) {
      _toast('请输入服务器地址');
      return;
    }
    if (widget.initial == null && _tokenCtrl.text.trim().isEmpty) {
      _toast('新建需填写 API Key / Token');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'type': _type,
      'url': _urlCtrl.text.trim(),
      'username': _userCtrl.text.trim(),
      'is_default': _isDefault,
      'refresh_on_download': _refreshOnDownload,
      'notify_on_play': _notifyOnPlay,
      'direct_link_enabled': _directLinkEnabled,
      'direct_link_base': _directLinkBaseCtrl.text.trim(),
      if (_tokenCtrl.text.trim().isNotEmpty) 'token': _tokenCtrl.text.trim(),
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _field('名称', TextField(
          controller: _nameCtrl,
          decoration: const InputDecoration(isDense: true, hintText: '如：客厅 Emby', border: OutlineInputBorder()),
        )),
        _field('类型', DropdownButton<String>(
          value: _type,
          isExpanded: true,
          items: const [
            DropdownMenuItem(value: 'emby', child: Text('Emby')),
            DropdownMenuItem(value: 'jellyfin', child: Text('Jellyfin')),
            DropdownMenuItem(value: 'plex', child: Text('Plex')),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'emby'),
        )),
        _field('地址', TextField(
          controller: _urlCtrl,
          decoration: const InputDecoration(isDense: true, hintText: 'http://192.168.1.10:8096', border: OutlineInputBorder()),
        )),
        _field('API Key / Token', TextField(
          controller: _tokenCtrl,
          obscureText: true,
          decoration: InputDecoration(
            isDense: true,
            hintText: widget.initial == null ? 'Emby/JF API Key 或 Plex X-Plex-Token' : '留空=保留原凭据',
            border: const OutlineInputBorder(),
          ),
        )),
        _field('用户名（可选）', TextField(
          controller: _userCtrl,
          decoration: const InputDecoration(isDense: true, hintText: 'Emby/JF 管理员（通知目标）', border: OutlineInputBorder()),
        )),
        _switchRow('设为默认', _isDefault, (v) => setState(() => _isDefault = v)),
        _switchRow('下载完成刷新', _refreshOnDownload, (v) => setState(() => _refreshOnDownload = v)),
        _switchRow('播放通知', _notifyOnPlay, (v) => setState(() => _notifyOnPlay = v)),
        _switchRow('启用直链反代', _directLinkEnabled, (v) => setState(() => _directLinkEnabled = v)),
        if (_directLinkEnabled)
          _field('直链基址', TextField(
            controller: _directLinkBaseCtrl,
            decoration: const InputDecoration(
              isDense: true,
              hintText: 'https://cdn.example.com/library',
              border: OutlineInputBorder(),
            ),
          )),
        const SizedBox(height: 8),
        FilledButton(
          onPressed: _saving ? null : _submit,
          child: _saving
              ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
              : const Text('保存'),
        ),
      ],
    );
  }

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(value: value, onChanged: onChanged),
        ],
      ),
    );
  }
}
