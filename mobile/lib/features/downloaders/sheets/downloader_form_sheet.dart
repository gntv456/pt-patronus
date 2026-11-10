// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 downloaders_page.dart 拆出（part of 主文件，共享私有类型/工具函数；行为不变）。
part of 'package:ptpatronus/features/downloaders/downloaders_page.dart';

// 下载器添加/编辑表单（BottomSheet）：name/type/url/username/password/is_default。
// 编辑态 downloader 非 null，密码留空=不修改（api 仅非空才传）。新建态不显示「默认」开关（add 固定 false）。
class _DownloaderFormSheet extends StatefulWidget {
  final Downloader? downloader;
  const _DownloaderFormSheet({this.downloader});
  @override
  State<_DownloaderFormSheet> createState() => _DownloaderFormSheetState();
}

class _DownloaderFormSheetState extends State<_DownloaderFormSheet> {
  final _api = DownloaderApi();
  late final TextEditingController _name, _url, _username, _password;
  String _type = 'qb';
  bool _isDefault = false;
  bool _saving = false;

  bool get _isEdit => widget.downloader != null;

  @override
  void initState() {
    super.initState();
    final d = widget.downloader;
    _name = TextEditingController(text: d?.name ?? '');
    _url = TextEditingController(text: d?.url ?? '');
    _username = TextEditingController(text: d?.username ?? '');
    _password = TextEditingController(); // 编辑态留空=不修改
    _type = d?.type ?? 'qb';
    _isDefault = d?.isDefault ?? false;
  }

  @override
  void dispose() {
    _name.dispose();
    _url.dispose();
    _username.dispose();
    _password.dispose();
    super.dispose();
  }

  void _toast(String m) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(m)));
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    final url = _url.text.trim();
    if (name.isEmpty || url.isEmpty) {
      _toast('名称与地址必填');
      return;
    }
    if (!RegExp(r'^https?://.+').hasMatch(url)) {
      _toast('地址需以 http:// 或 https:// 开头');
      return;
    }
    setState(() => _saving = true);
    try {
      if (_isEdit) {
        await _api.update(
          widget.downloader!.id,
          name: name,
          type: _type,
          url: url,
          username: _username.text.trim(),
          password: _password.text,
          isDefault: _isDefault,
        );
        _toast('已更新');
      } else {
        await _api.add(
          name: name,
          type: _type,
          url: url,
          username: _username.text.trim(),
          password: _password.text,
        );
        _toast('已添加');
      }
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      _toast(e.toString().replaceFirst('Exception: ', ''));
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // 标题与键盘避让由 showAdaptiveSheet 浮层容器统一处理（宽屏对话框 / 窄屏底栏）。
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          decoration: const InputDecoration(labelText: '名称'),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<String>(
          initialValue: _type,
          decoration: const InputDecoration(labelText: '类型'),
          items: [
            DropdownMenuItem(value: 'qb', child: _dlOption(_dlMeta('qb'))),
            DropdownMenuItem(value: 'tr', child: _dlOption(_dlMeta('tr'))),
            DropdownMenuItem(value: 'ut', child: _dlOption(_dlMeta('ut'))),
            DropdownMenuItem(value: 'dl', child: _dlOption(_dlMeta('dl'))),
            DropdownMenuItem(value: 'ar', child: _dlOption(_dlMeta('ar'))),
            DropdownMenuItem(value: 'rt', child: _dlOption(_dlMeta('rt'))),
            DropdownMenuItem(value: 'fl', child: _dlOption(_dlMeta('fl'))),
          ],
          onChanged: (v) => setState(() => _type = v ?? 'qb'),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _url,
          decoration: const InputDecoration(
            labelText: '地址',
            hintText: 'http://127.0.0.1:8080',
          ),
          keyboardType: TextInputType.url,
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _username,
          decoration: const InputDecoration(labelText: '用户名'),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _password,
          obscureText: true,
          decoration: InputDecoration(
            labelText: '密码',
            hintText: _isEdit ? '留空不修改' : null,
          ),
        ),
        if (_isEdit)
          SwitchListTile(
            value: _isDefault,
            onChanged: (v) => setState(() => _isDefault = v),
            title: const Text('设为默认下载器'),
            contentPadding: EdgeInsets.zero,
            dense: true,
          ),
        const SizedBox(height: 20),
        Row(
          mainAxisAlignment: MainAxisAlignment.end,
          children: [
            TextButton(
              onPressed: _saving ? null : () => Navigator.pop(context, false),
              child: const Text('取消'),
            ),
            const SizedBox(width: 8),
            FilledButton(
              onPressed: _saving ? null : _submit,
              child: Text(_isEdit ? '保存' : '添加'),
            ),
          ],
        ),
      ],
    );
  }
}

