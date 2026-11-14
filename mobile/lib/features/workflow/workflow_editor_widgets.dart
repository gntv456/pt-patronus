// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// WorkflowEditorPage 的展示层扩展（无 setState 段；从 workflow_editor_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/workflow/workflow_editor_page.dart';

extension on _WorkflowEditorPageState {
  Widget _metaCard() =>
      GlassCard(padding: const EdgeInsets.all(10), child: _metaFields());

  Widget _connectBanner() => Container(
    width: double.infinity,
    color: PTColors.primary.withValues(alpha: 0.15),
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
    child: const Text('点选目标节点完成连线，或点源节点取消', style: TextStyle(fontSize: 12)),
  );

  Widget _narrowActionBar() {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 10),
        decoration: BoxDecoration(
          color: isDark ? const Color(0xFF1E2024) : Colors.white,
          border: Border(
            top: BorderSide(color: Colors.grey.withValues(alpha: 0.18)),
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: OutlinedButton.icon(
                onPressed: _openPalette,
                icon: const Icon(Icons.add, size: 18),
                label: const Text('添加节点'),
              ),
            ),
            const SizedBox(width: 8),
            IconButton.filledTonal(
              tooltip: '运行',
              onPressed: _run,
              icon: const Icon(Icons.play_arrow),
            ),
            const SizedBox(width: 8),
            FilledButton.icon(
              onPressed: _saving ? null : _save,
              icon: _saving
                  ? const SizedBox(
                      width: 16,
                      height: 16,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Icon(Icons.save_outlined, size: 18),
              label: const Text('保存'),
            ),
          ],
        ),
      ),
    );
  }

  Widget _canvasWidget() => InteractiveViewer(
    transformationController: _trans,
    minScale: 0.3,
    maxScale: 2.5,
    boundaryMargin: const EdgeInsets.all(500),
    constrained: false,
    child: SizedBox(
      width: WorkflowEditorOpsMixin._canvas.width,
      height: WorkflowEditorOpsMixin._canvas.height,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          Semantics(
            // 连线为纯视觉装饰（节点/把手自身有语义），对读屏隐藏
            excludeSemantics: true,
            child: CustomPaint(
              size: WorkflowEditorOpsMixin._canvas,
              painter: _EdgePainter(
              edges: _edges,
              nodes: {for (final n in _nodes) n.id: n},
              nodeSize: const Size(WorkflowEditorOpsMixin._nodeW, WorkflowEditorOpsMixin._nodeH),
              color: Theme.of(context).colorScheme.primary,
            ),
          ),
          ),
          for (final n in _nodes) _nodeWidget(n),
        ],
      ),
    ),
  );

  // 左栏：节点库（按分类分组的 ActionChip，点击即添加到画布）。
  Widget _palettePanel() {
    final cats = <String, List<NodeInfo>>{};
    for (final a in _infos.values) {
      (cats[a.category] ??= []).add(a);
    }
    return SizedBox(
      width: 210,
      child: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          const Text(
            '节点库',
            style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 8),
          for (final entry in cats.entries) ...[
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 6),
              child: Text(
                entry.key,
                style: TextStyle(
                  fontWeight: FontWeight.w700,
                  color: context.dimText,
                ),
              ),
            ),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: entry.value
                  .map(
                    (a) => ActionChip(
                      label: Text(a.name),
                      onPressed: () => _addNode(a.type),
                    ),
                  )
                  .toList(),
            ),
          ],
        ],
      ),
    );
  }

  Color _catColor(String type) {
    final info = _infos[type];
    final m = {
      '资源': PTColors.primary2,
      '过滤': PTColors.warn,
      '下载': PTColors.success,
      '通知': Color(0xFFE100FF),
      '数据': PTColors.primary,
      '站点': Color(0xFF00B894),
      '逻辑': PTColors.danger,
      '流程': Color(0xFF0F9B0F),
      '媒体': Color(0xFFE84393),
      '文件': Color(0xFF0984E3),
    };
    return (info != null ? m[info.category] : null) ?? PTColors.primary;
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }


  Future<void> _run() async {
    if (widget.workflowId <= 0) {
      _toast('请先保存');
      return;
    }
    try {
      final run = await _api.run(widget.workflowId);
      if (!mounted) return;
      showDialog(
        context: context,
        builder: (_) => AlertDialog(
          title: Text('执行${run.state == 'success' ? '完成' : '失败'}'),
          content: SizedBox(
            width: double.maxFinite,
            child: ListView(
              shrinkWrap: true,
              children: [
                Text(run.result, style: const TextStyle(fontSize: 13)),
                const Divider(),
                ...run.nodeExecutions.map(
                  (l) => ListTile(
                    dense: true,
                    leading: Icon(
                      l.status == 'success'
                          ? Icons.check_circle
                          : l.status == 'failed'
                          ? Icons.cancel
                          : Icons.skip_next,
                      color: l.status == 'success'
                          ? PTColors.success
                          : l.status == 'failed'
                          ? PTColors.danger
                          : Colors.grey,
                      size: 20,
                    ),
                    title: Text(
                      l.nodeName,
                      style: const TextStyle(fontSize: 13),
                    ),
                    subtitle: Text(
                      '${l.durationMs}ms${l.message.isEmpty ? '' : ' · ${l.message}'}',
                      style: const TextStyle(fontSize: 11),
                    ),
                  ),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('关闭'),
            ),
          ],
        ),
      );
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _confirmExit(bool didPop, Object? result) async {
    if (didPop) return;
    final navigator = Navigator.of(context);
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('放弃改动？'),
        content: const Text('工作流有未保存的改动，确定离开吗？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('继续编辑'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('放弃'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    navigator.pop();
  }

  Future<void> _openTemplates() async {
    try {
      final tpls = await _api.templates();
      if (!mounted) return;
      final chosen = await showModalBottomSheet<WorkflowTemplate>(
        context: context,
        builder: (_) => ListView(
          padding: const EdgeInsets.all(16),
          children: [
            const Text(
              '从模板创建',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            ...tpls.map(
              (t) => Card(
                child: ListTile(
                  title: Text(t.name),
                  subtitle: Text(
                    t.description,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                  ),
                  trailing: const Icon(Icons.chevron_right),
                  onTap: () => Navigator.pop(context, t),
                ),
              ),
            ),
          ],
        ),
      );
      if (chosen != null) _applyTemplate(chosen);
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _openPalette() async {
    final cats = <String, List<NodeInfo>>{};
    for (final a in _infos.values) {
      (cats[a.category] ??= []).add(a);
    }
    final selected = await showModalBottomSheet<String>(
      context: context,
      isScrollControlled: true,
      builder: (_) => DraggableScrollableSheet(
        expand: false,
        minChildSize: 0.3,
        initialChildSize: 0.6,
        maxChildSize: 0.9,
        builder: (context, controller) => ListView(
          controller: controller,
          padding: const EdgeInsets.all(16),
          children: [
            const Text(
              '添加节点',
              style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 12),
            for (final entry in cats.entries) ...[
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 6),
                child: Text(
                  entry.key,
                  style: TextStyle(
                    fontWeight: FontWeight.w700,
                    color: context.dimText,
                  ),
                ),
              ),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: entry.value
                    .map(
                      (a) => ActionChip(
                        label: Text(a.name),
                        onPressed: () => Navigator.pop(context, a.type),
                      ),
                    )
                    .toList(),
              ),
            ],
          ],
        ),
      ),
    );
    if (selected != null) _addNode(selected);
  }

}
