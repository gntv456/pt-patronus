import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:ptpatronus/api/agent.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';

/// AI 助手页（对齐 Web Assistant.vue）：自然语言控制全平台 + 多轮上下文 + 深度思考。
/// 会话历史持久化；SSE 流式（fetch 流读）；思考折叠 + 工具调用卡 + 停止。
class AssistantPage extends StatefulWidget {
  const AssistantPage({super.key});
  @override
  State<AssistantPage> createState() => _AssistantPageState();
}

class _AssistantPageState extends State<AssistantPage> {
  final _api = AgentApi();
  List<AgentConversation> _convs = [];
  int? _activeId;
  List<AgentMessage> _messages = [];

  final _inputCtrl = TextEditingController();
  final _scrollCtrl = ScrollController();
  bool _sending = false;
  // 实时累积
  String _liveContent = '';
  String _liveReasoning = '';
  bool _hasLive = false;
  List<Map<String, dynamic>> _liveTools = [];

  @override
  void initState() {
    super.initState();
    _loadConversations();
  }

  @override
  void dispose() {
    _inputCtrl.dispose();
    _scrollCtrl.dispose();
    _api.cancel();
    super.dispose();
  }

  Future<void> _loadConversations() async {
    try {
      _convs = await _api.conversations();
      if (mounted && _activeId == null && _convs.isNotEmpty) {
        await _select(_convs.first.id);
      }
      setState(() {});
    } catch (_) {}
  }

  Future<void> _select(int id) async {
    setState(() { _activeId = id; _hasLive = false; });
    try {
      _messages = await _api.messages(id);
    } catch (_) {
      _messages = [];
    } finally {
      if (mounted) setState(() {});
      _scrollToBottom();
    }
  }

  void _newChat() {
    _api.cancel();
    setState(() { _activeId = null; _messages = []; _hasLive = false; });
  }

