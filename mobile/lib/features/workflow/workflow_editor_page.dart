// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/workflow.dart';
import 'package:ptpatronus/core/responsive.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_motion.dart';
import 'node_config_sheet.dart';

part 'workflow_edge_painter.dart';

part 'workfloweditoropsmixin.dart';
part 'workflow_editor_widgets.dart';

/// 工作流编辑器（自绘节点画布）：InteractiveViewer 缩放平移 + Stack/Positioned 可拖拽节点 +
/// CustomPainter 贝塞尔连线。点节点的「连线」进入连接态，再点目标节点建边（条件节点选 真/假 分支）。
class WorkflowEditorPage extends StatefulWidget {
  final int workflowId; // 0=新建
  const WorkflowEditorPage({super.key, required this.workflowId});
  @override
  State<WorkflowEditorPage> createState() => _WorkflowEditorPageState();
}

class _WorkflowEditorPageState extends State<WorkflowEditorPage> with WorkflowEditorOpsMixin {
  @override
  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  @override
  Future<String?> _pickBranch() async {
    return showDialog<String>(
      context: context,
      builder: (_) => SimpleDialog(
        title: const Text('选择分支'),
        children: [
          SimpleDialogOption(
            onPressed: () => Navigator.pop(context, 'true'),
            child: const Text('真（条件成立）'),
          ),
          SimpleDialogOption(
            onPressed: () => Navigator.pop(context, 'false'),
            child: const Text('假（不成立）'),
          ),
        ],
      ),
    );
  }
  @override
  Map<String, dynamic> _body() {
    return {
      'name': _nameCtl.text.trim(),
      'description': _descCtl.text.trim(),
      'trigger_type': _triggerType,
      'timer': _timerCtl.text.trim(),
      'event_type': _eventTypeCtl.text.trim(),
      'enabled': _enabled,
      'nodes': _nodes.map((n) => n.toJson()).toList(),
      'edges': _edges.map((e) => e.toJson()).toList(),
    };
  }
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _nameCtl.dispose();
    _descCtl.dispose();
    _timerCtl.dispose();
    _eventTypeCtl.dispose();
    _trans.dispose();
    super.dispose();
  }

  double get _scale {
    final s = _trans.value[0]; // m11（x 缩放）
    return s == 0 ? 1.0 : s;
  }

  bool _narrowFitApplied = false;

  @override
  Widget build(BuildContext context) {
    final wide = isWideScreen(context);
    if (!_loading && !wide && !_narrowFitApplied) {
      _narrowFitApplied = true;
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        _trans.value = Matrix4.diagonal3Values(0.68, 0.68, 1);
      });
    }
    return GlassBackground(
      child: PopScope(
        canPop: !_dirty,
        onPopInvokedWithResult: _confirmExit,
        child: Scaffold(
          backgroundColor: Colors.transparent,
          appBar: AppBar(
            title: Text(widget.workflowId > 0 ? '编辑工作流' : '新建工作流'),
            backgroundColor: Colors.transparent,
            elevation: 0,
            actions: [
              PTIconAction(
                icon: Icons.dynamic_form,
                tooltip: '从模板导入',
                onPressed: _openTemplates,
              ),
              PTIconAction(
                icon: Icons.play_arrow,
                tooltip: '运行',
                onPressed: _run,
              ),
              PTIconAction(
                icon: Icons.save,
                tooltip: '保存',
                onPressed: _saving ? null : _save,
                loading: _saving,
              ),
            ],
          ),
          body: _loading
              ? const PTLoadingView()
              : wide
              ? _wideBody()
              : _narrowBody(),
          bottomNavigationBar: (!wide && !_loading) ? _narrowActionBar() : null,
        ),
      ),
    );
  }

  // 窄屏：元信息卡 + 连接提示 + 画布（节点库/节点配置走 FAB 与节点 ⚙ 的 sheet）。
  Widget _narrowBody() => Column(
    children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
        child: _metaCard(),
      ),
      if (_connectFrom != null) _connectBanner(),
      Expanded(child: _canvasWidget()),
    ],
  );

  // 宽屏三栏（对齐 Web WorkflowEditor.vue）：左节点库 | 中画布 | 右配置面板。
  Widget _wideBody() => Row(
    children: [
      _palettePanel(),
      const VerticalDivider(width: 1),
      Expanded(
        child: Column(
          children: [
            if (_connectFrom != null) _connectBanner(),
            Expanded(child: _canvasWidget()),
          ],
        ),
      ),
      const VerticalDivider(width: 1),
      _configPanel(),
    ],
  );

  // 工作流元信息字段（名称/触发/定时/事件/启用）——窄屏包 GlassCard，宽屏嵌右面板。
  Widget _metaFields() => Column(
    mainAxisSize: MainAxisSize.min,
    children: [
      TextField(
        controller: _nameCtl,
        decoration: const InputDecoration(
          isDense: true,
          labelText: '名称',
          border: OutlineInputBorder(),
        ),
      ),
      const SizedBox(height: 8),
      Row(
        children: [
          const Text('触发：'),
          DropdownButton<String>(
            value: _triggerType,
            items: const [
              DropdownMenuItem(value: 'manual', child: Text('手动')),
              DropdownMenuItem(value: 'timer', child: Text('定时')),
              DropdownMenuItem(value: 'event', child: Text('事件')),
            ],
            onChanged: (v) => setState(() => _triggerType = v ?? 'manual'),
          ),
          const SizedBox(width: 8),
          if (_triggerType == 'timer')
            Expanded(
              child: TextField(
                controller: _timerCtl,
                decoration: const InputDecoration(
                  isDense: true,
                  hintText: '*/30 * * * *',
                ),
              ),
            ),
          if (_triggerType == 'event')
            Expanded(
              child: TextField(
                controller: _eventTypeCtl,
                decoration: const InputDecoration(
                  isDense: true,
                  hintText: '事件类型',
                ),
              ),
            ),
          Switch(
            value: _enabled,
            onChanged: (v) => setState(() => _enabled = v),
          ),
          const Text('启用', style: TextStyle(fontSize: 12)),
        ],
      ),
    ],
  );

  // 右栏：工作流元信息 + 选中节点的参数表单（内联 NodeParamForm，实时写回驱动画布重绘）。
  Widget _configPanel() {
    final sel = _selectedId == null
        ? null
        : _nodes.where((n) => n.id == _selectedId).firstOrNull;
    final info = sel == null ? null : _infos[sel.type];
    return SizedBox(
      width: 320,
      child: ListView(
        padding: const EdgeInsets.all(12),
        children: [
          _metaFields(),
          const SizedBox(height: 12),
          const Divider(height: 1),
          const SizedBox(height: 10),
          if (sel != null && info != null) ...[
            Row(
              children: [
                const Text(
                  '节点配置',
                  style: TextStyle(fontSize: 14, fontWeight: FontWeight.w700),
                ),
                const Spacer(),
                GestureDetector(
                  onTap: () => _deleteNode(sel),
                  child: const Icon(
                    Icons.delete_outline,
                    size: 18,
                    color: PTColors.danger,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            // key 按节点 id：切换选中时强制重建表单，控制器读取新节点初值。
            NodeParamForm(
              key: ValueKey(sel.id),
              node: sel,
              info: info,
              onChanged: () => setState(() {}),
            ),
          ] else
            Padding(
              padding: EdgeInsets.symmetric(vertical: 24),
              child: Center(
                child: Text(
                  '点选画布节点查看/编辑参数',
                  style: TextStyle(color: context.dimText, fontSize: 12),
                ),
              ),
            ),
        ],
      ),
    );
  }

  Widget _nodeWidget(GraphNode n) {
    final selected = _selectedId == n.id;
    final connecting = _connectFrom?.id == n.id;
    final info = _infos[n.type];
    final color = _catColor(n.type);
    return Positioned(
      left: n.position.x,
      top: n.position.y,
      width: WorkflowEditorOpsMixin._nodeW,
      child: GestureDetector(
        onTap: () => _onNodeTap(n),
        onPanUpdate: (d) {
          setState(() {
            n.position = Position(
              x: math.max(0, n.position.x + d.delta.dx / _scale),
              y: math.max(0, n.position.y + d.delta.dy / _scale),
            );
          });
        },
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 120),
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: Theme.of(context).colorScheme.surface,
            borderRadius: BorderRadius.circular(10),
            border: Border.all(
              color: connecting
                  ? PTColors.danger
                  : (selected ? color : color.withValues(alpha: 0.5)),
              width: selected || connecting ? 2.5 : 1.5,
            ),
            boxShadow: const [
              BoxShadow(
                color: Colors.black26,
                blurRadius: 6,
                offset: Offset(0, 3),
              ),
            ],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                children: [
                  Icon(Icons.circle, size: 10, color: color),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      n.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 13,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 2),
              Text(
                info?.category ?? '',
                style: TextStyle(fontSize: 11, color: context.dimText),
              ),
              const SizedBox(height: 4),
              Row(
                children: [
                  Icon(Icons.south, size: 12, color: context.dimText),
                  const SizedBox(width: 4),
                  GestureDetector(
                    onTap: () => _startConnect(n),
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 6,
                        vertical: 2,
                      ),
                      decoration: BoxDecoration(
                        color: color.withValues(alpha: 0.15),
                        borderRadius: BorderRadius.circular(6),
                      ),
                      child: const Text('连线', style: TextStyle(fontSize: 11)),
                    ),
                  ),
                  const Spacer(),
                  GestureDetector(
                    onTap: () => _configNode(n),
                    child: Icon(
                      Icons.settings,
                      size: 15,
                      color: context.dimText,
                    ),
                  ),
                  const SizedBox(width: 6),
                  GestureDetector(
                    onTap: () => _deleteNode(n),
                    child: const Icon(
                      Icons.delete_outline,
                      size: 15,
                      color: PTColors.danger,
                    ),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }

}
