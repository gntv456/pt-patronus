// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// AssistantPage 的展示层扩展（无 setState 段；从 assistant_page.dart 拆出，行为不变）。
part of 'package:ptpatronus/features/assistant/assistant_page.dart';

extension on _AssistantPageState {

  Widget _msgList() {
    final units = _buildUnits();
    if (units.isEmpty && !_hasLive) {
      // 读取失败不能画成「还没说过话」的空态：那会让用户以为会话丢了。
      if (_messagesNotice.isNotEmpty) {
        return PTEmptyState(
          icon: Icons.error_outline,
          text: _messagesNotice,
          description: '会话内容还在服务器上，只是这次没读出来。',
          onRetry: _activeId == null ? null : () => _select(_activeId!),
        );
      }
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
              ? FloatingActionButton.small(tooltip: '停止生成', onPressed: _stop, backgroundColor: PTColors.danger, child: const Icon(Icons.stop, color: Colors.white))
              : FloatingActionButton.small(tooltip: '发送消息', onPressed: _send, child: const Icon(Icons.send)),
        ]),
      ),
    );
  }

  // —— 配置 ——
  void _openConfig() async {
    final AgentConfig cfg;
    List<ToolCatalogEntry> tools;
    var toolsError = '';
    try {
      cfg = await _api.getConfig();
    } catch (e) {
      // 读不到配置就绝不打开面板：面板里的字段是整份 PUT 回去的，
      // 用默认值顶着打开一次再点保存，就把真配置覆盖成默认了。
      _toast('配置读不出来：$e');
      return;
    }
    try {
      tools = await _api.tools();
    } catch (e) {
      tools = [];
      toolsError = '$e';
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
              if (toolsError.isNotEmpty)
                Container(
                  margin: const EdgeInsets.only(bottom: 8),
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(color: PTColors.danger.withValues(alpha: 0.12), borderRadius: BorderRadius.circular(8)),
                  child: Text(
                    '工具目录读不出来：$toolsError\n此时不能改「启用全部工具」——关掉它会把清单存成空，而空清单在服务器上等于「全部允许」。',
                    style: const TextStyle(fontSize: 11, height: 1.4, color: PTColors.danger),
                  ),
                ),
              SwitchListTile(
                contentPadding: EdgeInsets.zero,
                value: allowAll,
                // 目录读失败时禁用开关（onChanged=null 即禁用）：enabled 的初值取自这份目录，空目录会让「自选」被存成全开。
                onChanged: toolsError.isEmpty
                    ? (v) => setS(() {
                          allowAll = v;
                          if (!v && enabled.isEmpty) enabled = tools.map((t) => t.name).toSet();
                        })
                    : null,
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
                } catch (e) {
                  _toast('保存失败：$e');
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
