import 'package:flutter/material.dart';
import 'package:ptpatronus/api/cookiecloud.dart';
import 'package:ptpatronus/api/dirwatch.dart';
import 'package:ptpatronus/api/llm_gateway.dart';
import 'package:ptpatronus/api/tgbot.dart';
import 'package:ptpatronus/core/api_client.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';
import 'package:ptpatronus/widgets/glass_icon.dart';
import 'package:ptpatronus/widgets/pt_page_scaffold.dart';
import '../system/logs_page.dart';

/// 系统设置页：聚合三个管理员级配置卡片（后端均已存在）。
///  1. AI 网关（/llmgateway/config）—— 把内置 Agent 以 OpenAI/Anthropic 兼容协议暴露。
///  2. Telegram Bot（/tgbot/config）—— 入站 bot 配置。
///  3. 目录监视（/transfer/dirwatch）—— 本地目录 → 整理流水线。
///
/// 整页受 [ApiClient.isAdmin] 守卫；三卡初始化并发加载，各自独立保存（失败回滚+重载）。
/// 设计范式同 [MediaServerPage]：PTPageScaffold + GlassCard + 脱敏令牌（留空=保留）。
class SystemSettingsPage extends StatefulWidget {
  const SystemSettingsPage({super.key});
  @override
  State<SystemSettingsPage> createState() => _SystemSettingsPageState();
}

class _SystemSettingsPageState extends State<SystemSettingsPage> {
  final _llmApi = LlmGatewayApi();
  final _tgApi = TgBotApi();
  final _dirApi = DirwatchApi();
  final _ccApi = CookieCloudApi();

  LlmGatewayConfig? _llm;
  TgBotConfig? _tg;
  DirwatchConfig? _dir;
  CookieCloudConfig? _cc;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _loadAll();
  }

  /// 并发拉取三份配置；任一失败 → 全局错误态（带重试，对齐 MediaServerPage 单错误态范式）。
  Future<void> _loadAll() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final results = await Future.wait<dynamic>([
        _llmApi.get(),
        _tgApi.get(),
        _dirApi.get(),
        _ccApi.get(),
      ]);
      if (!mounted) return;
      _llm = results[0] as LlmGatewayConfig;
      _tg = results[1] as TgBotConfig;
      _dir = results[2] as DirwatchConfig;
      _cc = results[3] as CookieCloudConfig;
    } catch (e) {
      if (!mounted) return;
      _error = '$e';
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // 非管理员：整页拦截（后端仍是权威鉴权，此处仅 UI 显隐）。
    if (!ApiClient().isAdmin) {
      return PTPageScaffold(
        title: '系统设置',
        body: const PTEmptyState(
          icon: Icons.admin_panel_settings_outlined,
          text: '仅管理员可访问',
        ),
      );
    }
    return PTPageScaffold(
      title: '系统设置',
      actions: [
        PTBarAction(
          icon: Icons.refresh,
          label: '刷新',
          loading: _loading,
          onPressed: _loading ? null : _loadAll,
        ),
      ],
      body: Column(
        children: [
          if (_loading && _llm == null && _tg == null && _dir == null && _cc == null)
            const LinearProgressIndicator(minHeight: 2),
          Expanded(child: _content()),
        ],
      ),
    );
  }

  Widget _content() {
    if (_loading && _llm == null && _tg == null && _dir == null && _cc == null) {
      return const Center(child: CircularProgressIndicator());
    }
    if (_error != null && _llm == null && _tg == null && _dir == null && _cc == null) {
      return PTEmptyState(
        icon: Icons.error_outline,
        text: '加载失败',
        description: _error,
        onRetry: _loadAll,
      );
    }
    return RefreshIndicator(
      onRefresh: _loadAll,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 24),
        children: [
          if (_llm != null)
            _LlmGatewayCard(api: _llmApi, initial: _llm!)
          else
            _failedCard('AI 网关'),
          const SizedBox(height: 12),
          if (_tg != null)
            _TgBotCard(api: _tgApi, initial: _tg!)
          else
            _failedCard('Telegram Bot'),
          const SizedBox(height: 12),
          if (_dir != null)
            _DirwatchCard(api: _dirApi, initial: _dir!)
          else
            _failedCard('目录监视'),
          const SizedBox(height: 12),
          if (_cc != null)
            _CookieCloudCard(api: _ccApi, initial: _cc!)
          else
            _failedCard('CookieCloud'),
        ],
      ),
    );
  }

  Widget _failedCard(String name) => GlassCard(
        padding: const EdgeInsets.all(14),
        child: Row(
          children: [
            const Icon(Icons.error_outline, color: PTColors.danger, size: 18),
            const SizedBox(width: 8),
            Expanded(
              child: Text('$name 配置加载失败，点右上「刷新」重试',
                  style: const TextStyle(fontSize: 13, color: PTColors.danger)),
            ),
          ],
        ),
      );
}

