import 'dart:convert';
import 'package:http/http.dart' as http;
import 'package:ptpatronus/core/api_client.dart';

// AgentConversation — 对话会话（对齐后端 models.AgentConversation）。
class AgentConversation {
  final int id;
  final String title;
  final String updatedAt;
  AgentConversation({required this.id, required this.title, required this.updatedAt});
  factory AgentConversation.fromJson(Map<String, dynamic> j) => AgentConversation(
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
  AgentMessage({required this.id, required this.role, required this.content, this.reasoning = '', this.toolCalls = '', this.toolCallId = '', this.name = ''});
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
    required this.enabled, required this.provider, required this.baseUrl, required this.model,
    required this.temperature, required this.maxTokens, required this.enableThinking,
    required this.maxIterations, required this.systemPrompt, required this.enabledTools,
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
        enabledTools: ((j['enabled_tools'] as List?) ?? const []).map((e) => e.toString()).toList(),
        apiKeyConfigured: j['api_key_configured'] as bool? ?? false,
      );
  Map<String, dynamic> toJson() => {
        'enabled': enabled, 'provider': provider, 'base_url': baseUrl, 'model': model,
        'temperature': temperature, 'max_tokens': maxTokens, 'enable_thinking': enableThinking,
        'max_iterations': maxIterations, 'system_prompt': systemPrompt, 'enabled_tools': enabledTools,
      };
}

// AgentEvent — SSE 事件（type: conversation/token/reasoning/tool_call/tool_result/done/error）。
class AgentEvent {
  final String event;
  final Map<String, dynamic> data;
  AgentEvent(this.event, this.data);
}

class AgentApi {
  final _api = ApiClient();
  final _client = http.Client(); // 用于流式 chat（外部 close 可中断）

  Future<List<AgentConversation>> conversations() async {
    final r = await _api.get('/agent/conversations');
    return ((r['conversations'] as List?) ?? const []).map((e) => AgentConversation.fromJson(e as Map<String, dynamic>)).toList();
  }
  Future<List<AgentMessage>> messages(int id) async {
    final r = await _api.get('/agent/conversations/$id/messages');
    return ((r['messages'] as List?) ?? const []).map((e) => AgentMessage.fromJson(e as Map<String, dynamic>)).toList();
  }
  Future<void> deleteConversation(int id) => _api.delete('/agent/conversations/$id');
  Future<AgentConfig> getConfig() async => AgentConfig.fromJson(await _api.get('/agent/config'));
  Future<void> setConfig(AgentConfig c, {String? apiKey}) =>
      _api.put('/agent/config', apiKey != null ? {...c.toJson(), 'api_key': apiKey} : c.toJson());

  /// 流式 chat：yield SSE 事件。外部调 [cancel] 中断（关闭 _client → await-for 抛错）。
  Stream<AgentEvent> chat(String message, int? conversationId) async* {
    final uri = Uri.parse('${_api.baseUrl}/api/v1/agent/chat');
    final req = http.Request('POST', uri)
      ..headers['Content-Type'] = 'application/json'
      ..headers['Authorization'] = 'Bearer ${_api.token ?? ''}'
      ..body = jsonEncode({'conversation_id': conversationId ?? 0, 'message': message});
    final resp = await _client.send(req);
    if (resp.statusCode != 200) {
      final body = await resp.stream.bytesToString();
      throw Exception('HTTP ${resp.statusCode}: ${body.length > 200 ? body.substring(0, 200) : body}');
    }
    String buf = '';
    await for (final chunk in resp.stream.transform(utf8.decoder)) {
      buf += chunk;
      int idx;
      while ((idx = buf.indexOf('\n\n')) >= 0) {
        final frame = buf.substring(0, idx);
        buf = buf.substring(idx + 2);
        final ev = _parseFrame(frame);
        if (ev != null) yield ev;
      }
    }
  }

  void cancel() => _client.close();

  AgentEvent? _parseFrame(String frame) {
    String event = 'message';
    final dataLines = <String>[];
    for (final line in frame.split('\n')) {
      if (line.startsWith('event:')) {
        event = line.substring(6).trim();
      } else if (line.startsWith('data:')) {
        dataLines.add(line.substring(5).trim());
      }
    }
    if (dataLines.isEmpty) return null;
    try {
      return AgentEvent(event, jsonDecode(dataLines.join('\n')) as Map<String, dynamic>);
    } catch (_) {
      return null;
    }
  }
}
