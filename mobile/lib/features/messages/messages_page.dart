import 'package:flutter/material.dart';
import 'package:ptpatronus/features/home/home_menu_button.dart';
import 'package:ptpatronus/api/message.dart';
import 'package:ptpatronus/theme/glass.dart';

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
        _api.markRead(msg.siteId, [msg.adminActions!.itemId]).then((_) => _load());
      }
    } catch (e) {
      content = '加载失败：$e';
    }
    if (!mounted) return;
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (_) => _DetailSheet(msg: msg, content: content, onReply: () => _reply(msg), onDelete: () => _delete(msg)),
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
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
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
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
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
          actions: [IconButton(icon: const Icon(Icons.refresh), onPressed: _loading ? null : _load)],
        ),
        body: Column(
          children: [
            _mailboxTabs(),
            Expanded(
              child: RefreshIndicator(
                onRefresh: _load,
                child: _loading
                    ? const Center(child: CircularProgressIndicator())
                    : _error != null
                        ? ListView(children: [const SizedBox(height: 120), Center(child: Text(_error!, style: const TextStyle(color: Colors.grey)))])
                        : _results.isEmpty
                            ? ListView(children: const [SizedBox(height: 120), Center(child: Text('暂无消息', style: TextStyle(color: Colors.grey)))])
                            : ListView(padding: const EdgeInsets.all(12), children: _results.map(_group).toList()),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _mailboxTabs() {
    const tabs = [
      (Mailbox.inbox, '收件箱'),
      (Mailbox.system, '系统'),
      (Mailbox.sent, '发件箱'),
      (Mailbox.cheater, '作弊者'),
      (Mailbox.reports, '举报信箱'),
      (Mailbox.staff, '管理组信箱'),
    ];
    return SizedBox(
      height: 44,
      child: ListView(
        scrollDirection: Axis.horizontal,
        children: tabs
            .map((t) => GestureDetector(
                  onTap: () => _switchMailbox(t.$1),
                  child: Container(
                    margin: const EdgeInsets.symmetric(horizontal: 4, vertical: 6),
                    padding: const EdgeInsets.symmetric(horizontal: 16),
                    alignment: Alignment.center,
                    decoration: BoxDecoration(
                      color: _mailbox == t.$1 ? const Color(0xFF6C5CE7).withValues(alpha: 0.16) : Colors.transparent,
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Text(t.$2, style: TextStyle(color: _mailbox == t.$1 ? const Color(0xFF6C5CE7) : null, fontWeight: _mailbox == t.$1 ? FontWeight.w600 : null)),
                  ),
                ))
            .toList(),
      ),
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
            if (r.error.isNotEmpty) Text(r.error, style: const TextStyle(color: Color(0xFFF56C6C), fontSize: 12))
            else if (r.messages.isEmpty) const Text('无消息', style: TextStyle(color: Colors.grey, fontSize: 12)),
          ]),
          ...r.messages.map((m) => ListTile(
                dense: true,
                contentPadding: EdgeInsets.zero,
                leading: m.isRead ? null : const CircleAvatar(radius: 4, backgroundColor: Color(0xFF6C5CE7)),
                title: Text(m.title, style: TextStyle(fontWeight: m.isRead ? FontWeight.w400 : FontWeight.w600)),
                subtitle: Text('${m.sender} · ${m.date}', style: const TextStyle(fontSize: 12)),
                onTap: () => _openDetail(m),
              )),
        ],
      ),
    );
  }
}

class _DetailSheet extends StatelessWidget {
  final Message msg;
  final String content;
  final VoidCallback onReply;
  final VoidCallback onDelete;
  const _DetailSheet({required this.msg, required this.content, required this.onReply, required this.onDelete});

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.75,
      child: Column(
        children: [
          AppBar(
            title: Text(msg.title, overflow: TextOverflow.ellipsis),
            automaticallyImplyLeading: false,
            actions: [IconButton(onPressed: () => Navigator.pop(context), icon: const Icon(Icons.close))],
          ),
          Expanded(
            child: SingleChildScrollView(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('${msg.sender} · ${msg.date} · ${msg.siteName}', style: const TextStyle(color: Colors.grey, fontSize: 13)),
                  const SizedBox(height: 12),
                  Text(content.isNotEmpty ? content : '（无正文）', style: const TextStyle(fontSize: 14, height: 1.6)),
                ],
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(12),
            child: Row(children: [
              Expanded(child: OutlinedButton.icon(onPressed: onReply, icon: const Icon(Icons.reply), label: const Text('回复'))),
              const SizedBox(width: 10),
              Expanded(child: OutlinedButton.icon(onPressed: onDelete, icon: const Icon(Icons.delete_outline, color: Color(0xFFF56C6C)), label: const Text('删除', style: TextStyle(color: Color(0xFFF56C6C))))),
            ]),
          ),
        ],
      ),
    );
  }
}
