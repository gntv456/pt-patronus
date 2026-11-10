// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 contract_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/contract/contract_page.dart';

/// 新建 / 编辑保种契约的表单（放进 showAdaptiveSheet）。
/// 自持下拉 / 开关 / 文本框状态；[onSubmit] 返回 true 时表单自关闭。
class _ContractForm extends StatefulWidget {
  final List<Site> sites;
  final ContractRule? initial;
  final Future<bool> Function(Map<String, dynamic> body) onSubmit;

  const _ContractForm({
    required this.sites,
    required this.onSubmit,
    this.initial,
  });

  @override
  State<_ContractForm> createState() => _ContractFormState();
}

class _ContractFormState extends State<_ContractForm> {
  late final _ratioCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minRatio}',
  );
  late final _seedingCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minSeeding}',
  );
  late final _uploadCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minUploadGb}',
  );
  late final _sizeCtrl = TextEditingController(
    text: widget.initial == null ? '' : '${widget.initial!.minSeedingSizeGb}',
  );

  int? _siteId;
  bool _enabled = true;
  bool _officialOnly = false;
  String _period = '';
  DateTime? _startTime;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final ini = widget.initial;
    if (ini != null) {
      _siteId =
          widget.sites.any((s) => s.id == ini.siteId) ? ini.siteId : null;
      _enabled = ini.enabled;
      _officialOnly = ini.officialOnly;
      _period = ini.period;
      _startTime = ini.startTime;
    } else {
      _siteId = widget.sites.isNotEmpty ? widget.sites.first.id : null;
    }
  }

  @override
  void dispose() {
    _ratioCtrl.dispose();
    _seedingCtrl.dispose();
    _uploadCtrl.dispose();
    _sizeCtrl.dispose();
    super.dispose();
  }

  double _parseDouble(String s, [double def = 0]) =>
      double.tryParse(s.trim()) ?? def;
  int _parseInt(String s, [int def = 0]) => int.tryParse(s.trim()) ?? def;

  Future<void> _pickStartTime() async {
    final now = DateTime.now();
    final d = await showDatePicker(
      context: context,
      initialDate: _startTime ?? now,
      firstDate: DateTime(now.year - 1),
      lastDate: DateTime(now.year + 5),
    );
    if (d == null || !mounted) return;
    final t = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(_startTime ?? now),
    );
    if (t == null || !mounted) return;
    setState(() {
      _startTime = DateTime(d.year, d.month, d.day, t.hour, t.minute);
    });
  }

  String _fmtStartTime(DateTime d) {
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit() async {
    if (_siteId == null) {
      _toast('请选择站点');
      return;
    }
    setState(() => _saving = true);
    final body = <String, dynamic>{
      if (widget.initial != null) 'id': widget.initial!.id,
      'site_id': _siteId,
      'enabled': _enabled,
      'min_ratio': _parseDouble(_ratioCtrl.text),
      'min_seeding': _parseInt(_seedingCtrl.text),
      'min_upload_gb': _parseInt(_uploadCtrl.text),
      'official_only': _officialOnly,
      'min_seeding_size_gb': _parseInt(_sizeCtrl.text),
      'period': _period,
      if (_startTime != null)
        'start_time': _startTime!.toUtc().toIso8601String(),
    };
    final ok = await widget.onSubmit(body);
    if (!mounted) return;
    setState(() => _saving = false);
    if (ok) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final editing = widget.initial != null;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        _field(
          '站点',
          DropdownButton<int>(
            value: _siteId,
            isExpanded: true,
            hint: const Text('选择站点'),
            items: widget.sites
                .map((s) => DropdownMenuItem(value: s.id, child: Text(s.name)))
                .toList(),
            onChanged:
                editing ? null : (v) => setState(() => _siteId = v),
          ),
        ),
        _switchRow('启用', _enabled, (v) => setState(() => _enabled = v)),
        const Divider(height: 20),
        const Text(
          '保种下限',
          style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
        ),
        const SizedBox(height: 8),
        _field(
          '最低分享率',
          TextField(
            controller: _ratioCtrl,
            keyboardType:
                const TextInputType.numberWithOptions(decimal: true),
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 1.0',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '最少做种数',
          TextField(
            controller: _seedingCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 5',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '最低上传（GiB）',
          TextField(
            controller: _uploadCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 100',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        const Divider(height: 24),
        const Align(
          alignment: Alignment.centerLeft,
          child: Text(
            '扩展维度（官种 / 体积 / 周期 / 生效时间）',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 13),
          ),
        ),
        const SizedBox(height: 8),
        _switchRow(
          '仅官种（做种数/体积只计官种）',
          _officialOnly,
          (v) => setState(() => _officialOnly = v),
        ),
        _field(
          '做种体积下限（GiB）',
          TextField(
            controller: _sizeCtrl,
            keyboardType: TextInputType.number,
            decoration: const InputDecoration(
              isDense: true,
              hintText: '如 500',
              border: OutlineInputBorder(),
            ),
          ),
        ),
        _field(
          '考核周期',
          DropdownButton<String>(
            value: _period,
            isExpanded: true,
            items: const [
              DropdownMenuItem(value: '', child: Text('每日')),
              DropdownMenuItem(value: 'weekly', child: Text('每周')),
              DropdownMenuItem(value: 'monthly', child: Text('每月')),
            ],
            onChanged: (v) => setState(() => _period = v ?? ''),
          ),
        ),
        _field(
          '生效时间',
          InkWell(
            onTap: _pickStartTime,
            child: InputDecorator(
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
              child: Text(
                _startTime == null
                    ? '留空 = 立即生效'
                    : _fmtStartTime(_startTime!),
                style: TextStyle(
                  color: _startTime == null ? context.dimText : null,
                ),
              ),
            ),
          ),
        ),
        const SizedBox(height: 12),
        FilledButton.icon(
          onPressed: _saving ? null : _submit,
          icon: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Icon(Icons.save_outlined, size: 18),
          label: Text(editing ? '保存修改' : '创建契约'),
        ),
      ],
    );
  }

  /// 统一的「标签 + 控件」字段行。
  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: context.dimText,
            ),
          ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  /// 统一的开关行。
  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 4),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(
            value: value,
            onChanged: onChanged,
            materialTapTargetSize: MaterialTapTargetSize.padded,
          ),
        ],
      ),
    );
  }
}
