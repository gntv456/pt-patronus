// 从 cloud_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/cloud/cloud_page.dart';

/// STRM 规则表单（添加/编辑）。
class _RuleForm extends StatefulWidget {
  final List<CloudDrive> drives;
  final StrmRule? initial;
  final Future<bool> Function(Map<String, dynamic>) onSubmit;
  final Future<String?> Function(CloudDrive) onBrowse;
  const _RuleForm({required this.drives, this.initial, required this.onSubmit, required this.onBrowse});
  @override
  State<_RuleForm> createState() => _RuleFormState();
}

class _RuleFormState extends State<_RuleForm> {
  late final _nameCtrl = TextEditingController(text: widget.initial?.name ?? '');
  late final _cloudCtrl = TextEditingController(text: widget.initial?.cloudPath ?? '/');
  late final _outCtrl = TextEditingController(text: widget.initial?.outputDir ?? '');
  late final _mountCtrl = TextEditingController(text: widget.initial?.mountRoot ?? '');
  late final _tplCtrl = TextEditingController(text: widget.initial?.template ?? '');
  late int _driveId = widget.initial?.driveId ?? (widget.drives.isNotEmpty ? widget.drives.first.id : 0);
  late bool _recursive = widget.initial?.recursive ?? true;
  late bool _keepExt = widget.initial?.keepExt ?? true;
  late bool _enabled = widget.initial?.enabled ?? true;
  bool _saving = false;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _cloudCtrl.dispose();
    _outCtrl.dispose();
    _mountCtrl.dispose();
    _tplCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) => ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));

  Future<void> _browse() async {
    final d = widget.drives.firstWhere((x) => x.id == _driveId, orElse: () => widget.drives.first);
    final p = await widget.onBrowse(d);
    if (p != null && mounted) setState(() => _cloudCtrl.text = p);
  }

  Future<void> _submit() async {
    if (_nameCtrl.text.trim().isEmpty || _driveId == 0 || _outCtrl.text.trim().isEmpty) {
      _toast('名称/网盘/输出目录必填');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      'name': _nameCtrl.text.trim(),
      'drive_id': _driveId,
      'cloud_path': _cloudCtrl.text.trim(),
      'output_dir': _outCtrl.text.trim(),
      'mount_root': _mountCtrl.text.trim(),
      'template': _tplCtrl.text.trim(),
      'recursive': _recursive,
      'keep_ext': _keepExt,
      'enabled': _enabled,
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          _field('名称', TextField(controller: _nameCtrl, decoration: const InputDecoration(isDense: true, hintText: '如：电影 STRM', border: OutlineInputBorder()))),
          _field('网盘', DropdownButton<int>(
            value: _driveId,
            isExpanded: true,
            items: [for (final d in widget.drives) DropdownMenuItem(value: d.id, child: Text(d.name))],
            onChanged: (v) => setState(() => _driveId = v ?? 0),
          )),
          _field('云盘目录', Row(children: [
            Expanded(child: TextField(controller: _cloudCtrl, decoration: const InputDecoration(isDense: true, hintText: '/电影', border: OutlineInputBorder()))),
            const SizedBox(width: 8),
            OutlinedButton(onPressed: _browse, child: const Text('浏览')),
          ])),
          _field('输出目录', TextField(controller: _outCtrl, decoration: const InputDecoration(isDense: true, hintText: '/strm/电影', border: OutlineInputBorder()))),
          _field('挂载根', TextField(controller: _mountCtrl, decoration: const InputDecoration(isDense: true, hintText: '/CloudDrive2/115', border: OutlineInputBorder()))),
          _field('路径模板', TextField(controller: _tplCtrl, decoration: const InputDecoration(isDense: true, hintText: '留空=默认 {mount}{cloudpath}', border: OutlineInputBorder()))),
          _switchRow('递归扫描', _recursive, (v) => setState(() => _recursive = v)),
          _switchRow('保留扩展名', _keepExt, (v) => setState(() => _keepExt = v)),
          _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
          const SizedBox(height: 8),
          FilledButton(
            onPressed: _saving ? null : _submit,
            child: _saving
                ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                : const Text('保存'),
          ),
        ],
      ),
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