  Future<void> _delete(int id) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除会话'),
        content: const Text('确定删除该会话？'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
          TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
        ],
      ),
    );
    if (ok != true) return;
    await _api.deleteConversation(id);
    if (_activeId == id) _newChat();
    await _loadConversations();
  }

  Future<void> _send() async {
    final text = _inputCtrl.value.text.trim();
    if (text.isEmpty || _sending) return;
    _inputCtrl.clear();
    _api.cancel();
    setState(() {
      _messages = [..._messages, AgentMessage(id: -1, role: 'user', content: text)];
      _liveContent = '';
      _liveReasoning = '';
      _liveTools = [];
      _hasLive = true;
      _sending = true;
    });
    _scrollToBottom();
    try {
      await for (final ev in _api.chat(text, _activeId)) {
        if (!mounted) break;
        switch (ev.event) {
          case 'conversation':
            _activeId = ev.data['id'] as int?;
          case 'token':
            _liveContent += (ev.data['content'] as String?) ?? '';
          case 'reasoning':
            _liveReasoning += (ev.data['content'] as String?) ?? '';
          case 'tool_call':
            _liveTools.add({'id': ev.data['id'], 'name': ev.data['name'], 'arguments': ev.data['arguments'], 'result': null});
          case 'tool_result':
            final tid = ev.data['id'];
            for (final t in _liveTools) {
              if (t['id'] == tid && t['result'] == null) {
                t['result'] = ev.data['result'];
                break;
              }
            }
          case 'confirm_request':
            await _handleConfirm(ev.data);
          case 'error':
            _toast((ev.data['message'] as String?) ?? '错误');
        }
        setState(() {});
        _scrollToBottom();
      }
      _hasLive = false;
      if (_activeId != null) await _select(_activeId!);
      await _loadConversations();
    } catch (e) {
      _toast('发送失败：$e');
    } finally {
      if (mounted) setState(() => _sending = false);
    }
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg), duration: const Duration(seconds: 2)));
  }

  /// 危险工具二次确认：收到 confirm_request 时弹窗，用户选择后 POST 决断。
  /// await-for 在弹窗期间自然暂停——后端在 Resolve 前不下发后续帧，确认后同一 SSE 连接继续。
  Future<void> _handleConfirm(Map<String, dynamic> d) async {
    final name = (d['name'] as String?) ?? '';
    final summary = (d['summary'] as String?) ?? 'AI 想执行一个不可逆操作';
    final args = (d['arguments'] as String?) ?? '';
    final id = (d['id'] as String?) ?? '';
    final argLine = (args.isNotEmpty && args != '{}') ? '\n参数：$args' : '';
    if (!mounted) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('⚠️ 危险操作确认'),
        content: Text('$summary\n工具：$name$argLine'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('拒绝')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('允许执行')),
        ],
      ),
    );
    try {
      if (_activeId != null) await _api.confirmDecision(_activeId!, id, approved == true);
    } catch (e) {
      _toast('确认回传失败：$e');
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollCtrl.hasClients) _scrollCtrl.jumpTo(_scrollCtrl.position.maxScrollExtent);
    });
  }

  @override
  Widget build(BuildContext context) {
    return PTPageScaffold(
      title: const Text('AI 助手', style: TextStyle(fontSize: 17, fontWeight: FontWeight.w700)),
      actions: [
        PTBarAction(icon: Icons.add_comment, label: '新对话', onPressed: _newChat),
        PTBarAction(icon: Icons.settings, label: '设置', onPressed: _openConfig),
      ],
      body: Column(children: [
        // 会话选择条
        if (_convs.isNotEmpty)
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 6, 12, 0),
            child: Row(children: [
              Expanded(
                child: DropdownButton<int>(
                  value: _activeId,
                  isExpanded: true,
                  hint: const Text('选择会话'),
                  items: _convs.map((c) => DropdownMenuItem(value: c.id, child: Text(c.title.isEmpty ? '(新对话)' : c.title, maxLines: 1, overflow: TextOverflow.ellipsis))).toList(),
                  onChanged: (v) { if (v != null) _select(v); },
                ),
              ),
              IconButton(icon: const Icon(Icons.delete_outline, size: 20), onPressed: _activeId == null ? null : () => _delete(_activeId!), tooltip: '删除会话'),
            ]),
          ),
        Expanded(child: _msgList()),
        _composer(),
      ]),
    );
  }

  Widget _msgList() {
    final units = _buildUnits();
    if (units.isEmpty && !_hasLive) {
      return PTEmptyState(
        icon: Icons.smart_toy,
        title: '和 PTPatronus 助手对话',
        description: '问站点统计、搜影视、订阅、整理下载…自然语言即可',
      );
    }
    return ListView.builder(
      controller: _scrollCtrl,
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
      itemCount: units.length + (_hasLive ? 1 : 0),
      itemBuilder: (_, i) {
        if (i == units.length) return _liveBubble();
        return units[i];
      },
    );
  }

  // 把持久化消息分组为展示单元（user/assistant 气泡 + tool 步骤卡）。
  List<Widget> _buildUnits() {
    final out = <Widget>[];
    List<Map<String, dynamic>>? curTools;
    for (final m in _messages) {
      if (m.role == 'user') {
        curTools = null;
        out.add(_bubble(m.content, true));
      } else if (m.role == 'assistant') {
        final tcs = m.toolCalls.isNotEmpty ? (jsonDecode(m.toolCalls) as List).cast<Map<String, dynamic>>() : <Map<String, dynamic>>[];
        if (m.content.trim().isNotEmpty) {
          curTools = null;
          out.add(_assistantBubble(m.content, m.reasoning));
        } else if (tcs.isNotEmpty) {
          curTools = tcs.map((t) => {'name': t['name'], 'arguments': t['arguments'], 'result': null}).toList();
          out.add(_toolCard(curTools, m.reasoning));
        }
      } else if (m.role == 'tool' && curTools != null) {
        for (final c in curTools) {
          if (c['result'] == null) {
            c['result'] = m.content;
            break;
          }
        }
      }
    }
    return out;
  }

  Widget _bubble(String text, bool isUser) {
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 10),
        constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.8),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(
          gradient: isUser ? const LinearGradient(colors: [Color(0xFF6C5CE7), Color(0xFF4285F4)]) : null,
          color: isUser ? null : Theme.of(context).colorScheme.surfaceContainerHighest,
          borderRadius: BorderRadius.circular(14),
        ),
        child: Text(text, style: TextStyle(color: isUser ? Colors.white : null, fontSize: 14, height: 1.5)),
      ),
    );
  }

  Widget _assistantBubble(String content, String reasoning) {
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 10),
        constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.85),
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        decoration: BoxDecoration(color: Theme.of(context).colorScheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(14)),
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          if (reasoning.isNotEmpty)
            ExpansionTile(
              tilePadding: EdgeInsets.zero,
              dense: true,
              title: const Text('🧠 思考过程', style: TextStyle(fontSize: 12)),
              children: [Align(alignment: Alignment.centerLeft, child: Text(reasoning, style: const TextStyle(fontSize: 12, height: 1.4)))],
            ),
          Text(content, style: const TextStyle(fontSize: 14, height: 1.5)),
        ]),
      ),
    );
  }

  Widget _toolCard(List<Map<String, dynamic>> calls, String reasoning) {
    return Align(
      alignment: Alignment.centerLeft,
      child: Container(
        margin: const EdgeInsets.only(bottom: 10),
        constraints: BoxConstraints(maxWidth: MediaQuery.of(context).size.width * 0.85),
        decoration: BoxDecoration(color: Theme.of(context).colorScheme.surfaceContainerLow, borderRadius: BorderRadius.circular(10)),
        child: ExpansionTile(
          tilePadding: const EdgeInsets.symmetric(horizontal: 10),
          dense: true,
          title: Text('🔧 调用 ${calls.length} 个工具', style: const TextStyle(fontSize: 12)),
          children: calls.map((c) => Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(c['name']?.toString() ?? '', style: const TextStyle(fontWeight: FontWeight.w600, color: Color(0xFF4285F4), fontSize: 12)),
              if (c['arguments'] != null) Text(c['arguments'].toString(), style: const TextStyle(fontSize: 11), maxLines: 3, overflow: TextOverflow.ellipsis),
              if (c['result'] != null)
                Container(
                  margin: const EdgeInsets.only(top: 2),
                  padding: const EdgeInsets.all(4),
                  color: Theme.of(context).colorScheme.surfaceContainerHighest,
                  child: SizedBox(
                    height: 180,
                    child: SingleChildScrollView(child: SelectableText(c['result'].toString(), style: const TextStyle(fontSize: 11))),
                  ),
                ),
            ]),
          )).toList(),
        ),
      ),
    );
  }

  Widget _liveBubble() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        _assistantBubble(_liveContent.isEmpty && _liveTools.isEmpty ? '…' : _liveContent, _liveReasoning),
        if (_liveTools.isNotEmpty) _toolCard(_liveTools, ''),
      ],
    );
  }

  Widget _composer() {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(10, 4, 10, 6),
        child: Row(crossAxisAlignment: CrossAxisAlignment.end, children: [
          Expanded(
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 10),
              decoration: BoxDecoration(color: Theme.of(context).colorScheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(10)),
              child: TextField(
                controller: _inputCtrl,
                minLines: 1,
                maxLines: 4,
                enabled: !_sending,
                onSubmitted: (_) => _send(),
                decoration: const InputDecoration(border: InputBorder.none, isDense: true, hintText: '输入指令…'),
              ),
            ),
          ),
          const SizedBox(width: 8),
          _sending
              ? FloatingActionButton.small(tooltip: '停止生成', onPressed: _api.cancel, backgroundColor: PTColors.danger, child: const Icon(Icons.stop, color: Colors.white))
              : FloatingActionButton.small(tooltip: '发送消息', onPressed: _send, child: const Icon(Icons.send)),
        ]),
      ),
    );
  }

  // —— 配置 ——
  void _openConfig() async {
    final AgentConfig cfg;
    List<ToolCatalogEntry> tools;
    try {
      cfg = await _api.getConfig();
    } catch (_) {
      _toast('加载配置失败');
      return;
    }
    try {
      tools = await _api.tools();
    } catch (_) {
      tools = [];
    }
    AgentStats? stats;
    try {
      stats = await _api.stats();
    } catch (_) {
      stats = null;
    }
    if (!mounted) return;
    // enabled_tools 为空 = 全开；否则按显式清单。
    bool allowAll = cfg.enabledTools.isEmpty;
    Set<String> enabled = Set<String>.from(allowAll ? tools.map((t) => t.name) : cfg.enabledTools);
    final apiKeyCtrl = TextEditingController();
    await showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (ctx) => StatefulBuilder(builder: (ctx, setS) {
        // 按风险分组渲染勾选区（关闭「启用全部」时展示）。
        Widget group(String title, List<ToolCatalogEntry> items) {
          if (items.isEmpty) return const SizedBox.shrink();
          return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Padding(padding: const EdgeInsets.only(top: 8, bottom: 2), child: Text(title, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w600))),
            ...items.map((t) => CheckboxListTile(
              dense: true,
              contentPadding: const EdgeInsets.symmetric(horizontal: 4),
              controlAffinity: ListTileControlAffinity.leading,
              value: enabled.contains(t.name),
              title: Text(t.name, style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600)),
              subtitle: Text(t.risk == 'destructive' ? (t.riskNote.isNotEmpty ? t.riskNote : t.description) : t.description, style: const TextStyle(fontSize: 10)),
              onChanged: (v) => setS(() {
                if (v == true) {
                  enabled.add(t.name);
                } else {
                  enabled.remove(t.name);
                }
              }),
            )),
          ]);
        }
        return SafeArea(
          child: Padding(
            padding: EdgeInsets.fromLTRB(16, 12, 16, 16 + MediaQuery.of(ctx).viewInsets.bottom),
            child: SingleChildScrollView(child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              const Text('AI 助手设置', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w700)),
              const SizedBox(height: 8),
              if (stats != null) _statsCard(stats),
              const SizedBox(height: 12),
              SwitchListTile(value: cfg.enabled, onChanged: (v) => setS(() => cfg.enabled = v), title: const Text('启用')),
              SwitchListTile(value: cfg.enableThinking, onChanged: (v) => setS(() => cfg.enableThinking = v), title: const Text('深度思考')),
              _drop('Provider', cfg.provider, {'openai': 'OpenAI 兼容', 'anthropic': 'Anthropic Claude'}, (v) => setS(() => cfg.provider = v ?? 'openai')),
              const SizedBox(height: 8),
              TextField(controller: apiKeyCtrl, obscureText: true, decoration: InputDecoration(labelText: cfg.apiKeyConfigured ? 'API Key（已配置，不改留空）' : 'API Key', isDense: true)),
              const SizedBox(height: 8),
              TextField(controller: TextEditingController(text: cfg.baseUrl), decoration: const InputDecoration(labelText: 'Base URL', isDense: true), onChanged: (v) => cfg.baseUrl = v),
              const SizedBox(height: 8),
              TextField(controller: TextEditingController(text: cfg.model), decoration: const InputDecoration(labelText: '模型', isDense: true), onChanged: (v) => cfg.model = v),
              const Divider(height: 28),
              // 工具白名单：启用全部 ↔ 按风险勾选。
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: allowAll,
                onChanged: (v) => setS(() {
                  allowAll = v;
                  if (!v && enabled.isEmpty) enabled = tools.map((t) => t.name).toSet();
                }),
                title: const Text('启用全部工具', style: TextStyle(fontSize: 14)),
                subtitle: const Text('关闭后可按风险勾选，新增工具不自动可用', style: TextStyle(fontSize: 11)),
              ),
              if (!allowAll) ...[
                group('⚠️ 危险操作（执行前仍需二次确认）', tools.where((t) => t.risk == 'destructive').toList()),
                group('写入', tools.where((t) => t.risk == 'write').toList()),
                group('只读', tools.where((t) => t.risk == 'read').toList()),
              ],
              const SizedBox(height: 16),
              FilledButton(onPressed: () async {
                try {
                  cfg.enabledTools = allowAll ? <String>[] : enabled.toList();
                  await _api.setConfig(cfg, apiKey: apiKeyCtrl.text.trim().isEmpty ? null : apiKeyCtrl.text.trim());
                  if (ctx.mounted) Navigator.pop(ctx);
                  _toast('配置已保存');
                } catch (_) {
                  _toast('保存失败');
                }
              }, child: const Text('保存')),
            ])),
          ),
        );
      }),
    );
  }

  Widget _statsCard(AgentStats s) {
    final success = s.toolCalls == 0 ? '—' : '${((1 - s.toolErrors / s.toolCalls) * 100).round()}%';
    Widget chip(String label, String value) => Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(children: [
        Text(value, style: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700)),
        const SizedBox(height: 2),
        Text(label, style: const TextStyle(fontSize: 10)),
      ]),
    );
    return Wrap(spacing: 6, runSpacing: 6, children: [
      chip('请求', '${s.totalRequests}'),
      chip('Token', '${s.totalInputTokens + s.totalOutputTokens}'),
      chip('费用', '\$${s.totalCostUsd.toStringAsFixed(4)}'),
      chip('均延迟', '${s.avgLatencyMs}ms'),
      chip('错误', '${s.errorCount}'),
      chip('限流', '${s.rateLimitCount}'),
      chip('工具调用', '${s.toolCalls}'),
      chip('成功率', success),
    ]);
  }

  Widget _drop(String label, String value, Map<String, String> opts, ValueChanged<String?> onChanged) {
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Padding(padding: const EdgeInsets.only(top: 8, bottom: 4), child: Text(label, style: const TextStyle(fontSize: 13))),
      DropdownButton<String>(value: value, isExpanded: true, items: opts.entries.map((e) => DropdownMenuItem(value: e.key, child: Text(e.value))).toList(), onChanged: onChanged),
    ]);
  }
}
