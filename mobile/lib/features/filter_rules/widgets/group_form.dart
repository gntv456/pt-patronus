// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'package:flutter/material.dart';
import 'package:ptpatronus/api/filter_rules.dart';

/// 规则组新增 / 编辑表单。渲染为 showAdaptiveSheet 内容。
///
/// [initial] 空 → 新增（name 可编辑）；非空 → 编辑（name 只读，PUT :name 用原值定位）。
/// rule_string 语法：`>` 分优先级层（左=最高优先），层内 `! & | ()` 为规则 id 的布尔表达式。
class FilterGroupForm extends StatefulWidget {
  final FilterRuleGroupInput? initial;
  final Future<bool> Function(FilterRuleGroupInput input) onSave;

  const FilterGroupForm({super.key, this.initial, required this.onSave});

  @override
  State<FilterGroupForm> createState() => _FilterGroupFormState();
}

class _FilterGroupFormState extends State<FilterGroupForm> {
  static const _mediaTypes = {'': '不限', '电影': '电影', '电视剧': '电视剧'};

  late final TextEditingController _nameCtrl;
  late final TextEditingController _ruleCtrl;
  late final TextEditingController _categoryCtrl;
  String _mediaType = '';
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    final i = widget.initial;
    _nameCtrl = TextEditingController(text: i?.name ?? '');
    _ruleCtrl = TextEditingController(text: i?.ruleString ?? '');
    _categoryCtrl = TextEditingController(text: i?.category ?? '');
    _mediaType = (i?.mediaType ?? '');
    if (!_mediaTypes.containsKey(_mediaType)) _mediaType = '';
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _ruleCtrl.dispose();
    _categoryCtrl.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    final name = _nameCtrl.text.trim();
    final rule = _ruleCtrl.text.trim();
    if (name.isEmpty) {
      _toast('请填写规则组名称');
      return;
    }
    if (rule.isEmpty) {
      _toast('请填写 rule_string');
      return;
    }
    setState(() => _saving = true);
    final input = FilterRuleGroupInput(
      name: name,
      ruleString: rule,
      mediaType: _mediaType,
      category: _categoryCtrl.text.trim(),
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
      TextField(
        controller: _nameCtrl,
        enabled: !isEdit,
        decoration: const InputDecoration(labelText: '名称 *', isDense: true),
      ),
      const SizedBox(height: 8),
      DropdownButton<String>(
        value: _mediaType,
        isExpanded: true,
        items: _mediaTypes.entries
            .map((e) => DropdownMenuItem(value: e.key, child: Text(e.value)))
            .toList(),
        onChanged: (v) => setState(() => _mediaType = v ?? ''),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _categoryCtrl,
        decoration: const InputDecoration(labelText: '分类（可选）', isDense: true),
      ),
      const SizedBox(height: 8),
      TextField(
        controller: _ruleCtrl,
        maxLines: 3,
        decoration: const InputDecoration(
          labelText: 'rule_string *',
          alignLabelWithHint: true,
          isDense: true,
          helperText: '> 分优先级层（左=最高）；层内 ! & | () 连接规则 id',
          hintText: '例：4K & !BLU > 1080P',
        ),
        style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
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
