// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 system_settings_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/system_settings/system_settings_page.dart';

class _LlmGatewayCard extends StatefulWidget {
  final LlmGatewayApi api;
  final LlmGatewayConfig initial;
  const _LlmGatewayCard({required this.api, required this.initial});
  @override
  State<_LlmGatewayCard> createState() => _LlmGatewayCardState();
}

class _LlmGatewayCardState extends State<_LlmGatewayCard> {
  late bool _enabled = widget.initial.enabled;
  // 访问令牌：编辑时不回显明文（GET 本就不返回）；留空 = 保留原令牌。
  late final _tokenCtrl = TextEditingController();
  late final _modelsCtrl = TextEditingController(
    text: widget.initial.allowedModels.join(', '),
  );
  late final _concurrencyCtrl = TextEditingController(
    text: '${widget.initial.maxConcurrency}',
  );
  bool _saving = false;

  @override
  void dispose() {
    _tokenCtrl.dispose();
    _modelsCtrl.dispose();
    _concurrencyCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    // 逗号分隔 → 去空 → 列表；max_concurrency 必须回传（后端整体赋值，漏传会被零值覆盖）。
    final models = _modelsCtrl.text
        .split(',')
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .toList();
    final concurrency = int.tryParse(_concurrencyCtrl.text.trim()) ?? 4;
    final body = <String, dynamic>{
      'enabled': _enabled,
      'allowed_models': models,
      'max_concurrency': concurrency,
      // access_token 仅在用户输入新令牌时携带；留空 = 保留原令牌。
      if (_tokenCtrl.text.trim().isNotEmpty)
        'access_token': _tokenCtrl.text.trim(),
    };
    try {
      final updated = await widget.api.update(body);
      if (!mounted) return;
      _tokenCtrl.clear();
      setState(() => _enabled = updated.enabled);
      _toast('AI 网关配置已保存');
    } catch (e) {
      _toast('保存失败：$e');
      await _reload();
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _reload() async {
    try {
      final c = await widget.api.get();
      if (!mounted) return;
      setState(() {
        _enabled = c.enabled;
        _modelsCtrl.text = c.allowedModels.join(', ');
        _concurrencyCtrl.text = '${c.maxConcurrency}';
      });
      _tokenCtrl.clear();
    } catch (_) {
      // 重载失败保留当前态；用户可再点保存或刷新整页。
    }
  }

  @override
  Widget build(BuildContext context) {
    // 外部客户端对接信息：OpenAI 兼容端点 + 默认模型（只读展示，对齐 Web LlmGateway.vue）。
    final baseUrl = ApiClient().baseUrl;
    final openaiEp = '$baseUrl/api/v1/openai/v1';
    final anthropicEp = '$baseUrl/api/v1/anthropic/v1';
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'bot',
                from: Color(0xFF6C5CE7),
                to: Color(0xFFA855F7),
                size: 32,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('AI 网关',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            '把内置 AI Agent 以 OpenAI / Anthropic 兼容协议对外暴露，'
            '供 Cursor / Claude Desktop / continue.dev 等客户端接入。',
            style: TextStyle(fontSize: 12, color: context.dimText),
          ),
          const SizedBox(height: 12),
          _infoRow('OpenAI 端点', openaiEp),
          _infoRow('Anthropic 端点', anthropicEp),
          _infoRow('默认模型', 'ptpatronus-agent'),
          const SizedBox(height: 12),
          _field(
            '访问令牌',
            TextField(
              controller: _tokenCtrl,
              obscureText: true,
              decoration: const InputDecoration(
                isDense: true,
                hintText: '留空 = 保留原令牌',
                border: OutlineInputBorder(),
              ),
            ),
            hint: widget.initial.accessTokenConfigured ? '令牌已配置（不回显）' : null,
          ),
          _field(
            '允许的模型（可选，逗号分隔；留空 = 接受任意）',
            TextField(
              controller: _modelsCtrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: '如：ptpatronus-agent, gpt-4o',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '并发上限（0 = 不限）',
            TextField(
              controller: _concurrencyCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          const SizedBox(height: 8),
          FilledButton.icon(
            onPressed: _saving ? null : _save,
            icon: _saving
                ? const SizedBox(
                    width: 16, height: 16,
                    child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                : const Icon(Icons.save_outlined, size: 18),
            label: const Text('保存'),
          ),
        ],
      ),
    );
  }

  Widget _infoRow(String label, String value) => Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(
              width: 96,
              child: Text(label,
                  style: TextStyle(fontSize: 12, color: context.dimText)),
            ),
            Expanded(
              child: SelectableText(
                value,
                style: const TextStyle(
                    fontSize: 12, fontFamily: 'monospace'),
              ),
            ),
          ],
        ),
      );

  Widget _field(String label, Widget child, {String? hint}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: context.dimText)),
          if (hint != null)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: Text(hint,
                  style: TextStyle(fontSize: 11, color: context.dimText)),
            ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}
