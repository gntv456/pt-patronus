import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../api/downloader.dart';
import '../api/notice.dart';
import '../core/api_client.dart';
import '../core/config.dart';

// 单下载器的实时种子快照（对齐后端 ws.TorrentSnapshot）。
class TorrentSnapshot {
  final int downloaderId;
  final String name;
  final String error;
  final List<Torrent> torrents;
  // 全局速率 bytes/s（qb 零请求取；tr 留空为 null）
  final int? dlRate;
  final int? upRate;

  TorrentSnapshot({
    required this.downloaderId,
    required this.name,
    required this.error,
    required this.torrents,
    this.dlRate,
    this.upRate,
  });

  factory TorrentSnapshot.fromJson(Map<String, dynamic> j) {
    final stats = j['stats'] as Map<String, dynamic>?;
    return TorrentSnapshot(
      downloaderId: (j['downloader_id'] as num?)?.toInt() ?? 0,
      name: j['name'] as String? ?? '',
      error: j['error'] as String? ?? '',
      torrents: ((j['torrents'] as List?) ?? const [])
          .map((e) => Torrent.fromJson(e as Map<String, dynamic>))
          .toList(),
      dlRate: (stats?['dl_rate'] as num?)?.toInt(),
      upRate: (stats?['up_rate'] as num?)?.toInt(),
    );
  }
}

// 一条 H&R 危险种子（对齐后端 ws.HRAlert）。
class HRAlert {
  final String site;
  final String title;
  final String status;
  final String deadline;

  HRAlert({
    required this.site,
    required this.title,
    required this.status,
    required this.deadline,
  });

  factory HRAlert.fromJson(Map<String, dynamic> j) => HRAlert(
        site: j['site'] as String? ?? '',
        title: j['title'] as String? ?? '',
        status: j['status'] as String? ?? '',
        deadline: j['deadline'] as String? ?? '',
      );
}

// WsClient 连后端实时推送 WS（/api/v1/ws?ticket=<一次性>），监听种子快照广播流，
// 断线指数退避重连。单例：同一时刻一个连接服务所有订阅者。
// 先用 header JWT 换一次性 ticket 再连（JWT 不落 URL），对齐 Web useWs。
class WsClient {
  WsClient._();
  static final WsClient _i = WsClient._();
  factory WsClient() => _i;

  WebSocketChannel? _ch;
  StreamSubscription? _sub;
  int _retry = 0;
  bool _disposed = false; // 进程退出（永久）：停止重连 + 关闭广播流
  bool _stopped = false; // 登出（可逆）：停止重连，但保留单例与广播流供重登后 connect()
  Timer? _reconnectTimer; // 待执行的重连定时器（登出时可取消，避免旧 token 换 ticket 失败循环）
  final _ctl = StreamController<List<TorrentSnapshot>>.broadcast();
  final _ctlHR = StreamController<List<HRAlert>>.broadcast();
  final _ctlCookie = StreamController<List<String>>.broadcast();
  final _ctlNotice = StreamController<Notice>.broadcast();

  /// 种子快照广播流。订阅即收后续推送；首帧由后端握手后单推。
  Stream<List<TorrentSnapshot>> get snapshots => _ctl.stream;

  /// H&R 告警广播流（hr-alert 消息）。
  Stream<List<HRAlert>> get hrAlerts => _ctlHR.stream;

  /// Cookie 失效告警广播流（cookie-alert 消息，data 为站点名列表）。
  Stream<List<String>> get cookieAlerts => _ctlCookie.stream;

  /// 通知中心实时推送流（notice 消息，data 为单条通知对象）。
  Stream<Notice> get notices => _ctlNotice.stream;

  /// 当前是否连着（UI 状态展示用）。
  bool get isConnected => _ch != null;

  /// 连接（幂等：已连则跳过）。先用 header JWT 换一次性 ticket，再用 ?ticket= 连（JWT 不落 URL）。
  Future<void> connect() async {
    if (_ch != null || _disposed) return;
    _stopped = false; // 重登后重新启用重连
    final api = ApiClient();
    final token = api.token;
    if (token == null || token.isEmpty) return;
    String ticket;
    try {
      ticket = (await api.post('/auth/ws-ticket', {}))['ticket'] as String? ?? '';
    } catch (_) {
      _scheduleReconnect();
      return;
    }
    if (ticket.isEmpty || _disposed) return;
    final base = Config.baseUrl.replaceFirst(RegExp(r'^http'), 'ws');
    final uri = Uri.parse('$base/api/v1/ws?ticket=${Uri.encodeQueryComponent(ticket)}');
    try {
      _ch = WebSocketChannel.connect(uri);
      _retry = 0;
      _sub = _ch!.stream.listen(_onData, onDone: _onDone, onError: (_) => _onDone());
    } catch (_) {
      _scheduleReconnect();
    }
  }

  void _onData(dynamic raw) {
    try {
      final j = jsonDecode(raw as String) as Map<String, dynamic>;
      final list = (j['data'] as List?) ?? const [];
      switch (j['type']) {
        case 'torrents':
          _ctl.add(list
              .map((e) => TorrentSnapshot.fromJson(e as Map<String, dynamic>))
              .toList());
          break;
        case 'hr-alert':
          _ctlHR.add(list
              .map((e) => HRAlert.fromJson(e as Map<String, dynamic>))
              .toList());
          break;
        case 'cookie-alert':
          _ctlCookie.add(list.map((e) => e as String).toList());
          break;
        case 'notice':
          final d = j['data'] as Map<String, dynamic>?;
          if (d != null) _ctlNotice.add(Notice.fromJson(d));
          break;
      }
    } catch (_) {
      // 忽略畸形帧
    }
  }

  void _onDone() {
    _ch = null;
    _sub?.cancel();
    _sub = null;
    _scheduleReconnect();
  }

  // 指数退避：2,4,6,8,10,12s…封顶 60s。用 Timer 而非 Future.delayed 以便登出时可取消。
  void _scheduleReconnect() {
    if (_disposed || _stopped) return;
    _retry++;
    final secs = _retry > 6 ? 60 : _retry * 2;
    _reconnectTimer?.cancel();
    _reconnectTimer = Timer(Duration(seconds: secs), () {
      if (!_disposed && !_stopped) connect();
    });
  }

  /// 登出时主动断开：取消待重连、关连接，但保留单例与广播流——重登后 connect() 即恢复。
  /// 不用 dispose()：那会置 _disposed 永久杀死单例，导致重登后 WS 再也连不上。
  void disconnect() {
    _stopped = true;
    _reconnectTimer?.cancel();
    _reconnectTimer = null;
    _retry = 0;
    _sub?.cancel();
    _sub = null;
    _ch?.sink.close();
    _ch = null;
  }

  /// 关闭并禁用（app 退出时调；登出请用 disconnect）。
  void dispose() {
    _disposed = true;
    _reconnectTimer?.cancel();
    _sub?.cancel();
    _ch?.sink.close();
    _ch = null;
    _ctl.close();
    _ctlHR.close();
    _ctlCookie.close();
    _ctlNotice.close();
  }
}
