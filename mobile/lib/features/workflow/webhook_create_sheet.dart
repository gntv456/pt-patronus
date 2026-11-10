// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 workflow_list_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/workflow/workflow_list_page.dart';

// Webhook 新建表单：name + 目标工作流（0=不直接触发）+ 可选 HMAC secret。
class _WebhookCreateSheet extends StatefulWidget {
  final List<Workflow> workflows;
  final WebhookApi api;
  const _WebhookCreateSheet({required this.workflows, required this.api});
  @override
  State<_WebhookCreateSheet> createState() => _WebhookCreateSheetState();
}

class _WebhookCreateSheetState extends State<_WebhookCreateSheet> {
  final _name = TextEditingController();
  final _secret = TextEditingController();
  int _target = 0; // 0=不直接触发
  bool _saving = false;

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _submit() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      _toast('请填写名称');
      return;
    }
    setState(() => _saving = true);
    try {
      await widget.api.create({
        'name': name,
        'target_workflow_id': _target,
        'secret': _secret.text.trim(),
        'enabled': true,
      });
      _toast('已创建');
      if (mounted) Navigator.pop(context, true);
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  void dispose() {
    _name.dispose();
    _secret.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        TextField(
          controller: _name,
          decoration: const InputDecoration(labelText: '名称'),
        ),
        const SizedBox(height: 12),
        DropdownButtonFormField<int>(
          initialValue: _target,
          decoration: const InputDecoration(labelText: '目标工作流'),
          items: [
            const DropdownMenuItem(value: 0, child: Text('不直接触发（仅记录/发事件）')),
            ...widget.workflows.map(
              (w) => DropdownMenuItem(value: w.id, child: Text(w.name)),
            ),
          ],
          onChanged: (v) => setState(() => _target = v ?? 0),
        ),
        const SizedBox(height: 12),
        TextField(
          controller: _secret,
          decoration: const InputDecoration(
            labelText: '签名密钥（可选）',
            hintText: 'HMAC 校验密钥，留空则不校验',
          ),
        ),
        const SizedBox(height: 20),
        FilledButton(
          onPressed: _saving ? null : _submit,
          child: _saving
              ? const SizedBox(
                  width: 16,
                  height: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: Colors.white,
                  ),
                )
              : const Text('创建'),
        ),
      ],
    );
  }
}
