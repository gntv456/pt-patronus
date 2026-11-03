import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:http/http.dart' as http;
// 连接超时需要 dart:io HttpClient + package:http 的 IOClient 包装
// （Web 平台不可用；本项目未发 Web 端，与 features 下直接 import dart:io 的既有口径一致）。
import 'dart:io' show HttpClient;
import 'package:http/io_client.dart' show IOClient;
import 'package:shared_preferences/shared_preferences.dart';
import 'config.dart';

part 'api_client_session.dart';
part 'api_client_transfer.dart';

/// 轻量 HTTP 客户端：调后端 /api/v1，自动注入 JWT。
/// token 持久化走 flutter_secure_storage（收权加固：Keychain/Keystore/Win Credential
/// Manager，防 root/备份直接提取）；非敏感配置（baseUrl/允许明文开关）仍走
/// SharedPreferences。user 身份为只读展示数据（无凭据），留在 SharedPreferences。
class ApiClient {
  static final ApiClient _i = ApiClient._();
  factory ApiClient() => _i;
  ApiClient._();

  static const _kToken = 'token';
  static const _kUser = 'user';
  static const _kBaseUrl = 'base_url';
  static const _kAllowInsecureHttp = 'allow_insecure_http';

  /// 安全存储（token 专用）。默认实现：iOS/macOS Keychain、Android 自动迁移的
  /// 自定义加密、Windows Credential Manager——均非明文，防 root/备份直接提取。
  static const _secureStorage = FlutterSecureStorage();

  /// 运行时配的后端地址（SharedPreferences 持久化），优先于编译期 [Config.baseUrl]。
  /// 分发场景：每个用户后端不同，APP 首次启动让用户填，存本地。
  String? _runtimeBaseUrl;

  /// 实际使用的后端地址：运行时配置 > 编译期 Config.baseUrl。
  String get baseUrl => (_runtimeBaseUrl != null && _runtimeBaseUrl!.isNotEmpty)
      ? _runtimeBaseUrl!
      : Config.baseUrl;

  /// 是否已配置过后端地址（首次启动判断：false → 引导到服务器配置页）。
  bool get hasCustomBaseUrl =>
      _runtimeBaseUrl != null && _runtimeBaseUrl!.isNotEmpty;

  /// 登录后写入；后续请求自动带 Authorization 头。
  String? token;

  /// 当前登录用户身份（username/is_admin/vip_level 等），随 token 一起持久化，重启后恢复。
  Map<String, dynamic>? user;

  /// 是否管理员（备份导入 / webhook / agent / 删除等 Web 端 admin 守卫的客户端镜像）。
  /// 后端仍是权威鉴权；此 getter 仅用于 UI 显隐，与 Web 的 auth.isAdmin 对齐。
  bool get isAdmin => user?['is_admin'] == true;

  /// 是否允许公网地址走明文 http（默认 false：仅放行私网 http）。
  /// 用户在服务器配置页显式勾选「我知风险」才置 true——登录凭据将明文传输，自负风险。
  bool _allowInsecureHttp = false;
  bool get allowInsecureHttp => _allowInsecureHttp;

  /// 会话过期回调（由 main.dart 注入：断 WS + 回登录页）。本类不直接依赖 ws_client/main，避免循环 import。
  static void Function()? onSessionExpired;

  /// 并发 401 单飞标志：多请求同时 401 只清洗一次，避免重复清/重复 reload。
  bool _expired = false;

  Map<String, String> get _headers => {
        'Content-Type': 'application/json',
        if (token != null) 'Authorization': 'Bearer $token',
      };

  Uri _u(String path) => Uri.parse('$baseUrl/api/v1$path');

