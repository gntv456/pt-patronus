import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/core/config.dart';
import 'package:ptpatronus/api/workflow.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import 'workflow_editor_page.dart';

part 'webhook_manager.dart';
part 'webhook_create_sheet.dart';

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
  Color _stateColor(BuildContext context, String s) =>
      {
        'idle': context.dimText,
        'running': Color(0xFF2980B9),
        'success': PTColors.success,
        'failed': PTColors.danger,
      }[s] ??
      context.dimText;
  String _triggerText(String s) =>
      const {'manual': '手动', 'timer': '定时', 'event': '事件'}[s] ?? s;

  Widget _statCell(BuildContext context, String num, String lab, [Color? c]) => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      Text(
        num,
        style: TextStyle(fontSize: 20, fontWeight: FontWeight.w700, color: c),
      ),
      Text(lab, style: TextStyle(fontSize: 11, color: context.dimText)),
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
          ? const PTLoadingView()
          : _list.isEmpty
          ? Center(
              child: Text('暂无工作流，点右上角新建', style: TextStyle(color: context.dimText)),
            )
          : RefreshIndicator(
              onRefresh: _load,
              child: ListView.builder(
                padding: const EdgeInsets.all(12),
                itemCount: _list.length + (_stats != null ? 1 : 0),
                itemBuilder: (context, i) {
                  if (_stats != null && i == 0) {
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 10),
                      child: GlassCard(
                        padding: const EdgeInsets.all(10),
                        child: Row(
                          mainAxisAlignment: MainAxisAlignment.spaceAround,
                          children: [
                            _statCell(context, '${_stats!.total}', '工作流'),
                            _statCell(context, '${_stats!.enabled}', '启用'),
                            _statCell(context, '${_stats!.runsTotal}', '运行'),
                            _statCell(context,
                              '${_stats!.runsSuccess}',
                              '成功',
                              PTColors.success,
                            ),
                            _statCell(context,
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
                                  color: _stateColor(context, 
                                    w.state,
                                  ).withValues(alpha: 0.18),
                                  borderRadius: BorderRadius.circular(999),
                                ),
                                child: Text(
                                  _stateText(w.state),
                                  style: TextStyle(
                                    color: _stateColor(context, w.state),
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
                            style: TextStyle(
                              fontSize: 12,
                              color: context.dimText,
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
