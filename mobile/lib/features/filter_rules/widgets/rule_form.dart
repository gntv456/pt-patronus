import 'package:flutter/material.dart';
import 'package:ptpatronus/api/filter_rules.dart';

/// 自定义规则新增 / 编辑表单。渲染为 showAdaptiveSheet 内容（标题 + 滚动由外层负责）。
///
/// [initial] 为空 → 新增模式（id 可编辑）；非空 → 编辑模式（id 只读，避免破坏 rule_string 引用）。
/// [onSave] 返回 true 则关闭表单。字段语义对齐后端 filter.CustomRuleInput。
class FilterRuleForm extends StatefulWidget {
  final FilterCustomRuleInput? initial;
  final Future<bool> Function(FilterCustomRuleInput input) onSave;

  const FilterRuleForm({super.key, this.initial, required this.onSave});

  @override
  State<FilterRuleForm> createState() => _FilterRuleFormState();
}

class _FilterRuleFormState extends State<FilterRuleForm> {
  late final TextEditingController _idCtrl;
  late final TextEditingController _nameCtrl;
  late final TextEditingController _includeCtrl;
  late final TextEditingController _excludeCtrl;
  late final TextEditingController _sizeCtrl;
  late final TextEditingController _seedersCtrl;
  late final TextEditingController _publishCtrl;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final i = widget.initial;
    _idCtrl = TextEditingController(text: i?.id ?? '');
    _nameCtrl = TextEditingController(text: i?.name ?? '');
    _includeCtrl = TextEditingController(text: i?.include ?? '');
    _excludeCtrl = TextEditingController(text: i?.exclude ?? '');
    _sizeCtrl = TextEditingController(text: i?.sizeRange ?? '');
    _seedersCtrl = TextEditingController(text: (i != null && i.seeders > 0) ? '${i.seeders}' : '');
    _publishCtrl = TextEditingController(text: i?.publishTime ?? '');
  }

  @override
  void dispose() {
    _idCtrl.dispose();
    _nameCtrl.dispose();
    _includeCtrl.dispose();
    _excludeCtrl.dispose();
    _sizeCtrl.dispose();
    _seedersCtrl.dispose();
    _publishCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final id = _idCtrl.text.trim();
    if (id.isEmpty) {
      _toast('请填写规则 id');
      return;
    }
    setState(() => _saving = true);
    final input = FilterCustomRuleInput(
      id: id,
      name: _nameCtrl.text.trim(),
      include: _includeCtrl.text.trim(),
      exclude: _excludeCtrl.text.trim(),
      sizeRange: _sizeCtrl.text.trim(),
      seeders: int.tryParse(_seedersCtrl.text.trim()) ?? 0,
      publishTime: _publishCtrl.text.trim(),
    );
    try {
      final ok = await widget.onSave(input);
      if (ok && mounted) Navigator.of(context).pop(true);
    } catch (e) {
      _toast('保存失败：$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 2)),
    );
  }

  @override
  Widget build(BuildContext context) {
    final isEdit = widget.initial != null;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      // id：编辑时只读（rule_string 引用 id，改名会断链）。
      TextField(
        controller: _idCtrl,
        enabled: !isEdit,
        decoration: const InputDecoration(
          labelText: '规则 id *',
          isDense: true,
          helperText: '仅字母数字，用作规则组表达式中的 token',
        ),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _nameCtrl,
        decoration: const InputDecoration(labelText: '名称', isDense: true),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _includeCtrl,
        decoration: const InputDecoration(
          labelText: '包含（正则）',
          isDense: true,
          helperText: '命中才保留；多选项写 a|b',
        ),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _excludeCtrl,
        decoration: const InputDecoration(
          labelText: '排除（正则）',
          isDense: true,
          helperText: '命中即拒绝',
        ),
      ),
      const SizedBox(height: 8),
      Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Expanded(child: TextField(
          controller: _sizeCtrl,
          decoration: const InputDecoration(
            labelText: '体积范围(MB)',
            isDense: true,
            helperText: '1000-5000 / >1000 / <5000',
          ),
        )),
        const SizedBox(width: 8),
        Expanded(child: TextField(
          controller: _seedersCtrl,
          keyboardType: TextInputType.number,
          decoration: const InputDecoration(labelText: '最少做种', isDense: true),
        )),
      ]),
      const SizedBox(height: 8),
      TextField(
        controller: _publishCtrl,
        decoration: const InputDecoration(
          labelText: '发布时间(分钟)',
          isDense: true,
          helperText: '60（≥60分钟）或 60-1440（闭区间）',
        ),
      ),
      const SizedBox(height: 18),
      FilledButton(
        onPressed: _saving ? null : _submit,
        child: _saving
            ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
            : Text(isEdit ? '保存' : '新增'),
      ),
    ]);
  }
}