  /// 明文 http 应用层防线（收权加固）：公网地址 + http + 未显式勾选「我知风险」
  /// → 直接拒绝请求（配置页只拦一次，这里对每个请求强制；私网 http 照常放行）。
  /// 返回 null=放行，否则为拒绝理由（调用方转 ApiException）。
  String? _insecureBlockedReason(Uri u) {
    if (u.scheme != 'http' || _allowInsecureHttp) return null;
    final h = u.host;
    final isPrivate =
        h == 'localhost' || h == '127.0.0.1' || h == '0.0.0.0' || h == '10.0.2.2' ||
            h.startsWith('10.') ||
            h.startsWith('192.168.') ||
            RegExp(r'^172\.(1[6-9]|2\d|3[01])\.').hasMatch(h) ||
            h.endsWith('.local'); // mDNS 局域网主机
    if (isPrivate) return null;
    return '公网地址不允许明文 http：请在服务器配置页改用 https，或显式勾选「允许公网 http（我知风险）」';
  }

  // ── 统一超时（收权加固：http 包默认无超时，弱网下 UI 无限挂起）──
  // 连接 15s / 常规请求 60s。长耗时场景走专用通道：SSE 不设 receive 超时、
  // 大文件下载走 dio 的 downloadToFile（30min 接收窗口）。
  // 复用单例连接池（原先每请求新建 HttpClient 且不 close：弱网高频请求下
  // TLS 握手开销大 + FD 压力；连接复用后 keep-alive 生效）。
  static final http.Client _sharedClient = IOClient(
    HttpClient()..connectionTimeout = const Duration(seconds: 15),
  );
  http.Client _newClient() => _sharedClient;
  static const _requestTimeout = Duration(seconds: 60);

  Future<http.Response> _get(Uri u, {Map<String, String>? headers}) async {
    final blocked = _insecureBlockedReason(u);
    if (blocked != null) throw ApiException(0, blocked, userMessage: blocked);
    return _withRetry(() => _newClient().get(u, headers: headers).timeout(_requestTimeout));
  }

  Future<http.Response> _post(Uri u, {Map<String, String>? headers, Object? body}) async {
    final blocked = _insecureBlockedReason(u);
    if (blocked != null) throw ApiException(0, blocked, userMessage: blocked);
    return _newClient().post(u, headers: headers, body: body).timeout(_requestTimeout);
  }

  Future<http.Response> _put(Uri u, {Map<String, String>? headers, Object? body}) async {
    final blocked = _insecureBlockedReason(u);
    if (blocked != null) throw ApiException(0, blocked, userMessage: blocked);
    return _newClient().put(u, headers: headers, body: body).timeout(_requestTimeout);
  }

  Future<http.Response> _delete(Uri u, {Map<String, String>? headers, Object? body}) async {
    final blocked = _insecureBlockedReason(u);
    if (blocked != null) throw ApiException(0, blocked, userMessage: blocked);
    return _withRetry(() => _newClient().delete(u, headers: headers, body: body).timeout(_requestTimeout));
  }

  /// 幂等 GET/DELETE 的网络层重试（连接失败/超时重试 1 次，指数退避）。
  /// HTTP 层错误（>=400）不重试——那是后端语义，不是网络抖动。
  Future<http.Response> _withRetry(Future<http.Response> Function() fn) async {
    try {
      return await fn();
    } catch (e) {
      if (e is ApiException) rethrow;
      await Future<void>.delayed(const Duration(milliseconds: 800));
      return fn();
    }
  }

  /// 续期单飞：并发 401 只发起一次 /auth/refresh。
  bool _refreshing = false;

  /// 仅含 Authorization 的请求头（供 Image.network 取反代图片用）。
  Map<String, String> get authHeaders => {
        if (token != null && token!.isNotEmpty) 'Authorization': 'Bearer $token',
      };

  /// 豆瓣 doubanio.com 防盗链：浏览器 / Image.network 直连只回 1×1 占位图。
  /// 豆瓣图改走后端 /media/image 反代（注入 movie.douban.com Referer 取真图）；
  /// 其它公网 CDN 直连，返回 null。返回的 Uri 需配合 [authHeaders] 加载。
  Uri? mediaImageUri(String posterUrl) {
    if (posterUrl.isEmpty || !posterUrl.contains('doubanio.com')) return null;
    return _u('/media/image?url=${Uri.encodeComponent(posterUrl)}');
  }

