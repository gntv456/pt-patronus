// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// WorkflowEditorPage 的 WorkflowEditorOpsMixin mixin（含 setState 段；从 workflow_editor_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/workflow/workflow_editor_page.dart';

mixin WorkflowEditorOpsMixin on State<WorkflowEditorPage> {

  bool _saving = false; // 保存防重复点击（双击并发 create 会建重复工作流——审计 P1）
  bool _dirty = false; // 画布有未保存改动：返回键拦截确认（避免丢节点编排）
  final _api = WorkflowApi();
  final _trans = TransformationController();
  static const _canvas = Size(2200, 1500);
  static const double _nodeW = 160, _nodeH = 74;
  List<GraphNode> _nodes = [];
  List<GraphEdge> _edges = [];
  Map<String, NodeInfo> _infos = {};
  bool _loading = true;
  // 工作流元信息
  final _nameCtl = TextEditingController();
  final _descCtl = TextEditingController();
  final _timerCtl = TextEditingController(text: '*/30 * * * *');
  String _triggerType = 'manual';
  final _eventTypeCtl = TextEditingController();
  bool _enabled = false;
  String? _selectedId;
  GraphNode? _connectFrom; // 连接态源节点
  String _connectPort = ''; // 条件分支端口

  // 宿主提供的钩子。
  Future<String?> _pickBranch();
  Map<String, dynamic> _body();
  void _toast(String s);
  Future<void> _load() async {
    try {
      final actions = await _api.actions();
      _infos = {for (final a in actions) a.type: a};
      if (widget.workflowId > 0) {
        final wf = await _api.get(widget.workflowId);
        _nameCtl.text = wf.name;
        _descCtl.text = wf.description;
        _triggerType = wf.triggerType;
        _timerCtl.text = wf.timer.isEmpty ? '*/30 * * * *' : wf.timer;
        _eventTypeCtl.text = wf.eventType;
        _enabled = wf.enabled;
        _nodes = wf.nodes;
        _edges = wf.edges;
      }
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) {
        setState(() {
          _loading = false;
          _dirty = false;
        });
      }
    }
  }

  void _addNode(String type) {
    final info = _infos[type];
    if (info == null) return;
    // 简单错开放置
    final n = _nodes.length;
    setState(() {
      _dirty = true;
      _nodes.add(
        GraphNode(
          id: '$type-${DateTime.now().millisecondsSinceEpoch}',
          type: type,
          name: info.name,
          params: Map<String, dynamic>.from(info.defaultParams),
          position: Position(
            x: 120 + (n % 5) * 200.0,
            y: 100 + (n ~/ 5) * 140.0,
          ),
        ),
      );
    });
  }

  void _onNodeTap(GraphNode n) {
    if (_connectFrom != null) {
      // 完成连线
      if (n.id != _connectFrom!.id) {
        setState(() {
          _dirty = true;
          _edges.add(
            GraphEdge(
              id: 'e-${DateTime.now().millisecondsSinceEpoch}',
              source: _connectFrom!.id,
              target: n.id,
              sourcePort: _connectPort,
            ),
          );
        });
      }
      setState(() {
        _connectFrom = null;
        _connectPort = '';
      });
    } else {
      setState(() => _selectedId = n.id);
    }
  }

  Future<void> _startConnect(GraphNode n) async {
    String port = '';
    if (n.type == 'condition') {
      port = await _pickBranch() ?? '';
      if (port.isEmpty) return;
    }
    if (!mounted) return;
    setState(() {
      _connectFrom = n;
      _connectPort = port;
      _selectedId = n.id;
    });
    _toast('已选源节点，点目标节点连线（再次点源取消）');
  }

  Future<void> _configNode(GraphNode n) async {
    final info = _infos[n.type];
    if (info == null) return;
    final saved = await showNodeConfigSheet(context, n, info);
    if (saved && mounted) setState(() => _dirty = true);
  }

  void _deleteNode(GraphNode n) {
    setState(() {
      _dirty = true;
      _nodes.removeWhere((e) => e.id == n.id);
      _edges.removeWhere((e) => e.source == n.id || e.target == n.id);
      if (_selectedId == n.id) _selectedId = null;
    });
  }

  Future<void> _save() async {
    if (_saving) return;
    if (_nameCtl.text.trim().isEmpty) {
      _toast('请填写名称');
      return;
    }
    setState(() => _saving = true);
    try {
      if (widget.workflowId > 0) {
        await _api.update(widget.workflowId, _body());
      } else {
        final created = await _api.create(_body());
        _toast('已保存 #${created.id}');
        if (mounted) Navigator.pop(context, true);
        return;
      }
      _toast('已保存');
      _dirty = false;
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  void _applyTemplate(WorkflowTemplate t) {
    setState(() {
      _nameCtl.text = t.name;
      _descCtl.text = t.description;
      _triggerType = t.triggerType;
      _timerCtl.text = t.timer.isEmpty ? '*/30 * * * *' : t.timer;
      _enabled = false;
      _nodes = List<GraphNode>.from(t.nodes);
      _edges = List<GraphEdge>.from(t.edges);
    });
    _toast('已载入模板，编辑后保存');
  }
}
