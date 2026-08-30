// 从 cloud_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/cloud/cloud_page.dart';

/// 网盘表单（添加/编辑）。
class _DriveForm extends StatefulWidget {
  final CloudDrive? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  const _DriveForm({this.initial, required this.onSubmit});
  @override
  State<_DriveForm> createState() => _DriveFormState();
}

class _DriveFormState extends State<_DriveForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _tokenCtrl = TextEditingController();
  late final _rootCtrl = TextEditingController(text: widget.initial?.rootPath ?? '/');
  late String _type = widget.initial?.type ?? '115';
  late bool _isDefault = widget.initial?.isDefault ?? false;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _tokenCtrl.dispose();
    _rootCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty) {
      _toast('请输入名称');
      return;
    }
    if (widget.initial == null && _tokenCtrl.text.trim().isEmpty) {
      _toast('新建需填写 Cookie / Token');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'type': _type,
      'root_path': _rootCtrl.text.trim(),
      'is_default': _isDefault,
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
        _field('名称', TextField(controller: _nameCtrl, decoration: const InputDecoration(isDense: true, hintText: '如：我的 115', border: OutlineInputBorder()))),
        _field('类型', DropdownButton<String>(
          value: _type,
          isExpanded: true,
          items: const [
            DropdownMenuItem(value: '115', child: Text('115网盘')),
            DropdownMenuItem(value: 'quark', child: Text('夸克网盘')),
            DropdownMenuItem(value: '123', child: Text('123云盘')),
          ],
          onChanged: (v) => setState(() => _type = v ?? '115'),
        )),
        _field('Cookie / Token', TextField(
          controller: _tokenCtrl,
          maxLines: 3,
          decoration: InputDecoration(
            isDense: true,
            hintText: widget.initial == null ? '115/夸克填整段 cookie，123 填 accessToken' : '留空=保留原凭据',
            border: const OutlineInputBorder(),
          ),
        )),
        _field('根目录', TextField(controller: _rootCtrl, decoration: const InputDecoration(isDense: true, hintText: '/（网盘根）', border: OutlineInputBorder()))),
        _switchRow('设为默认', _isDefault, (v) => setState(() => _isDefault = v)),
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

  Widget _field(String label, Widget child) => Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Text(label, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          const SizedBox(height: 6),
          child,
        ]),
      );

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) => Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(children: [Expanded(child: Text(label, style: const TextStyle(fontSize: 13))), Switch(value: value, onChanged: onChanged)]),
      );
}
