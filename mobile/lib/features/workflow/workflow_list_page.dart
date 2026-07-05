import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/config.dart';
import 'package:ptpatronus/api/workflow.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'workflow_editor_page.dart';

class WorkflowListPage extends StatefulWidget {
  const WorkflowListPage({super.key});
  @override
  State<WorkflowListPage> createState() => _WorkflowListPageState();
}

class _WorkflowListPageState extends State<WorkflowListPage> {
  final _api = WorkflowApi();
  List<Workflow> _list = [];
  WorkflowStats? _stats;
  bool _loading = false;
  int? _running;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() => _loading = true);
    try {
      final results = await Future.wait([
        _api.list(),
        _api.stats().catchError((_) => WorkflowStats()),
      ]);
      _list = results[0] as List<Workflow>;
      _stats = results[1] as WorkflowStats;
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _run(Workflow w) async {
    setState(() => _running = w.id);
    try {
      final run = await _api.run(w.id);
      _toast('执行${run.state == 'success' ? '完成' : '失败'}：${run.result}');
      await _load();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _running = null);
    }
  }

  Future<void> _toggle(Workflow w) async {
    try {
      if (w.enabled) {
        await _api.disable(w.id);
      } else {
        await _api.enable(w.id);
      }
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _delete(Workflow w) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('确认删除'),
        content: Text('删除工作流「${w.name}」？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(w.id);
      await _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _edit(Workflow? w) async {
    final changed = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => WorkflowEditorPage(workflowId: w?.id ?? 0),
      ),
    );
    if (changed == true) _load();
  }

  // Webhook 管理入口（admin 专属，对齐 Web Workflow.vue 的「Webhook 入口」按钮 + 后端 RequireAdmin）。
  Future<void> _openWebhooks() async {
    await showAdaptiveSheet(
      context,
      title: 'Webhook 入口',
      wideWidth: 640,
      builder: (_) => _WebhookManager(workflows: _list),
    );
  }

  String _stateText(String s) =>
      const {
        'idle': '空闲',
        'running': '执行中',
        'success': '成功',
        'failed': '失败',
      }[s] ??
      s;
  Color _stateColor(String s) =>
      const {
        'idle': Colors.grey,
        'running': Color(0xFF2980B9),
        'success': PTColors.success,
        'failed': PTColors.danger,
      }[s] ??
      Colors.grey;
  String _triggerText(String s) =>
      const {'manual': '手动', 'timer': '定时', 'event': '事件'}[s] ?? s;

  Widget _statCell(String num, String lab, [Color? c]) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      Text(
        num,
        style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700, color: c),
      ),
      Text(lab, style: const TextStyle(fontSize: 11, color: Colors.grey)),
    ],
  );

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: '工作流',
      actions: [
        if (ApiClient().isAdmin)
          PTBarAction(
            onPressed: _openWebhooks,
            icon: Icons.webhook,
            label: 'Webhook 入口',
          ),
        PTBarAction(onPressed: () => _edit(null), icon: Icons.add, label: '新建'),
      ],
      body: _loading && _list.isEmpty
          ? const Center(child: CircularProgressIndicator())
          : _list.isEmpty
          ? const Center(
              child: Text('暂无工作流，点右上角新建', style: TextStyle(color: Colors.grey)),
            )
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView.builder(
                padding: const EdgeInsets.all(12),
                itemCount: _list.length + (_stats != null ? 1 : 0),
                itemBuilder: (_, i) {
                  if (_stats != null && i == 0) {
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: GlassCard(
                        padding: const EdgeInsets.all(10),
                        child: Row(
                          mainAxisAlignment: MainAxisAlignment.spaceAround,
                          children: [
                            _statCell('${_stats!.total}', '工作流'),
                            _statCell('${_stats!.enabled}', '启用'),
                            _statCell('${_stats!.runsTotal}', '运行'),
                            _statCell(
                              '${_stats!.runsSuccess}',
                              '成功',
                              PTColors.success,
                            ),
                            _statCell(
                              '${_stats!.runsFailed}',
                              '失败',
                              PTColors.danger,
                            ),
                          ],
                        ),
                      ),
                    );
                  }
                  final w = _list[i - (_stats != null ? 1 : 0)];
                  return Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: GlassCard(
                      padding: const EdgeInsets.all(14),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Expanded(
                                child: Text(
                                  w.name,
                                  style: const TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                              ),
                              Container(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 8,
                                  vertical: 2,
                                ),
                                decoration: BoxDecoration(
                                  color: _stateColor(
                                    w.state,
                                  ).withValues(alpha: 0.18),
                                  borderRadius: BorderRadius.circular(999),
                                ),
                                child: Text(
                                  _stateText(w.state),
                                  style: TextStyle(
                                    color: _stateColor(w.state),
                                    fontSize: 12,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                              ),
                              const SizedBox(width: 6),
                              Container(
                                padding: const EdgeInsets.symmetric(
                                  horizontal: 8,
                                  vertical: 2,
                                ),
                                decoration: BoxDecoration(
                                  color: PTColors.primary.withValues(
                                    alpha: 0.18,
                                  ),
                                  borderRadius: BorderRadius.circular(999),
                                ),
                                child: Text(
                                  _triggerText(w.triggerType),
                                  style: const TextStyle(
                                    color: PTColors.primary,
                                    fontSize: 12,
                                  ),
                                ),
                              ),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '运行 ${w.runCount} 次 · ${w.description.isEmpty ? '—' : w.description}',
                            style: const TextStyle(
                              fontSize: 12,
                              color: Colors.grey,
                            ),
                          ),
                          if (w.lastResult.isNotEmpty)
                            Padding(
                              padding: const EdgeInsets.only(top: 2),
                              child: Text(
                                '上次：${w.lastResult}',
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                                style: TextStyle(
                                  fontSize: 12,
                                  color: Theme.of(context).colorScheme.primary,
                                ),
                              ),
                            ),
                          const SizedBox(height: 8),
                          Wrap(
                            spacing: 8,
                            children: [
                              FilledButton.tonalIcon(
                                onPressed: _running == w.id
                                    ? null
                                    : () => _edit(w),
                                icon: const Icon(Icons.edit, size: 16),
                                label: const Text('编辑'),
                              ),
                              FilledButton.icon(
                                onPressed: _running == w.id
                                    ? null
                                    : () => _run(w),
                                icon: _running == w.id
                                    ? const SizedBox(
                                        width: 14,
                                        height: 14,
                                        child: CircularProgressIndicator(
                                          strokeWidth: 2,
                                        ),
                                      )
                                    : const Icon(Icons.play_arrow, size: 18),
                                label: const Text('运行'),
                              ),
                              OutlinedButton(
                                onPressed: () => _toggle(w),
                                child: Text(w.enabled ? '禁用' : '启用'),
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
                },
              ),
            ),
    );
  }
}

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
              style: const TextStyle(fontSize: 12, color: Colors.grey),
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
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(child: CircularProgressIndicator()),
          )
        else if (_list.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text('暂无 Webhook', style: TextStyle(color: Colors.grey)),
            ),
          )
        else
          ..._list.map(_card),
      ],
    );
  }

  Widget _card(Webhook w) {
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
                    w.enabled ? PTColors.primary : Colors.grey,
                  ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              '触发 ${w.triggerCount} 次${w.lastTriggeredAt != null ? ' · ${_fmtTime(w.lastTriggeredAt!)}' : ''}',
              style: const TextStyle(fontSize: 11, color: Colors.grey),
            ),
            const SizedBox(height: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
              decoration: BoxDecoration(
                color: Colors.grey.withValues(alpha: 0.10),
                borderRadius: BorderRadius.circular(8),
              ),
              child: Row(
                children: [
                  Expanded(
                    child: SelectableText(
                      _url(w),
                      style: const TextStyle(fontSize: 11, color: Colors.grey),
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
