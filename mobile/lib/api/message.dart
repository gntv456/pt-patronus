import '../core/api_client.dart';

enum Mailbox { inbox, sent, system, cheater, reports, staff }

String mailboxParam(Mailbox m) => switch (m) {
      Mailbox.inbox => 'inbox',
      Mailbox.sent => 'sent',
      Mailbox.system => 'system',
      Mailbox.cheater => 'cheater',
      Mailbox.reports => 'reports',
      Mailbox.staff => 'staff',
    };

class AdminActions {
  final String replyUrl, deleteUrl, itemId;
  AdminActions({required this.replyUrl, required this.deleteUrl, required this.itemId});
  factory AdminActions.fromJson(Map<String, dynamic> j) => AdminActions(
        replyUrl: j['reply_url'] as String? ?? '',
        deleteUrl: j['delete_url'] as String? ?? '',
        itemId: j['item_id'] as String? ?? '',
      );
}

class Message {
  final int id;
  final int siteId;
  final String siteName;
  final String sender;
  final String title;
  final String date;
  final String url;
  final bool isRead;
  final String content;
  final Mailbox mailbox;
  final AdminActions? adminActions;
  Message({
    required this.id,
    required this.siteId,
    required this.siteName,
    required this.sender,
    required this.title,
    required this.date,
    required this.url,
    required this.isRead,
    required this.content,
    required this.mailbox,
    required this.adminActions,
  });
  factory Message.fromJson(Map<String, dynamic> j) {
    final aa = j['admin_actions'] as Map<String, dynamic>?;
    return Message(
      id: (j['id'] as num?)?.toInt() ?? 0,
      siteId: (j['site_id'] as num?)?.toInt() ?? 0,
      siteName: j['site_name'] as String? ?? '',
      sender: j['sender'] as String? ?? '',
      title: j['title'] as String? ?? '',
      date: j['date'] as String? ?? '',
      url: j['url'] as String? ?? '',
      isRead: j['is_read'] as bool? ?? false,
      content: j['content'] as String? ?? '',
      mailbox: Mailbox.values.firstWhere(
        (m) => mailboxParam(m) == (j['mailbox'] as String? ?? 'inbox'),
        orElse: () => Mailbox.inbox,
      ),
      adminActions: aa == null ? null : AdminActions.fromJson(aa),
    );
  }
}

class MessageResult {
  final int siteId;
  final String siteName;
  final List<Message> messages;
  final String error;
  MessageResult({required this.siteId, required this.siteName, required this.messages, required this.error});
  factory MessageResult.fromJson(Map<String, dynamic> j) => MessageResult(
        siteId: (j['site_id'] as num?)?.toInt() ?? 0,
        siteName: j['site_name'] as String? ?? '',
        messages: ((j['messages'] as List?) ?? const [])
            .map((e) => Message.fromJson(e as Map<String, dynamic>))
            .toList(),
        error: j['error'] as String? ?? '',
      );
}

class MessageApi {
  final _api = ApiClient();
  Future<List<MessageResult>> all(Mailbox mailbox) async {
    final r = await _api.get('/messages?mailbox=${mailboxParam(mailbox)}');
    final list = (r['sites'] as List?) ?? const [];
    return list.map((e) => MessageResult.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<String> content(int siteId, String url) async {
    final r = await _api.get('/messages/$siteId/content?url=${Uri.encodeQueryComponent(url)}');
    return r['content'] as String? ?? '';
  }

  Future<Map<String, dynamic>> reply(int siteId, String url, String content) =>
      _api.post('/messages/$siteId/reply', {'url': url, 'content': content});

  Future<void> remove(int siteId, String url) async {
    await _api.post('/messages/$siteId/delete', {'url': url});
  }

  /// 标记已读。返回响应体：站点常回 HTTP 200 却 `failed:1`（或明确不支持远程标记），
  /// 成败要靠 body 判定，不能只等 Future 完成。
  Future<Map<String, dynamic>> markRead(int siteId, List<String> ids) =>
      _api.post('/messages/$siteId/markread', {'ids': ids});
}
