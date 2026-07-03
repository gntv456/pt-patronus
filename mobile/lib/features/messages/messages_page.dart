import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/message.dart';
import 'package:ptpatronus/theme/glass.dart';
import 'package:ptpatronus/widgets/adaptive.dart';

class MessagesPage extends StatefulWidget {
  const MessagesPage({super.key});
  @override
  State<MessagesPage> createState() => _MessagesPageState();
}

class _MessagesPageState extends State<MessagesPage> {
  final _api = MessageApi();
  Mailbox _mailbox = Mailbox.inbox;
  List<MessageResult> _results = [];
  bool _loading = true;
  String? _error;
  int? _siteFilter;

  List<MessageResult> get _filteredResults => _siteFilter == null
      ? _results
      : _results.where((r) => r.siteId == _siteFilter).toList();

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      _results = await _api.all(_mailbox);
    } catch (e) {
      _error = e.toString();
    } finally {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _switchMailbox(Mailbox m) {
    _mailbox = m;
    _load();
  }

  Future<void> _openDetail(Message msg) async {
    String content = msg.content;
    try {
      if (msg.url.isNotEmpty) content = await _api.content(msg.siteId, msg.url);
      if (!msg.isRead && msg.adminActions?.itemId.isNotEmpty == true) {
        _api.markRead(msg.siteId, [msg.adminActions!.itemId]).then((_) {
          if (mounted) _load(); // 守卫：避免 dispose 后 setState（审计 P1）
        });
      }
    } catch (e) {
      content = '加载失败：${_friendlyError('$e')}';
    }
    if (!mounted) return;
    await showAdaptiveSheet(
      context,
      title: msg.title,
      wideWidth: 720,
      builder: (_) => _detailContent(msg, content),
    );
  }

  Widget _detailContent(Message msg, String content) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text('${msg.sender} · ${msg.date} · ${msg.siteName}', style: const TextStyle(color: Colors.grey, fontSize: 13)),
        const SizedBox(height: 12),
        SelectableText(content.isNotEmpty ? content : '（无正文）', style: const TextStyle(fontSize: 14, height: 1.6)),
        const SizedBox(height: 16),
        Row(children: [
          Expanded(child: OutlinedButton.icon(onPressed: () => _reply(msg), icon: const Icon(Icons.reply), label: const Text('回复'))),
          const SizedBox(width: 10),
          Expanded(child: OutlinedButton.icon(onPressed: () => _delete(msg), icon: Icon(Icons.delete_outline, color: PTColors.danger), label: Text('删除', style: TextStyle(color: PTColors.danger)))),
        ]),
      ],
    );
  }

  Future<void> _reply(Message msg) async {
    Navigator.pop(context);
    final text = await _inputDialog('回复消息');
    if (text == null || text.isEmpty) return;
    try {
      final r = await _api.reply(msg.siteId, msg.url, text);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(r['message'] as String? ?? '已发送')));
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_friendlyError('$e'))));
    }
  }

  Future<void> _delete(Message msg) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (_) => AlertDialog(title: const Text('删除消息'), content: const Text('确定删除该消息？'), actions: [TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')), TextButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除'))]),
    );
    if (ok != true) return;
    if (!mounted) return;
    Navigator.pop(context);
    try {
      await _api.remove(msg.siteId, msg.adminActions?.deleteUrl.isNotEmpty == true ? msg.adminActions!.deleteUrl : msg.url);
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('已删除')));
      _load();
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(_friendlyError('$e'))));
    }
  }

  Future<String?> _inputDialog(String title) async {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (_) => AlertDialog(
        title: Text(title),
        content: TextField(controller: ctrl, maxLines: 6, decoration: const InputDecoration(border: OutlineInputBorder())),
        actions: [TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')), TextButton(onPressed: () => Navigator.pop(context, ctrl.text), child: const Text('发送'))],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassBackground(
      child: Scaffold(
        backgroundColor: Colors.transparent,
        appBar: AppBar(
          leading: const HomeMenuButton(),
          title: const Text('消息'),
          backgroundColor: Colors.transparent,
          elevation: 0,
          actions: [PTIconAction(icon: Icons.refresh, tooltip: '刷新', loading: _loading, onPressed: _loading ? null : _load)],
        ),
        body: Column(
          children: [
            _filterRow(),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _load,
                child: _loading
                    ? const Center(child: CircularProgressIndicator())
                    : _error != null
                        ? PTEmptyState(text: _error!, onRetry: _load)
                        : _results.isEmpty
                            ? PTEmptyState(icon: Icons.mail_outline, text: '暂无消息')
                            : ListView(padding: const EdgeInsets.all(12), children: _filteredResults.map(_group).toList()),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _filterRow() {
    const tabs = [
      (Mailbox.inbox, '收件箱'),
      (Mailbox.system, '系统'),
      (Mailbox.sent, '发件箱'),
      (Mailbox.cheater, '作弊者'),
      (Mailbox.reports, '举报信箱'),
      (Mailbox.staff, '管理组信箱'),
    ];
    // 邮箱 tab 是主导航 → 保留 PTChipBar 横滚；站点筛选 → 小 pill 弹候选。
    final mailBar = PTChipBar<Mailbox>(
      selected: _mailbox,
      onSelected: _switchMailbox,
      padding: EdgeInsets.zero,
      items: tabs.map((t) => PTChipItem<Mailbox>(value: t.$1, label: t.$2)).toList(),
    );
    String siteNameOf(int? id) {
      if (id == null) return '全部站点';
      for (final r in _results) {
        if (r.siteId == id) return r.siteName;
      }
      return '全部站点';
    }
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8),
      child: Row(children: [
        Expanded(child: mailBar),
        if (_results.isNotEmpty) ...[
          const SizedBox(width: 8),
          PTFilterPill(
            label: '站点',
            value: siteNameOf(_siteFilter),
            active: _siteFilter != null,
            onTap: () => showFilterSheet<int?>(
              context,
              title: '筛选站点',
              selected: _siteFilter,
              onSelected: (v) => setState(() => _siteFilter = v),
              options: [
                const PTFilterOption(value: null, label: '全部站点'),
                ..._results.map((r) => PTFilterOption(value: r.siteId, label: r.siteName)),
              ],
            ),
          ),
        ],
      ]),
    );
  }

  Widget _group(MessageResult r) {
    return GlassCard(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(children: [
            Text(r.siteName, style: const TextStyle(fontWeight: FontWeight.w600)),
            const SizedBox(width: 8),
            if (r.error.isNotEmpty)
              Expanded(child: Text(_friendlyError(r.error), maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(color: PTColors.danger, fontSize: 12)))
            else if (r.messages.isEmpty)
              const Text('无消息', style: TextStyle(color: Colors.grey, fontSize: 12)),
          ]),
          ...r.messages.map((m) => ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                leading: m.isRead ? null : CircleAvatar(radius: 4, backgroundColor: PTColors.primary),
                title: Text(m.title, style: TextStyle(fontWeight: m.isRead ? FontWeight.w400 : FontWeight.w600)),
                subtitle: Text('${m.sender} · ${m.date}', style: const TextStyle(fontSize: 12)),
                onTap: () => _openDetail(m),
              )),
        ],
      ),
    );
  }
}

/// 站点原始错误 → 中文友好提示（对齐 Web Messages.vue 的 friendlyError）。原文不可读时降级返回原串。
String _friendlyError(String e) {
  final l = e.toLowerCase();
  if (l.contains('context deadline exceeded') || l.contains('timeout') || l.contains('deadline')) {
    return '站点超时（可能未登录或 Cookie 失效）';
  }
  if (l.contains('no such host') || l.contains('dns')) return '无法解析站点域名';
  if (l.contains('403') || l.contains('forbidden')) return '站点拒绝访问（可能未登录）';
  if (l.contains('cookie')) return 'Cookie 失效，请重新登录站点';
  if (l.contains('eof') || l.contains('connection reset') || l.contains('connection refused')) return '站点连接异常';
  return e;
}
