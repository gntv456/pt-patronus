import 'package:ptpatronus/core/api_client.dart';

// 工作流：可视化节点图引擎（对齐后端 internal/workflow + api/v1/workflow.go）。

class Position {
  final double x, y;
  const Position({this.x = 0, this.y = 0});
  Map<String, dynamic> toJson() => {'x': x, 'y': y};
  factory Position.fromJson(Map<String, dynamic> j) => Position(
        x: (j['x'] as num?)?.toDouble() ?? 0,
        y: (j['y'] as num?)?.toDouble() ?? 0,
      );
}

class GraphNode {
  final String id;
  final String type;
  String name;
  Map<String, dynamic> params;
  Position position;
  GraphNode({
    required this.id,
    required this.type,
    required this.name,
    Map<String, dynamic>? params,
    this.position = const Position(),
  }) : params = params ?? {};
  Map<String, dynamic> toJson() => {
        'id': id,
        'type': type,
        'name': name,
        'params': params,
        'position': position.toJson(),
      };
  factory GraphNode.fromJson(Map<String, dynamic> j) => GraphNode(
        id: j['id'] as String? ?? '',
        type: j['type'] as String? ?? '',
        name: j['name'] as String? ?? '',
        params: Map<String, dynamic>.from(j['params'] as Map? ?? {}),
        position: j['position'] is Map
            ? Position.fromJson(Map<String, dynamic>.from(j['position'] as Map))
            : const Position(),
      );
}

class GraphEdge {
  final String id;
  final String source;
  final String target;
  final String sourcePort;
  final String targetPort;
  GraphEdge({
    required this.id,
    required this.source,
    required this.target,
    this.sourcePort = '',
    this.targetPort = '',
  });
  Map<String, dynamic> toJson() => {
        'id': id,
        'source': source,
        'target': target,
        if (sourcePort.isNotEmpty) 'source_port': sourcePort,
        if (targetPort.isNotEmpty) 'target_port': targetPort,
      };
  factory GraphEdge.fromJson(Map<String, dynamic> j) => GraphEdge(
        id: j['id'] as String? ?? '',
        source: j['source'] as String? ?? '',
        target: j['target'] as String? ?? '',
        sourcePort: j['source_port'] as String? ?? '',
        targetPort: j['target_port'] as String? ?? '',
      );
}

class ParamOption {
  final String label;
  final dynamic value;
  const ParamOption({required this.label, required this.value});
  factory ParamOption.fromJson(Map<String, dynamic> j) =>
      ParamOption(label: j['label'] as String? ?? '', value: j['value']);
}

class ParamField {
  final String name, label, type;
  final List<ParamOption> options;
  final bool required;
  final dynamic defaultValue;
  final String help;
  const ParamField({
    required this.name,
    required this.label,
    required this.type,
    this.options = const [],
    this.required = false,
    this.defaultValue,
    this.help = '',
  });
  factory ParamField.fromJson(Map<String, dynamic> j) => ParamField(
        name: j['name'] as String? ?? '',
        label: j['label'] as String? ?? '',
        type: j['type'] as String? ?? 'text',
        options: ((j['options'] as List?) ?? const [])
            .map((e) => ParamOption.fromJson(e as Map<String, dynamic>))
            .toList(),
        required: j['required'] as bool? ?? false,
        defaultValue: j['default'],
        help: j['help'] as String? ?? '',
      );
}