// ─────────────────────────────────────────────────────────────────────────
// Card 1 — AI 网关
// ─────────────────────────────────────────────────────────────────────────

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
          const Text(
            '把内置 AI Agent 以 OpenAI / Anthropic 兼容协议对外暴露，'
            '供 Cursor / Claude Desktop / continue.dev 等客户端接入。',
            style: TextStyle(fontSize: 12, color: Colors.grey),
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
                  style: const TextStyle(fontSize: 12, color: Colors.grey)),
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
              style: const TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          if (hint != null)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child: Text(hint,
                  style: const TextStyle(fontSize: 11, color: Colors.grey)),
            ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Card 2 — Telegram Bot
// ─────────────────────────────────────────────────────────────────────────

class _TgBotCard extends StatefulWidget {
  final TgBotApi api;
  final TgBotConfig initial;
  const _TgBotCard({required this.api, required this.initial});
  @override
  State<_TgBotCard> createState() => _TgBotCardState();
}

class _TgBotCardState extends State<_TgBotCard> {
  late bool _enabled = widget.initial.enabled;
  late bool _allowGroupChat = widget.initial.allowGroupChat;
  late final _tokenCtrl = TextEditingController();
  late final _idsCtrl = TextEditingController(
    text: widget.initial.allowedUserIds.join(', '),
  );
  late final _apiUrlCtrl = TextEditingController(text: widget.initial.apiUrl);
  bool _saving = false;

