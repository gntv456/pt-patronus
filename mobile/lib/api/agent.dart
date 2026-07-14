import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/core/api_client.dart';

// AgentConversation — 对话会话（对齐后端 models.AgentConversation）。
class AgentConversation {
  final int id;
  final String title;
  final String updatedAt;
  AgentConversation({
    required this.id,
    required this.title,
    required this.updatedAt,
  });
  factory AgentConversation.fromJson(Map<String, dynamic> j) =>
      AgentConversation(
        id: (j['id'] as num?)?.toInt() ?? 0,
        title: j['title'] as String? ?? '',
        updatedAt: j['updated_at'] as String? ?? '',
      );
}

// AgentMessage — 单条消息（含思考 + 工具调用）。
class AgentMessage {
  final int id;
  final String role; // user | assistant | tool
  final String content;
  final String reasoning;
  final String toolCalls; // JSON
  final String toolCallId;
  final String name;
  AgentMessage({
    required this.id,
    required this.role,
    required this.content,
    this.reasoning = '',
    this.toolCalls = '',
    this.toolCallId = '',
    this.name = '',
  });
  factory AgentMessage.fromJson(Map<String, dynamic> j) => AgentMessage(
    id: (j['id'] as num?)?.toInt() ?? 0,
    role: j['role'] as String? ?? '',
    content: j['content'] as String? ?? '',
    reasoning: j['reasoning'] as String? ?? '',
    toolCalls: j['tool_calls'] as String? ?? '',
    toolCallId: j['tool_call_id'] as String? ?? '',
    name: j['name'] as String? ?? '',
  );
}

class AgentConfig {
  bool enabled;
  String provider;
  String baseUrl;
  String model;
  double temperature;
  int maxTokens;
  bool enableThinking;
  int maxIterations;
  String systemPrompt;
  List<String> enabledTools;
  bool apiKeyConfigured;
  AgentConfig({
    required this.enabled,
    required this.provider,
    required this.baseUrl,
    required this.model,
    required this.temperature,
    required this.maxTokens,
    required this.enableThinking,
    required this.maxIterations,
    required this.systemPrompt,
    required this.enabledTools,
    required this.apiKeyConfigured,
  });
  factory AgentConfig.fromJson(Map<String, dynamic> j) => AgentConfig(
    enabled: j['enabled'] as bool? ?? false,
    provider: j['provider'] as String? ?? 'openai',
    baseUrl: j['base_url'] as String? ?? 'https://api.openai.com/v1',
    model: j['model'] as String? ?? 'gpt-4o-mini',
    temperature: (j['temperature'] as num?)?.toDouble() ?? 0.3,
    maxTokens: (j['max_tokens'] as num?)?.toInt() ?? 2048,
    enableThinking: j['enable_thinking'] as bool? ?? true,
    maxIterations: (j['max_iterations'] as num?)?.toInt() ?? 12,
    systemPrompt: j['system_prompt'] as String? ?? '',
    enabledTools: ((j['enabled_tools'] as List?) ?? const [])
        .map((e) => e.toString())
        .toList(),
    apiKeyConfigured: j['api_key_configured'] as bool? ?? false,
  );
  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'provider': provider,
    'base_url': baseUrl,
    'model': model,
    'temperature': temperature,
    'max_tokens': maxTokens,
    'enable_thinking': enableThinking,
    'max_iterations': maxIterations,
    'system_prompt': systemPrompt,
    'enabled_tools': enabledTools,
  };
}

// AgentEvent — SSE 事件（type: conversation/token/reasoning/tool_call/tool_result/done/error）。
class AgentEvent {
  final String event;
  final Map<String, dynamic> data;
  AgentEvent(this.event, this.data);
}

// ToolCatalogEntry — 工具目录条目（含 read/write/destructive 风险等级，供白名单 UI 分组）。
class ToolCatalogEntry {
  final String name;
  final String description;
  final String risk; // read | write | destructive
  final String riskNote;
  ToolCatalogEntry({required this.name, required this.description, required this.risk, required this.riskNote});
  factory ToolCatalogEntry.fromJson(Map<String, dynamic> j) => ToolCatalogEntry(
    name: j['name'] as String? ?? '',
    description: j['description'] as String? ?? '',
    risk: j['risk'] as String? ?? 'write',
    riskNote: j['risk_note'] as String? ?? '',
  );
}

// AgentUsageRecord — 单次对话用量记录。
class AgentUsageRecord {
  final int inputTokens;
  final int outputTokens;
  final double costUsd;
  final int durationMs;
  final String status;
  final int toolCalls;
  final int toolErrors;
  final String model;
  final String timestamp;
  AgentUsageRecord({required this.inputTokens, required this.outputTokens, required this.costUsd, required this.durationMs, required this.status, required this.toolCalls, required this.toolErrors, required this.model, required this.timestamp});
  factory AgentUsageRecord.fromJson(Map<String, dynamic> j) => AgentUsageRecord(
    inputTokens: (j['input_tokens'] as num?)?.toInt() ?? 0,
    outputTokens: (j['output_tokens'] as num?)?.toInt() ?? 0,
    costUsd: (j['cost_usd'] as num?)?.toDouble() ?? 0,
    durationMs: (j['duration_ms'] as num?)?.toInt() ?? 0,
    status: j['status'] as String? ?? '',
    toolCalls: (j['tool_calls'] as num?)?.toInt() ?? 0,
    toolErrors: (j['tool_errors'] as num?)?.toInt() ?? 0,
    model: j['model'] as String? ?? '',
    timestamp: j['timestamp'] as String? ?? '',
  );
}

