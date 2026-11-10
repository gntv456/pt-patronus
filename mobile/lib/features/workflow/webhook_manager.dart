// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 workflow_list_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/workflow/workflow_list_page.dart';

// ── Webhook 管理面板（admin）：列表 + 复制 URL / 重生成 token / 删除 + 新建。对齐 Web Workflow.vue 的 webhook 区。──
class _WebhookManager extends StatefulWidget {
  final List<Workflow> workflows; // 新建表单的目标工作流下拉用
  const _WebhookManager({required this.workflows});
  @override
  State<_WebhookManager> createState() => _WebhookManagerState();
}

class _WebhookManagerState extends State<_WebhookManager> {
  final _api = WebhookApi();
  List<Webhook> _list = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _load() async {
    try {
      _list = await _api.list();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  String _url(Webhook w) => w.receiveUrl(Config.baseUrl);

  Future<void> _copy(Webhook w) async {
    await Clipboard.setData(ClipboardData(text: _url(w)));
    _toast('接收 URL 已复制');
  }

  Future<void> _rotate(Webhook w) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('重生成 Token'),
        content: const Text('重生成后旧 URL 立即失效，确认继续？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('重生成'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.rotate(w.id);
      _toast('已重生成');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _delete(Webhook w) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除 Webhook'),
        content: Text('删除「${w.name}」？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(backgroundColor: PTColors.danger),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(w.id);
      _toast('已删除');
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _create() async {
    final created = await showAdaptiveSheet<bool>(
      context,
      title: '新建 Webhook',
      builder: (_) =>
          _WebhookCreateSheet(workflows: widget.workflows, api: _api),
    );
    if (created == true) _load();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Text(
              '${_list.length} 个 Webhook',
              style: TextStyle(fontSize: 12, color: context.dimText),
            ),
            const Spacer(),
            OutlinedButton.icon(
              onPressed: _create,
              icon: const Icon(Icons.add, size: 16),
              label: const Text('新建'),
            ),
          ],
        ),
        const SizedBox(height: 10),
        if (_loading)
          const PTLoadingView()
        else if (_list.isEmpty)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text('暂无 Webhook', style: TextStyle(color: context.dimText)),
            ),
          )
        else
          ..._list.map((w) => _card(context, w)),
      ],
    );
  }

  Widget _card(BuildContext context, Webhook w) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    w.name,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 15,
                    ),
                  ),
                ),
                if (w.hasSecret)
                  _chip('已签名', PTColors.success)
                else
                  _chip(
                    w.enabled ? '启用' : '禁用',
                    w.enabled ? PTColors.primary : context.dimText,
                  ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              '触发 ${w.triggerCount} 次${w.lastTriggeredAt != null ? ' · ${_fmtTime(w.lastTriggeredAt!)}' : ''}',
              style: TextStyle(fontSize: 11, color: context.dimText),
            ),
            const SizedBox(height: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
              decoration: BoxDecoration(
                color: context.dimText.withValues(alpha: 0.10),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: SelectableText(
                      _url(w),
                      style: TextStyle(fontSize: 11, color: context.dimText),
                      maxLines: 1,
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.copy, size: 16),
                    visualDensity: VisualDensity.compact,
                    onPressed: () => _copy(w),
                    tooltip: '复制 URL',
                  ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 6,
              children: [
                OutlinedButton.icon(
                  onPressed: () => _rotate(w),
                  icon: const Icon(Icons.refresh, size: 16),
                  label: const Text('重生成'),
                ),
                TextButton(
                  onPressed: () => _delete(w),
                  child: const Text(
                    '删除',
                    style: TextStyle(color: PTColors.danger),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _chip(String t, Color c) => Container(
    margin: const EdgeInsets.only(left: 6),
    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
    decoration: BoxDecoration(
      color: c.withValues(alpha: 0.16),
      borderRadius: BorderRadius.circular(999),
    ),
    child: Text(
      t,
      style: TextStyle(color: c, fontSize: 11, fontWeight: FontWeight.w600),
    ),
  );

  String _fmtTime(String iso) {
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.month}-${d.day} ${p(d.hour)}:${p(d.minute)}';
  }
}