  @override
  void dispose() {
    _tokenCtrl.dispose();
    _idsCtrl.dispose();
    _apiUrlCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    // 逗号分隔 → int 白名单；非法片段静默丢弃。
    final ids = _idsCtrl.text
        .split(',')
        .map((s) => s.trim())
        .where((s) => s.isNotEmpty)
        .map((s) => int.tryParse(s))
        .whereType<int>()
        .toList();
    final body = <String, dynamic>{
      'enabled': _enabled,
      'allowed_user_ids': ids,
      'allow_group_chat': _allowGroupChat,
      'api_url': _apiUrlCtrl.text.trim(),
      // bot_token 仅在用户输入新 token 时携带；留空 = 保留原 token。
      if (_tokenCtrl.text.trim().isNotEmpty)
        'bot_token': _tokenCtrl.text.trim(),
    };
    try {
      final updated = await widget.api.update(body);
      if (!mounted) return;
      _tokenCtrl.clear();
      setState(() {
        _enabled = updated.enabled;
        _allowGroupChat = updated.allowGroupChat;
        _idsCtrl.text = updated.allowedUserIds.join(', ');
        _apiUrlCtrl.text = updated.apiUrl;
      });
      _toast('Telegram Bot 配置已保存（部分改动需重启后端生效）');
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
        _allowGroupChat = c.allowGroupChat;
        _idsCtrl.text = c.allowedUserIds.join(', ');
        _apiUrlCtrl.text = c.apiUrl;
      });
      _tokenCtrl.clear();
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'message',
                from: Color(0xFF2193B0),
                to: Color(0xFF6DD5ED),
                size: 32,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('Telegram Bot',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          const Text(
            '入站 bot：长轮询收消息 → 斜杠命令或自然语言 → 复用 AI Agent → 回帖。',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
          const SizedBox(height: 12),
          _field(
            'Bot Token',
            TextField(
              controller: _tokenCtrl,
              obscureText: true,
              decoration: InputDecoration(
                isDense: true,
                hintText: widget.initial.botTokenConfigured
                    ? '令牌已配置（留空 = 保留）'
                    : '留空 = 保留原 Token',
                border: const OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '允许用户 ID（逗号分隔）',
            TextField(
              controller: _idsCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                hintText: '如：123456789, 987654321',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _switchRow('允许群聊响应', _allowGroupChat,
              (v) => setState(() => _allowGroupChat = v)),
          _field(
            'API 地址（可选，国内反代；留空 = 官方）',
            TextField(
              controller: _apiUrlCtrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: 'https://api.telegram.org',
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

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: const TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }

  Widget _switchRow(String label, bool value, ValueChanged<bool> onChanged) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Row(
        children: [
          Expanded(child: Text(label, style: const TextStyle(fontSize: 13))),
          Switch(value: value, onChanged: onChanged),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Card 3 — 目录监视
// ─────────────────────────────────────────────────────────────────────────

class _DirwatchCard extends StatefulWidget {
  final DirwatchApi api;
  final DirwatchConfig initial;
  const _DirwatchCard({required this.api, required this.initial});
  @override
  State<_DirwatchCard> createState() => _DirwatchCardState();
}

class _DirwatchRow {
  final TextEditingController path;
  final TextEditingController libraryPath;
  _DirwatchRow({String path = '', String libraryPath = ''})
      : path = TextEditingController(text: path),
        libraryPath = TextEditingController(text: libraryPath);

  void dispose() {
    path.dispose();
    libraryPath.dispose();
  }
}

class _DirwatchCardState extends State<_DirwatchCard> {
  late bool _enabled = widget.initial.enabled;
  late String _monitorMode = widget.initial.monitorMode.isEmpty
      ? 'auto'
      : widget.initial.monitorMode;
  late final _debounceCtrl = TextEditingController(
    text: '${widget.initial.debounceMs}',
  );
  late final _pollCtrl = TextEditingController(
    text: '${widget.initial.pollIntervalSec}',
  );
  late final List<_DirwatchRow> _rows = widget.initial.dirs
      .map((d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath))
      .toList();
  bool _saving = false;

  @override
  void dispose() {
    _debounceCtrl.dispose();
    _pollCtrl.dispose();
    for (final r in _rows) {
      r.dispose();
    }
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  void _addRow() => setState(() => _rows.add(_DirwatchRow()));

  void _removeRow(int i) => setState(() {
        _rows[i].dispose();
        _rows.removeAt(i);
      });

  Future<void> _save() async {
    setState(() => _saving = true);
    // 后端整体替换式 PUT（dirwatch.Config 直接绑定），故必须带上全部字段。
    final cfg = DirwatchConfig(
      enabled: _enabled,
      dirs: _rows
          .where((r) => r.path.text.trim().isNotEmpty)
          .map((r) => DirwatchDir(
                path: r.path.text.trim(),
                libraryPath: r.libraryPath.text.trim(),
              ))
          .toList(),
      monitorMode: _monitorMode,
      debounceMs: int.tryParse(_debounceCtrl.text.trim()) ?? 2000,
      pollIntervalSec: int.tryParse(_pollCtrl.text.trim()) ?? 0,
    );
    try {
      final updated = await widget.api.update(cfg);
      if (!mounted) return;
      // 同步本地行控制器到服务端结果。
      for (final r in _rows) {
        r.dispose();
      }
      setState(() {
        _enabled = updated.enabled;
        _monitorMode = updated.monitorMode.isEmpty ? 'auto' : updated.monitorMode;
        _debounceCtrl.text = '${updated.debounceMs}';
        _pollCtrl.text = '${updated.pollIntervalSec}';
        _rows
          ..clear()
          ..addAll(updated.dirs
              .map((d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath)));
      });
      _toast('目录监视配置已保存（需重启后端生效）');
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
      for (final r in _rows) {
        r.dispose();
      }
      setState(() {
        _enabled = c.enabled;
        _monitorMode = c.monitorMode.isEmpty ? 'auto' : c.monitorMode;
        _debounceCtrl.text = '${c.debounceMs}';
        _pollCtrl.text = '${c.pollIntervalSec}';
        _rows
          ..clear()
          ..addAll(c.dirs.map(
              (d) => _DirwatchRow(path: d.path, libraryPath: d.libraryPath)));
      });
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const GlassIcon(
                glyph: 'files',
                from: Color(0xFF6C5CE7),
                to: Color(0xFF00B894),
                size: 32,
              ),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('目录监视',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          const Text(
            '实时监视本地目录，发现媒体文件即喂给整理流水线'
            '（补 DoneHook + cron 覆盖不到的手动投放 / 非托管来源）。',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
          const SizedBox(height: 12),
          Row(
            children: [
              const SizedBox(
                width: 96,
                child: Text('监视目录',
                    style: TextStyle(
                        fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
              ),
              const Spacer(),
              TextButton.icon(
                icon: const Icon(Icons.add, size: 16),
                label: const Text('添加目录'),
                onPressed: _addRow,
              ),
            ],
          ),
          const SizedBox(height: 4),
          for (int i = 0; i < _rows.length; i++)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(
                    child: TextField(
                      controller: _rows[i].path,
                      decoration: const InputDecoration(
                        isDense: true,
                        hintText: '源目录 /downloads/movies',
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: TextField(
                      controller: _rows[i].libraryPath,
                      decoration: const InputDecoration(
                        isDense: true,
                        hintText: '媒体库 /library/movies',
                        border: OutlineInputBorder(),
                      ),
                    ),
                  ),
                  IconButton(
                    tooltip: '删除',
                    icon: Icon(Icons.delete_outline,
                        size: 20, color: PTColors.danger),
                    onPressed: () => _removeRow(i),
                  ),
                ],
              ),
            ),
          if (_rows.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 8),
              child: Text('暂无监视目录，点「添加目录」新建',
                  style: TextStyle(fontSize: 12, color: Colors.grey)),
            ),
          const SizedBox(height: 4),
          _field(
            '监视模式',
            DropdownButton<String>(
              value: _monitorMode == 'compatibility' ? 'compatibility' : 'auto',
              isExpanded: true,
              items: const [
                DropdownMenuItem(
                    value: 'auto', child: Text('auto（fsnotify 事件驱动）')),
                DropdownMenuItem(
                    value: 'compatibility',
                    child: Text('compatibility（轮询兜底）')),
              ],
              onChanged: (v) => setState(() => _monitorMode = v ?? 'auto'),
            ),
          ),
          _field(
            '事件去抖（毫秒）',
            TextField(
              controller: _debounceCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          if (_monitorMode == 'compatibility')
            _field(
              '轮询间隔（秒，仅 compatibility 模式）',
              TextField(
                controller: _pollCtrl,
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

  Widget _field(String label, Widget child) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: const TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────────────────
// Card 4 — CookieCloud 同步
// ─────────────────────────────────────────────────────────────────────────

class _CookieCloudCard extends StatefulWidget {
  final CookieCloudApi api;
  final CookieCloudConfig initial;
  const _CookieCloudCard({required this.api, required this.initial});
  @override
  State<_CookieCloudCard> createState() => _CookieCloudCardState();
}

class _CookieCloudCardState extends State<_CookieCloudCard> {
  late bool _enabled = widget.initial.enabled;
  late final _serverCtrl = TextEditingController(text: widget.initial.server);
  late final _uuidCtrl = TextEditingController(text: widget.initial.uuid);
  late final _passCtrl = TextEditingController();
  late final _intervalCtrl =
      TextEditingController(text: '${widget.initial.intervalMin}');
  bool _saving = false;
  bool _syncing = false;

  @override
  void dispose() {
    _serverCtrl.dispose();
    _uuidCtrl.dispose();
    _passCtrl.dispose();
    _intervalCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _save() async {
    setState(() => _saving = true);
    final interval = int.tryParse(_intervalCtrl.text.trim()) ?? 120;
    final body = <String, dynamic>{
      'enabled': _enabled,
      'server': _serverCtrl.text.trim(),
      'uuid': _uuidCtrl.text.trim(),
      'interval_min': interval,
      if (_passCtrl.text.trim().isNotEmpty) 'password': _passCtrl.text.trim(),
    };
    try {
      await widget.api.update(body);
      if (!mounted) return;
      _passCtrl.clear();
      _toast('CookieCloud 配置已保存');
    } catch (e) {
      _toast('保存失败：$e');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  Future<void> _sync() async {
    setState(() => _syncing = true);
    try {
      final r = await widget.api.sync();
      _toast('同步完成：更新 ${r.updated}，新增 ${r.added}，跳过 ${r.skipped}');
    } catch (e) {
      _toast('同步失败：$e');
    } finally {
      if (mounted) setState(() => _syncing = false);
    }
  }

  String _fmtTime(int unix) {
    final d = DateTime.fromMillisecondsSinceEpoch(unix * 1000);
    String two(int v) => v.toString().padLeft(2, '0');
    return '${d.year}-${two(d.month)}-${two(d.day)} ${two(d.hour)}:${two(d.minute)}';
  }

  @override
  Widget build(BuildContext context) {
    return GlassCard(
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.cloud_sync, size: 28, color: Color(0xFF0EA5E9)),
              const SizedBox(width: 10),
              const Expanded(
                child: Text('CookieCloud 同步',
                    style:
                        TextStyle(fontWeight: FontWeight.w700, fontSize: 15)),
              ),
              Switch(value: _enabled, onChanged: (v) => setState(() => _enabled = v)),
            ],
          ),
          const SizedBox(height: 4),
          const Text(
            '浏览器装 CookieCloud 扩展并填服务器/UUID/密码后，定时拉取最新 cookie，'
            '按域名覆盖本地已失效站点 + 自动新增匹配内置定义的站点。',
            style: TextStyle(fontSize: 12, color: Colors.grey),
          ),
          if (widget.initial.lastError.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text('上次错误：${widget.initial.lastError}',
                  style: const TextStyle(fontSize: 11, color: PTColors.danger)),
            )
          else if (widget.initial.lastSyncUnix > 0)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text('上次同步：${_fmtTime(widget.initial.lastSyncUnix)}',
                  style: const TextStyle(fontSize: 11, color: Colors.grey)),
            ),
          const SizedBox(height: 12),
          _field(
            '服务器',
            TextField(
              controller: _serverCtrl,
              decoration: const InputDecoration(
                isDense: true,
                hintText: 'https://cc.example.com',
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            'UUID / Key',
            TextField(
              controller: _uuidCtrl,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '端到端密码',
            TextField(
              controller: _passCtrl,
              obscureText: true,
              decoration: InputDecoration(
                isDense: true,
                hintText: widget.initial.passwordConfigured ? '已配置（留空=不修改）' : '密码',
                border: const OutlineInputBorder(),
              ),
            ),
          ),
          _field(
            '同步间隔（分钟）',
            TextField(
              controller: _intervalCtrl,
              keyboardType: TextInputType.number,
              decoration: const InputDecoration(
                isDense: true,
                border: OutlineInputBorder(),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: [
              FilledButton.tonalIcon(
                onPressed: _syncing ? null : _sync,
                icon: _syncing
                    ? const SizedBox(
                        width: 16, height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.sync, size: 18),
                label: const Text('立即同步'),
              ),
              const SizedBox(width: 8),
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
        ],
      ),
    );
  }

  Widget _field(String label, Widget child, {String? hint}) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label,
              style: const TextStyle(
                  fontSize: 12, fontWeight: FontWeight.w600, color: Colors.grey)),
          if (hint != null)
            Padding(
              padding: const EdgeInsets.only(top: 2),
              child:
                  Text(hint, style: const TextStyle(fontSize: 11, color: Colors.grey)),
            ),
          const SizedBox(height: 6),
          child,
        ],
      ),
    );
  }
}