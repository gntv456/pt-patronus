import 'dart:math' as math;
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/workflow.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'node_config_sheet.dart';

/// 工作流编辑器（自绘节点画布）：InteractiveViewer 缩放平移 + Stack/Positioned 可拖拽节点 +
/// CustomPainter 贝塞尔连线。点节点的「连线」进入连接态，再点目标节点建边（条件节点选 真/假 分支）。
class WorkflowEditorPage extends StatefulWidget {
  final int workflowId; // 0=新建
  const WorkflowEditorPage({super.key, required this.workflowId});
  @override
  State<WorkflowEditorPage> createState() => _WorkflowEditorPageState();
}

class _WorkflowEditorPageState extends State<WorkflowEditorPage> {
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
      if (mounted) setState(() => _loading = false);
    }
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  double get _scale {
    final s = _trans.value[0]; // m11（x 缩放）
    return s == 0 ? 1.0 : s;
  }

  Color _catColor(String type) {
    final info = _infos[type];
    const m = {
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

  void _addNode(String type) {
    final info = _infos[type];
    if (info == null) return;
    // 简单错开放置
    final n = _nodes.length;
    setState(() {
      _nodes.add(GraphNode(
        id: '$type-${DateTime.now().millisecondsSinceEpoch}',
        type: type,
        name: info.name,
        params: Map<String, dynamic>.from(info.defaultParams),
        position: Position(x: 120 + (n % 5) * 200.0, y: 100 + (n ~/ 5) * 140.0),
      ));
    });
  }

  void _onNodeTap(GraphNode n) {
    if (_connectFrom != null) {
      // 完成连线
      if (n.id != _connectFrom!.id) {
        setState(() {
          _edges.add(GraphEdge(
            id: 'e-${DateTime.now().millisecondsSinceEpoch}',
            source: _connectFrom!.id,
            target: n.id,
            sourcePort: _connectPort,
          ));
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

  Future<String?> _pickBranch() async {
    return showDialog<String>(
      context: context,
      builder: (_) => SimpleDialog(
        title: const Text('选择分支'),
        children: [
          SimpleDialogOption(onPressed: () => Navigator.pop(context, 'true'), child: const Text('真（条件成立）')),
          SimpleDialogOption(onPressed: () => Navigator.pop(context, 'false'), child: const Text('假（不成立）')),
        ],
      ),
    );
  }

  Future<void> _configNode(GraphNode n) async {
    final info = _infos[n.type];
    if (info == null) return;
    final saved = await showNodeConfigSheet(context, n, info);
    if (saved && mounted) setState(() {});
  }

  void _deleteNode(GraphNode n) {
    setState(() {
      _nodes.removeWhere((e) => e.id == n.id);
      _edges.removeWhere((e) => e.source == n.id || e.target == n.id);
      if (_selectedId == n.id) _selectedId = null;
    });
  }

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

  Future<void> _save() async {
    if (_nameCtl.text.trim().isEmpty) {
      _toast('请填写名称');
      return;
    }
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
    } catch (e) {
      _toast('$e');
    }
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
                ...run.nodeExecutions.map((l) => ListTile(
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
                      title: Text(l.nodeName, style: const TextStyle(fontSize: 13)),
                      subtitle: Text(
                        '${l.durationMs}ms${l.message.isEmpty ? '' : ' · ${l.message}'}',
                        style: const TextStyle(fontSize: 11),
                      ),
                    )),
              ],
            ),
          ),
          actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('关闭'))],
        ),
      );
    } catch (e) {
      _toast('$e');
    }
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          title: Text(widget.workflowId > 0 ? '编辑工作流' : '新建工作流'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [
            IconButton(onPressed: _openTemplates, icon: const Icon(Icons.dynamic_form)),
            IconButton(onPressed: _run, icon: const Icon(Icons.play_arrow)),
            IconButton(onPressed: _save, icon: const Icon(Icons.save)),
          ],
        ),
        body: _loading
            ? const Center(child: CircularProgressIndicator())
            : Column(
                children: [
                  // 元信息条
                  Padding(
                    padding: const EdgeInsets.fromLTRB(12, 4, 12, 8),
                    child: GlassCard(
                      padding: const EdgeInsets.all(10),
                      child: Column(children: [
                        TextField(
                          controller: _nameCtl,
                          decoration: const InputDecoration(
                              isDense: true, labelText: '名称', border: OutlineInputBorder()),
                        ),
                        const SizedBox(height: 8),
                        Row(children: [
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
                                      isDense: true, hintText: '*/30 * * * *')),
                            ),
                          if (_triggerType == 'event')
                            Expanded(
                              child: TextField(
                                  controller: _eventTypeCtl,
                                  decoration: const InputDecoration(
                                      isDense: true, hintText: '事件类型')),
                            ),
                          Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
                          const Text('启用', style: TextStyle(fontSize: 12)),
                        ]),
                      ]),
                    ),
                  ),
                  // 连接态提示
                  if (_connectFrom != null)
                    Container(
                      width: double.infinity,
                      color: PTColors.primary.withValues(alpha: 0.15),
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                      child: const Text('点选目标节点完成连线，或点源节点取消',
                          style: TextStyle(fontSize: 12)),
                    ),
                  // 画布
                  Expanded(
                    child: InteractiveViewer(
                      transformationController: _trans,
                      minScale: 0.3,
                      maxScale: 2.5,
                      boundaryMargin: const EdgeInsets.all(500),
                      constrained: false,
                      child: SizedBox(
                        width: _canvas.width,
                        height: _canvas.height,
                        child: Stack(
                          clipBehavior: Clip.none,
                          children: [
                            CustomPaint(
                              size: _canvas,
                              painter: _EdgePainter(
                                edges: _edges,
                                nodes: {for (final n in _nodes) n.id: n},
                                nodeSize: const Size(_nodeW, _nodeH),
                                color: Theme.of(context).colorScheme.primary,
                              ),
                            ),
                            for (final n in _nodes) _nodeWidget(n),
                          ],
                        ),
                      ),
                    ),
                  ),
                ],
              ),
        floatingActionButton: FloatingActionButton(
          heroTag: 'wf-add',
          onPressed: _openPalette,
          child: const Icon(Icons.add),
        ),
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
      width: _nodeW,
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
                color: connecting ? PTColors.danger : (selected ? color : color.withValues(alpha: 0.5)),
                width: selected || connecting ? 2.5 : 1.5),
            boxShadow: const [BoxShadow(color: Colors.black26, blurRadius: 6, offset: Offset(0, 3))],
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(children: [
                Icon(Icons.circle, size: 10, color: color),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(n.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 13)),
                ),
              ]),
              const SizedBox(height: 2),
              Text(info?.category ?? '',
                  style: const TextStyle(fontSize: 10, color: Colors.grey)),
              const SizedBox(height: 4),
              Row(children: [
                Icon(Icons.south, size: 12, color: Colors.grey[600]),
                const SizedBox(width: 4),
                GestureDetector(
                  onTap: () => _startConnect(n),
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                    decoration: BoxDecoration(
                        color: color.withValues(alpha: 0.15), borderRadius: BorderRadius.circular(6)),
                    child: const Text('连线', style: TextStyle(fontSize: 10)),
                  ),
                ),
                const Spacer(),
                GestureDetector(
                  onTap: () => _configNode(n),
                  child: const Icon(Icons.settings, size: 15, color: Colors.grey),
                ),
                const SizedBox(width: 6),
                GestureDetector(
                  onTap: () => _deleteNode(n),
                  child: const Icon(Icons.delete_outline, size: 15, color: PTColors.danger),
                ),
              ]),
            ],
          ),
        ),
      ),
    );
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
            const Text('从模板创建', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
            const SizedBox(height: 12),
            ...tpls.map((t) => Card(
                  child: ListTile(
                    title: Text(t.name),
                    subtitle: Text(t.description, maxLines: 2, overflow: TextOverflow.ellipsis),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => Navigator.pop(context, t),
                  ),
                )),
          ],
        ),
      );
      if (chosen != null) _applyTemplate(chosen);
    } catch (e) {
      _toast('$e');
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
            const Text('添加节点', style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700)),
            const SizedBox(height: 12),
            for (final entry in cats.entries) ...[
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 6),
                child: Text(entry.key,
                    style: const TextStyle(fontWeight: FontWeight.w700, color: Colors.grey)),
              ),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: entry.value
                    .map((a) => ActionChip(
                          label: Text(a.name),
                          onPressed: () => Navigator.pop(context, a.type),
                        ))
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