// AgentStats — 用量统计快照（请求数/Token/费用/延迟/错误/工具成功率）。
class AgentStats {
  final int totalRequests;
  final int totalInputTokens;
  final int totalOutputTokens;
  final double totalCostUsd;
  final int errorCount;
  final int rateLimitCount;
  final int avgLatencyMs;
  final int toolCalls;
  final int toolErrors;
  AgentStats({required this.totalRequests, required this.totalInputTokens, required this.totalOutputTokens, required this.totalCostUsd, required this.errorCount, required this.rateLimitCount, required this.avgLatencyMs, required this.toolCalls, required this.toolErrors});
  factory AgentStats.fromJson(Map<String, dynamic> j) => AgentStats(
    totalRequests: (j['total_requests'] as num?)?.toInt() ?? 0,
    totalInputTokens: (j['total_input_tokens'] as num?)?.toInt() ?? 0,
    totalOutputTokens: (j['total_output_tokens'] as num?)?.toInt() ?? 0,
    totalCostUsd: (j['total_cost_usd'] as num?)?.toDouble() ?? 0,
    errorCount: (j['error_count'] as num?)?.toInt() ?? 0,
    rateLimitCount: (j['rate_limit_count'] as num?)?.toInt() ?? 0,
    avgLatencyMs: (j['avg_latency_ms'] as num?)?.toInt() ?? 0,
    toolCalls: (j['tool_calls'] as num?)?.toInt() ?? 0,
    toolErrors: (j['tool_errors'] as num?)?.toInt() ?? 0,
  );
}

class AgentApi {
  final ApiClient _api;
  final http.Client Function() _clientFactory;
  final String? _baseUrl;
  http.Client? _streamClient;

  AgentApi({
    ApiClient? api,
    http.Client Function()? clientFactory,
    String? baseUrl,
  }) : _api = api ?? ApiClient(),
       _clientFactory = clientFactory ?? http.Client.new,
       _baseUrl = baseUrl;

  Future<List<AgentConversation>> conversations() async {
    final r = await _api.get('/agent/conversations');
    return ((r['conversations'] as List?) ?? const [])
        .map((e) => AgentConversation.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<List<AgentMessage>> messages(int id) async {
    final r = await _api.get('/agent/conversations/$id/messages');
    return ((r['messages'] as List?) ?? const [])
        .map((e) => AgentMessage.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> deleteConversation(int id) =>
      _api.delete('/agent/conversations/$id');

  /// 工具目录（含风险等级），供白名单 UI 分组勾选。
  Future<List<ToolCatalogEntry>> tools() async {
    final r = await _api.get('/agent/tools');
    return ((r['tools'] as List?) ?? const [])
        .map((e) => ToolCatalogEntry.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// 用量统计快照（请求数/Token/费用/延迟/错误/工具成功率）。
  Future<AgentStats> stats() async => AgentStats.fromJson(await _api.get('/agent/stats'));
  Future<AgentConfig> getConfig() async =>
      AgentConfig.fromJson(await _api.get('/agent/config'));
  Future<void> setConfig(AgentConfig c, {String? apiKey}) => _api.put(
    '/agent/config',
    apiKey != null ? {...c.toJson(), 'api_key': apiKey} : c.toJson(),
  );

  /// 危险工具二次确认决断：convId 走 URL，tool_call_id（= confirm_request.data['id']）走 body。
  Future<void> confirmDecision(int convId, String toolCallId, bool approved) =>
      _api.post('/agent/conversations/$convId/confirm', {
        'tool_call_id': toolCallId,
        'approved': approved,
      });

  /// 流式 chat：yield SSE 事件。外部调 [cancel] 中断（关闭 _client → await-for 抛错）。
  Stream<AgentEvent> chat(String message, int? conversationId) async* {
    cancel();
    final client = _clientFactory();
    _streamClient = client;
    final uri = Uri.parse('${_baseUrl ?? _api.baseUrl}/api/v1/agent/chat');
    final req = http.Request('POST', uri)
      ..headers['Content-Type'] = 'application/json'
      ..headers['Authorization'] = 'Bearer ${_api.token ?? ''}'
      ..body = jsonEncode({
        'conversation_id': conversationId ?? 0,
        'message': message,
      });
    try {
      final resp = await client.send(req);
      if (resp.statusCode != 200) {
        final body = await resp.stream.bytesToString();
        throw Exception(
          'HTTP ${resp.statusCode}: ${body.length > 200 ? body.substring(0, 200) : body}',
        );
      }
      String buf = '';
      await for (final chunk in resp.stream.transform(utf8.decoder)) {
        buf += chunk;
        RegExpMatch? boundary;
        while ((boundary = RegExp(r'\r?\n\r?\n').firstMatch(buf)) != null) {
          final frame = buf.substring(0, boundary!.start);
          buf = buf.substring(boundary.end);
          final ev = _parseFrame(frame);
          if (ev != null) yield ev;
        }
      }
    } finally {
      client.close();
      if (identical(_streamClient, client)) _streamClient = null;
    }
  }

  void cancel() {
    final client = _streamClient;
    _streamClient = null;
    client?.close();
  }

  AgentEvent? _parseFrame(String frame) {
    String event = 'message';
    final dataLines = <String>[];
    for (final line in frame.split(RegExp(r'\r?\n'))) {
      if (line.startsWith('event:')) {
        event = line.substring(6).trim();
      } else if (line.startsWith('data:')) {
        dataLines.add(line.substring(5).trim());
      }
    }
    if (dataLines.isEmpty) return null;
    try {
      return AgentEvent(
        event,
        jsonDecode(dataLines.join('\n')) as Map<String, dynamic>,
      );
    } catch (_) {
      return null;
    }
  }
}