  Future<Map<String, dynamic>> get(String path) =>
      _do(() => _get(_u(path), headers: _headers), path: path, method: 'GET');

  Future<Map<String, dynamic>> post(String path, Map<String, dynamic> body) =>
      _do(() => _post(_u(path), headers: _headers, body: jsonEncode(body)),
          path: path, method: 'POST');

  Future<Map<String, dynamic>> put(String path, Map<String, dynamic> body) =>
      _do(() => _put(_u(path), headers: _headers, body: jsonEncode(body)),
          path: path, method: 'PUT');

  Future<Map<String, dynamic>> delete(String path, {Object? body}) =>
      _do(() => _delete(_u(path), headers: _headers, body: body), path: path, method: 'DELETE');

  /// 取数组响应：后端部分端点直接回裸 JSON 数组（如 /dashboard/history/:id），
  /// 不能走 [_json] 的 `as Map` 解包。兼容裸数组与 {data|points|items|list:[...]} 包裹两种形态。
  Future<List<dynamic>> getList(String path) async {
    try {
      final r = await _get(_u(path), headers: _headers);
      if (r.statusCode >= 400) {
        if (r.statusCode == 401) _handle401();
        var msg = '请求失败';
        String? errCode;
        try {
          final b = jsonDecode(r.body);
          if (b is Map<String, dynamic>) {
            msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
            errCode = b['code'] as String?;
          }
        } catch (_) {}
        throw ApiException(r.statusCode, msg, code: errCode);
      }
      final body = r.body.isEmpty ? <dynamic>[] : jsonDecode(r.body);
      if (body is List) return body;
      if (body is Map<String, dynamic>) {
        for (final k in const ['data', 'points', 'items', 'list', 'results']) {
          final v = body[k];
          if (v is List) return v;
        }
      }
      return const [];
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  /// 发送任意 JSON body（支持数组，如 MoviePilot 站点数组）；响应按对象 Map 解包。
  /// method 留 null：401 续期后不重放（非幂等 POST），直接按原响应走 _json 语义。
  Future<Map<String, dynamic>> postJson(String path, Object body) =>
      _do(() => _post(_u(path), headers: _headers, body: jsonEncode(body)), path: null);

  /// 统一请求包装：网络层错误（连接失败/超时/DNS）转为友好 ApiException；HTTP 错误由 _json 转。
  /// 401 先透明续期一次（成功则用新 token 重放原请求），仍 401 才走全局清洗。
  /// 仅幂等 GET 重放：POST/PUT 无法安全重建 body，续期成功也直接按原 401 响应走 _json 语义
  /// （调用方收到 401 提示重登；避免把写操作静默降级成 GET）。
  Future<Map<String, dynamic>> _do(Future<http.Response> Function() fn,
      {String? path, String method = 'GET'}) async {
    try {
      var r = await fn();
      if (r.statusCode == 401 && path != null && path != '/auth/refresh') {
        final refreshed = await _tryRefresh();
        if (refreshed != null && method == 'GET') {
          // 用新 token 重放一次（重放请求不再触发续期，防循环）。
          r = await _replay(path, fn);
        }
      }
      return _json(r);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  /// 用更新后的 token 重放：闭包 fn 已绑定旧 headers，这里按 path 重建。
  /// 仅 GET 通道调用（_do 已按 method 守卫）；非 GET 不会走到这里。
  Future<http.Response> _replay(String path, Future<http.Response> Function() fn) async {
    final h = _headers;
    final u = _u(path);
    return _sharedClient.get(u, headers: h).timeout(_requestTimeout);
  }

  Map<String, dynamic> _json(http.Response r) {
    // 裸数组/标量响应防御：get()/post() 通道按 Map 解包，非 Map body 不再 crash
    // （getList 已兼容裸数组；这里防御误走通道的形态，返回包裹结果保语义）。
    Map<String, dynamic> body;
    if (r.body.isEmpty) {
      body = <String, dynamic>{};
    } else {
      final decoded = jsonDecode(r.body);
      if (decoded is Map<String, dynamic>) {
        body = decoded;
      } else if (decoded is List) {
        body = <String, dynamic>{'data': decoded};
      } else {
        body = <String, dynamic>{'data': decoded};
      }
    }
    if (r.statusCode >= 400) {
      // 401：token 失效/已登出 → 全局清洗（清 token+user、断 WS、回登录页）
      if (r.statusCode == 401) _handle401();
      final code = body['code'] as String?;
      // VIP 校验类 403：闸门已随好学站点关闭退役（#<issue>），这些码不再出现；
      // 保留映射一版兼容旧后端，文案改为通用会话提示。
      String? vipMsg;
      if (r.statusCode == 403) {
        vipMsg = switch (code) {
          'vip_expired' => '登录态已过期，请重新登录',
          'vip_not_verified' => '登录态已过期，请重新登录',
          'vip_unknown' => '登录态异常，请稍后重试或重新登录',
          _ => null,
        };
      }
      throw ApiException(r.statusCode,
          (body['error'] as String?) ?? (body['message'] as String?) ?? '请求失败',
          userMessage: vipMsg, code: code);
    }
    return body;
  }
}

class ApiException implements Exception {
  final int status;
  final String message; // 原始后端消息（日志/调试用）
  final String userMessage; // 给用户看的友好文案
  /// 机器可读错误码（后端统一信封 {code, message, error}）。空串 = 后端未码化。
  /// 仅作编程分支（如 risk_confirmation_required 弹风险确认），不影响文案展示。
  final String code;
  ApiException(this.status, this.message, {String? userMessage, String? code})
      : userMessage = userMessage ?? friendlyMessage(status, message),
        code = code ?? '';
  @override
  String toString() => userMessage;
}

/// 状态码/后端消息 → 给用户看的中文文案。后端已给可读消息时优先用，避免显示 ApiException(status)。
String friendlyMessage(int status, String backendMsg) {
  if (backendMsg.isNotEmpty && backendMsg != '请求失败') return backendMsg;
  if (status == 401) return '登录已失效，请重新登录';
  if (status == 403) return '没有权限执行此操作';
  if (status == 404) return '请求的资源不存在';
  if (status == 429) return '操作过于频繁，请稍后再试';
  if (status >= 500) return '服务暂时不可用，请稍后重试';
  return '请求失败，请稍后重试';
}

/// 解析一帧 SSE（event:/data:），data 按换行拼接后 JSON 解析。
MapEntry<String, Map<String, dynamic>>? _parseSSE(String frame) {
  String event = 'message';
  final dataLines = <String>[];
  for (final line in frame.split(RegExp(r'\r?\n'))) {
    if (line.startsWith('event:')) {
      event = line.substring(6).trim();
    } else if (line.startsWith('data:')) {
      dataLines.add(line.substring(5).trim());
    }
  }
  if (dataLines.isEmpty) return null;
  try {
    return MapEntry(event, jsonDecode(dataLines.join('\n')) as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
}

String _sseErrMsg(String body) {
  try {
    final b = jsonDecode(body) as Map<String, dynamic>;
    return (b['error'] as String?) ?? (b['message'] as String?) ?? '请求失败';
  } catch (_) {
    return '请求失败';
  }
}

/// 从 SSE 错误 body 提取机器可读错误码（统一信封 {code, message, error}）。公开供测试。
String? sseErrCode(String body) {
  try {
    final b = jsonDecode(body);
    if (b is Map<String, dynamic>) return b['code'] as String?;
  } catch (_) {}
  return null;
}
