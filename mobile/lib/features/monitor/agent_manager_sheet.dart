// 从 monitor_page.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/features/monitor/monitor_page.dart';

// ── Agent 管理器（admin）：列表 + 创建/重置 secret（明文仅一次）/删除。对齐 Web AgentManager.vue ──
class _AgentManagerSheet extends StatefulWidget {
  const _AgentManagerSheet();
  @override
  State<_AgentManagerSheet> createState() => _AgentManagerSheetState();
}

class _AgentManagerSheetState extends State<_AgentManagerSheet> {
  final _api = AgentApi();
  List<Agent> _agents = [];
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _load();
  }

  void _toast(String s) {
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(s)));
  }

  Future<void> _load() async {
    try {
      _agents = await _api.list();
    } catch (e) {
      _toast('$e');
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  Future<void> _create() async {
    final input = await _promptAgent();
    if (input == null || input.name.isEmpty) return;
    try {
      final res = await _api.create(input.name, type: input.type);
      _toast('已创建');
      await _showSecret(res.plainSecret);
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _rotate(Agent a) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('重置 Secret'),
        content: const Text('重置后旧 secret 立即失效，agent 需用新 secret 重连。确认？'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('重置'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      final secret = await _api.rotate(a.id);
      await _showSecret(secret);
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<void> _delete(Agent a) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('删除 Agent'),
        content: Text('删除 Agent「${a.name}」？将级联删除关联的监控目标（含快照/告警）。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(backgroundColor: Colors.redAccent),
            child: const Text('删除'),
          ),
        ],
      ),
    );
    if (ok != true) return;
    try {
      await _api.remove(a.id);
      _toast('已删除');
      _load();
    } catch (e) {
      _toast('$e');
    }
  }

  Future<({String name, String type})?> _promptAgent() async {
    final ctrl = TextEditingController();
    var type = 'nas';
    return showDialog<({String name, String type})>(
      context: context,
      builder: (_) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: const Text('新建 Agent'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: ctrl,
                autofocus: true,
                decoration: const InputDecoration(
                  hintText: 'Agent 名称（如 飞牛NAS）',
                ),
              ),
              const SizedBox(height: 12),
              DropdownButtonFormField<String>(
                initialValue: type,
                decoration: const InputDecoration(labelText: '设备类型'),
                items: const [
                  DropdownMenuItem(value: 'nas', child: Text('NAS')),
                  DropdownMenuItem(value: 'host', child: Text('服务器')),
                ],
                onChanged: (v) => setDialogState(() => type = v ?? type),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('取消'),
            ),
            FilledButton(
              onPressed: () =>
                  Navigator.pop(context, (name: ctrl.text.trim(), type: type)),
              child: const Text('创建'),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showSecret(String secret) async {
    if (secret.isEmpty) return;
    await showDialog(
      context: context,
      builder: (_) => AlertDialog(
        title: const Text('Agent Secret（仅显示一次）'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              '请立即复制保存，用于 agent 上报鉴权。关闭后无法再查看（可重置）。',
              style: TextStyle(color: context.dimText, fontSize: 12),
            ),
            const SizedBox(height: 10),
            Container(
              width: double.maxFinite,
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: context.dimText.withValues(alpha: 0.12),
                borderRadius: BorderRadius.circular(8),
              ),
              child: SelectableText(
                secret,
                style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: secret));
              if (!mounted) return;
              ScaffoldMessenger.of(
                context,
              ).showSnackBar(const SnackBar(content: Text('Secret 已复制')));
            },
            child: const Text('复制'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('关闭'),
          ),
        ],
      ),
    );
  }

  String _fmt(String iso) {
    final d = DateTime.tryParse(iso);
    if (d == null) return iso;
    String p(int n) => n.toString().padLeft(2, '0');
    return '${d.month}-${d.day} ${p(d.hour)}:${p(d.minute)}';
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Text(
              '${_agents.length} 个 Agent',
              style: TextStyle(fontSize: 12, color: context.dimText),
            ),
            const Spacer(),
            OutlinedButton.icon(
              onPressed: _create,
              icon: const Icon(Icons.add, size: 16),
              label: const Text('新建'),
            ),
          ],
        ),
        const SizedBox(height: 10),
        if (_loading)
          const PTLoadingView()
        else if (_agents.isEmpty)
          Padding(
            padding: EdgeInsets.symmetric(vertical: 24),
            child: Center(
              child: Text(
                '暂无 Agent，点「新建」创建',
                style: TextStyle(color: context.dimText),
              ),
            ),
          )
        else
          ..._agents.map(_card),
      ],
    );
  }

  Widget _card(Agent a) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 10),
      child: GlassCard(
        padding: const EdgeInsets.all(12),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  Icons.circle,
                  size: 10,
                  color: a.online ? PTColors.success : context.dimText,
                ),
                const SizedBox(width: 6),
                Expanded(
                  child: Text(
                    a.name,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 15,
                    ),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
                if (a.version.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(right: 6),
                    child: Text(
                      'v${a.version}',
                      style: TextStyle(color: context.dimText, fontSize: 11),
                    ),
                  ),
                PopupMenuButton<String>(
                  icon: const Icon(Icons.more_vert, size: 18),
                  onSelected: (v) {
                    if (v == 'rotate') {
                      _rotate(a);
                    } else if (v == 'delete') {
                      _delete(a);
                    }
                  },
                  itemBuilder: (_) => const [
                    PopupMenuItem(value: 'rotate', child: Text('重置 Secret')),
                    PopupMenuItem(value: 'delete', child: Text('删除')),
                  ],
                ),
              ],
            ),
            const SizedBox(height: 4),
            Text(
              '${a.hostname.isEmpty ? '—' : a.hostname}${a.ip.isNotEmpty ? ' · ${a.ip}' : ''}',
              style: TextStyle(color: context.dimText, fontSize: 12),
            ),
            const SizedBox(height: 2),
            Text(
              a.online
                  ? '在线 · 上报 ${_fmt(a.lastSeenAt)}'
                  : '离线 · 上次 ${a.lastSeenAt.isEmpty ? '—' : _fmt(a.lastSeenAt)}',
              style: TextStyle(
                color: a.online ? PTColors.success : context.dimText,
                fontSize: 11,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