class NodeInfo {
  final String type, name, category, description, icon;
  final Map<String, dynamic> defaultParams;
  final List<ParamField> paramSchema;
  const NodeInfo({
    required this.type,
    required this.name,
    required this.category,
    required this.description,
    required this.icon,
    required this.defaultParams,
    required this.paramSchema,
  });
  factory NodeInfo.fromJson(Map<String, dynamic> j) => NodeInfo(
        type: j['type'] as String? ?? '',
        name: j['name'] as String? ?? '',
        category: j['category'] as String? ?? '',
        description: j['description'] as String? ?? '',
        icon: j['icon'] as String? ?? '',
        defaultParams: Map<String, dynamic>.from(j['default_params'] as Map? ?? {}),
        paramSchema: ((j['param_schema'] as List?) ?? const [])
            .map((e) => ParamField.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class Workflow {
  final int id;
  final String name;
  final String description;
  final String triggerType;
  final String timer;
  final String eventType;
  final bool enabled;
  final String state;
  final int runCount;
  final String? lastRunAt;
  final String lastResult;
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;
  Workflow({
    required this.id,
    required this.name,
    this.description = '',
    this.triggerType = 'manual',
    this.timer = '',
    this.eventType = '',
    this.enabled = false,
    this.state = 'idle',
    this.runCount = 0,
    this.lastRunAt,
    this.lastResult = '',
    this.nodes = const [],
    this.edges = const [],
  });
  factory Workflow.fromJson(Map<String, dynamic> j) => Workflow(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        description: j['description'] as String? ?? '',
        triggerType: j['trigger_type'] as String? ?? 'manual',
        timer: j['timer'] as String? ?? '',
        eventType: j['event_type'] as String? ?? '',
        enabled: j['enabled'] as bool? ?? false,
        state: j['state'] as String? ?? 'idle',
        runCount: (j['run_count'] as num?)?.toInt() ?? 0,
        lastRunAt: j['last_run_at'] as String?,
        lastResult: j['last_result'] as String? ?? '',
        nodes: ((j['nodes'] as List?) ?? const [])
            .map((e) => GraphNode.fromJson(e as Map<String, dynamic>))
            .toList(),
        edges: ((j['edges'] as List?) ?? const [])
            .map((e) => GraphEdge.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class LogEntry {
  final String nodeId, nodeType, nodeName, status, message;
  final int durationMs;
  final String at;
  const LogEntry({
    this.nodeId = '',
    this.nodeType = '',
    this.nodeName = '',
    this.status = '',
    this.message = '',
    this.durationMs = 0,
    this.at = '',
  });
  factory LogEntry.fromJson(Map<String, dynamic> j) => LogEntry(
        nodeId: j['node_id'] as String? ?? '',
        nodeType: j['node_type'] as String? ?? '',
        nodeName: j['node_name'] as String? ?? '',
        status: j['status'] as String? ?? '',
        message: j['message'] as String? ?? '',
        durationMs: (j['duration_ms'] as num?)?.toInt() ?? 0,
        at: j['at'] as String? ?? '',
      );
}

class WorkflowRun {
  final int id, workflowId, durationMs;
  final String state, trigger, result;
  final String startedAt;
  final List<LogEntry> nodeExecutions;
  const WorkflowRun({
    this.id = 0,
    this.workflowId = 0,
    this.durationMs = 0,
    this.state = '',
    this.trigger = '',
    this.result = '',
    this.startedAt = '',
    this.nodeExecutions = const [],
  });
  factory WorkflowRun.fromJson(Map<String, dynamic> j) => WorkflowRun(
        id: (j['id'] as num?)?.toInt() ?? 0,
        workflowId: (j['workflow_id'] as num?)?.toInt() ?? 0,
        durationMs: (j['duration_ms'] as num?)?.toInt() ?? 0,
        state: j['state'] as String? ?? '',
        trigger: j['trigger'] as String? ?? '',
        result: j['result'] as String? ?? '',
        startedAt: j['started_at'] as String? ?? '',
        nodeExecutions: ((j['node_executions'] as List?) ?? const [])
            .map((e) => LogEntry.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

/// 工作流模板（GET /workflows/templates）。
class WorkflowTemplate {
  final String key, name, description, category, triggerType, timer;
  final List<GraphNode> nodes;
  final List<GraphEdge> edges;
  const WorkflowTemplate({
    this.key = '',
    this.name = '',
    this.description = '',
    this.category = '',
    this.triggerType = 'manual',
    this.timer = '',
    this.nodes = const [],
    this.edges = const [],
  });
  factory WorkflowTemplate.fromJson(Map<String, dynamic> j) => WorkflowTemplate(
        key: j['key'] as String? ?? '',
        name: j['name'] as String? ?? '',
        description: j['description'] as String? ?? '',
        category: j['category'] as String? ?? '',
        triggerType: j['trigger_type'] as String? ?? 'manual',
        timer: j['timer'] as String? ?? '',
        nodes: ((j['nodes'] as List?) ?? const [])
            .map((e) => GraphNode.fromJson(e as Map<String, dynamic>))
            .toList(),
        edges: ((j['edges'] as List?) ?? const [])
            .map((e) => GraphEdge.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

/// 节点耗时（统计看板）。
class NodeTiming {
  final String nodeType, nodeName;
  final int count, totalMs, avgMs, failed;
  const NodeTiming({
    this.nodeType = '',
    this.nodeName = '',
    this.count = 0,
    this.totalMs = 0,
    this.avgMs = 0,
    this.failed = 0,
  });
  factory NodeTiming.fromJson(Map<String, dynamic> j) => NodeTiming(
        nodeType: j['node_type'] as String? ?? '',
        nodeName: j['node_name'] as String? ?? '',
        count: (j['count'] as num?)?.toInt() ?? 0,
        totalMs: (j['total_ms'] as num?)?.toInt() ?? 0,
        avgMs: (j['avg_ms'] as num?)?.toInt() ?? 0,
        failed: (j['failed'] as num?)?.toInt() ?? 0,
      );
}

/// 运行统计（GET /workflows/stats）。
class WorkflowStats {
  final int total, enabled, runsTotal, runsSuccess, runsFailed;
  final int avgDurationMs;
  final List<NodeTiming> nodeTiming;
  const WorkflowStats({
    this.total = 0,
    this.enabled = 0,
    this.runsTotal = 0,
    this.runsSuccess = 0,
    this.runsFailed = 0,
    this.avgDurationMs = 0,
    this.nodeTiming = const [],
  });
  factory WorkflowStats.fromJson(Map<String, dynamic> j) => WorkflowStats(
        total: (j['total'] as num?)?.toInt() ?? 0,
        enabled: (j['enabled'] as num?)?.toInt() ?? 0,
        runsTotal: (j['runs_total'] as num?)?.toInt() ?? 0,
        runsSuccess: (j['runs_success'] as num?)?.toInt() ?? 0,
        runsFailed: (j['runs_failed'] as num?)?.toInt() ?? 0,
        avgDurationMs: (j['avg_duration_ms'] as num?)?.toInt() ?? 0,
        nodeTiming: ((j['node_timing'] as List?) ?? const [])
            .map((e) => NodeTiming.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class WorkflowApi {
  final _api = ApiClient();

  Future<List<Workflow>> list() async {
    final r = await _api.get('/workflows');
    final list = (r['workflows'] as List?) ?? const [];
    return list.map((e) => Workflow.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Workflow> get(int id) async {
    final r = await _api.get('/workflows/$id');
    return Workflow.fromJson(r);
  }

  Future<List<NodeInfo>> actions() async {
    final r = await _api.get('/workflows/actions');
    final list = (r['actions'] as List?) ?? const [];
    return list.map((e) => NodeInfo.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<WorkflowTemplate>> templates() async {
    final r = await _api.get('/workflows/templates');
    final list = (r['templates'] as List?) ?? const [];
    return list.map((e) => WorkflowTemplate.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<WorkflowStats> stats() async {
    final r = await _api.get('/workflows/stats');
    return WorkflowStats.fromJson(r);
  }

  Future<Workflow> create(Map<String, dynamic> body) async {
    final r = await _api.postJson('/workflows', body);
    return Workflow.fromJson(r);
  }

  Future<void> update(int id, Map<String, dynamic> body) => _api.put('/workflows/$id', body);

  Future<void> remove(int id) => _api.delete('/workflows/$id');

  Future<WorkflowRun> run(int id) async {
    final r = await _api.post('/workflows/$id/run', {});
    final run = r['run'];
    if (run is Map<String, dynamic>) return WorkflowRun.fromJson(run);
    throw ApiException(0, r['error'] as String? ?? '执行失败');
  }

  Future<void> enable(int id) => _api.post('/workflows/$id/enable', {});

  Future<void> disable(int id) => _api.post('/workflows/$id/disable', {});

  Future<List<WorkflowRun>> runs(int workflowId, {int limit = 20}) async {
    final r = await _api.get('/workflow-runs?workflow_id=$workflowId&limit=$limit');
    final list = (r['runs'] as List?) ?? const [];
    return list.map((e) => WorkflowRun.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<WorkflowRun> runDetail(int rid) async {
    final r = await _api.get('/workflow-runs/$rid');
    return WorkflowRun.fromJson(r);
  }
}