/// 贝塞尔连线：源节点底中点 → 目标节点顶中点。
class _EdgePainter extends CustomPainter {
  final List<GraphEdge> edges;
  final Map<String, GraphNode> nodes;
  final Size nodeSize;
  final Color color;
  _EdgePainter({required this.edges, required this.nodes, required this.nodeSize, required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color.withValues(alpha: 0.7)
      ..strokeWidth = 2.4
      ..style = PaintingStyle.stroke;
    for (final e in edges) {
      final s = nodes[e.source];
      final t = nodes[e.target];
      if (s == null || t == null) continue;
      final start = Offset(s.position.x + nodeSize.width / 2, s.position.y + nodeSize.height);
      final end = Offset(t.position.x + nodeSize.width / 2, t.position.y);
      final mid = (start.dy + end.dy) / 2;
      final path = Path()
        ..moveTo(start.dx, start.dy)
        ..cubicTo(start.dx, mid, end.dx, mid, end.dx, end.dy);
      canvas.drawPath(path, paint);
      // 箭头
      const arrow = 6.0;
      canvas.drawPath(
        Path()
          ..moveTo(end.dx, end.dy)
          ..lineTo(end.dx - arrow / 2, end.dy - arrow)
          ..lineTo(end.dx + arrow / 2, end.dy - arrow)
          ..close(),
        paint..style = PaintingStyle.fill,
      );
      paint.style = PaintingStyle.stroke;
    }
  }

  @override
  bool shouldRepaint(covariant _EdgePainter old) =>
      old.edges != edges || old.nodes != nodes || old.color != color;
}
