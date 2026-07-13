import 'dart:convert';
import 'dart:typed_data';
import 'package:dio/dio.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';
import 'config.dart';

/// 轻量 HTTP 客户端：调后端 /api/v1，自动注入 JWT；token 持久化（shared_preferences）。
class ApiClient {
  static final ApiClient _i = ApiClient._();
  factory ApiClient() => _i;
  ApiClient._();

  static const _kToken = 'token';
  static const _kUser = 'user';
  static const _kBaseUrl = 'base_url';
  static const _kAllowInsecureHttp = 'allow_insecure_http';

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

  /// 启动时从持久存储恢复 token + user 身份。
  Future<void> loadToken() async {
    final p = await SharedPreferences.getInstance();
    token = p.getString(_kToken);
    final us = p.getString(_kUser);
    if (us != null && us.isNotEmpty) {
      try {
        user = jsonDecode(us) as Map<String, dynamic>;
      } catch (_) {
        user = null;
      }
    }
  }

  /// 启动时从持久存储恢复运行时后端地址。
  Future<void> loadBaseUrl() async {
    final p = await SharedPreferences.getInstance();
    _runtimeBaseUrl = p.getString(_kBaseUrl);
    _allowInsecureHttp = p.getBool(_kAllowInsecureHttp) ?? false;
  }

  /// 设置并持久化后端地址（空串/null = 恢复编译期默认 Config.baseUrl）。
  Future<void> setBaseUrl(String? url) async {
    final v = (url != null && url.isNotEmpty) ? url : null;
    _runtimeBaseUrl = v;
    final p = await SharedPreferences.getInstance();
    if (v == null) {
      await p.remove(_kBaseUrl);
    } else {
      await p.setString(_kBaseUrl, v);
    }
  }

  /// 是否允许公网地址走明文 http（默认 false：仅放行私网 http）。
  /// 用户在服务器配置页显式勾选「我知风险」才置 true——登录凭据将明文传输，自负风险。
  bool _allowInsecureHttp = false;
  bool get allowInsecureHttp => _allowInsecureHttp;

  Future<void> setAllowInsecureHttp(bool v) async {
    _allowInsecureHttp = v;
    final p = await SharedPreferences.getInstance();
    if (v) {
      await p.setBool(_kAllowInsecureHttp, true);
    } else {
      await p.remove(_kAllowInsecureHttp);
    }
  }

  /// 持久化登录用户身份（null/空=清除，与 logout 对齐）。
  Future<void> setUser(Map<String, dynamic>? u) async {
    user = u;
    final p = await SharedPreferences.getInstance();
    if (u == null || u.isEmpty) {
      await p.remove(_kUser);
    } else {
      await p.setString(_kUser, jsonEncode(u));
    }
  }

  /// 登录成功写入 / 登出清除（持久化）。
  Future<void> setToken(String? t) async {
    token = t;
    if (t != null && t.isNotEmpty) _expired = false; // 新登录：重置 401 单飞
    final p = await SharedPreferences.getInstance();
    if (t == null || t.isEmpty) {
      await p.remove(_kToken);
    } else {
      await p.setString(_kToken, t);
    }
  }

  /// 登出：调后端 bump TokenVersion（使当前 token 失效）+ 清本地 token。
  Future<void> logout() async {
    final t = token;
    if (t != null && t.isNotEmpty) {
      try {
        await post('/auth/logout', <String, dynamic>{});
      } catch (_) {
        /* token 已失效也无妨 */
      }
    }
    await setToken(null);
    await setUser(null);
  }

  /// 会话过期回调（由 main.dart 注入：断 WS + 回登录页）。本类不直接依赖 ws_client/main，避免循环 import。
  static void Function()? onSessionExpired;

  /// 并发 401 单飞标志：多请求同时 401 只清洗一次，避免重复清/重复 reload。
  bool _expired = false;

  /// 统一 401 处理：清 token + user（内存+持久）、单飞、回调 UI 回登录页。
  /// 替代原先三处分散的"只清 token、不清 user、不跳登录"导致的僵尸已登录态。
  void _handle401() {
    if (_expired) return;
    _expired = true;
    token = null;
    user = null;
    SharedPreferences.getInstance().then((p) {
      p.remove(_kToken);
      p.remove(_kUser);
    });
    onSessionExpired?.call();
  }

  Map<String, String> get _headers => {
        'Content-Type': 'application/json',
        if (token != null) 'Authorization': 'Bearer $token',
      };

  Uri _u(String path) => Uri.parse('$baseUrl/api/v1$path');

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
      _do(() => http.get(_u(path), headers: _headers));

  Future<Map<String, dynamic>> post(String path, Map<String, dynamic> body) =>
      _do(() => http.post(_u(path), headers: _headers, body: jsonEncode(body)));

  Future<Map<String, dynamic>> put(String path, Map<String, dynamic> body) =>
      _do(() => http.put(_u(path), headers: _headers, body: jsonEncode(body)));

  Future<Map<String, dynamic>> delete(String path) =>
      _do(() => http.delete(_u(path), headers: _headers));

  /// 取数组响应：后端部分端点直接回裸 JSON 数组（如 /dashboard/history/:id），
  /// 不能走 [_json] 的 `as Map` 解包。兼容裸数组与 {data|points|items|list:[...]} 包裹两种形态。
  Future<List<dynamic>> getList(String path) async {
    try {
      final r = await http.get(_u(path), headers: _headers);
      if (r.statusCode >= 400) {
        if (r.statusCode == 401) _handle401();
        var msg = '请求失败';
        try {
          final b = jsonDecode(r.body);
          if (b is Map<String, dynamic>) {
            msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
          }
        } catch (_) {}
        throw ApiException(r.statusCode, msg);
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

  /// 取二进制响应（如备份导出的文件流）：跳过 JSON 解析，返回原始字节。
  /// HTTP 错误（>=400）解析后端 error 文案抛 ApiException；401 复刻 _json 清 token。
  Future<Uint8List> getBytes(String path) async {
    try {
      final r = await http.get(_u(path), headers: _headers);
      if (r.statusCode >= 400) {
        if (r.statusCode == 401) _handle401();
        var msg = '请求失败';
        try {
          final b = jsonDecode(r.body);
          if (b is Map<String, dynamic>) {
            msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
          }
        } catch (_) {}
        throw ApiException(r.statusCode, msg);
      }
      return r.bodyBytes;
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  /// POST 取二进制响应（如字幕下载：POST body 指定字幕，返回文件字节）。跳过 JSON 解析。
  Future<Uint8List> postBytes(String path, Map<String, dynamic> body) async {
    try {
      final r = await http.post(_u(path), headers: _headers, body: jsonEncode(body));
      if (r.statusCode >= 400) {
        if (r.statusCode == 401) _handle401();
        var msg = '请求失败';
        try {
          final b = jsonDecode(r.body);
          if (b is Map<String, dynamic>) {
            msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
          }
        } catch (_) {}
        throw ApiException(r.statusCode, msg);
      }
      return r.bodyBytes;
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  /// 流式下载到文件（大文件避免一次性入内存；支持进度/取消）。用 dio，复用 JWT header。
  /// HTTP 错误（>=400）解析后端 error 文案抛 ApiException；401 走全局清洗。
  Future<void> downloadToFile(
    String path,
    String savePath, {
    void Function(int received, int total)? onProgress,
    CancelToken? cancelToken,
  }) async {
    final dio = Dio(BaseOptions(
      connectTimeout: const Duration(seconds: 15),
      receiveTimeout: const Duration(minutes: 30),
      followRedirects: true,
    ));
    try {
      dio.options.headers.addAll(_headers);
      await dio.download(
        _u(path).toString(),
        savePath,
        onReceiveProgress: onProgress,
        cancelToken: cancelToken,
      );
    } on DioException catch (e) {
      final code = e.response?.statusCode ?? 0;
      if (code == 401) _handle401();
      if (code >= 400) {
        var msg = '下载失败';
        try {
          final b = jsonDecode(e.response?.data ?? '') as Map<String, dynamic>;
          msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
        } catch (_) {}
        throw ApiException(code, msg);
      }
      throw ApiException(0, '${e.message}', userMessage: '网络连接失败，请检查网络或后端地址');
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    } finally {
      dio.close(force: true);
    }
  }

  /// 发送任意 JSON body（支持数组，如 MoviePilot 站点数组）；响应按对象 Map 解包。
  Future<Map<String, dynamic>> postJson(String path, Object body) =>
      _do(() => http.post(_u(path), headers: _headers, body: jsonEncode(body)));

  /// 统一请求包装：网络层错误（连接失败/超时/DNS）转为友好 ApiException；HTTP 错误由 _json 转。
  Future<Map<String, dynamic>> _do(Future<http.Response> Function() fn) async {
    try {
      return _json(await fn());
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  /// multipart 上传（.torrent 文件等）。不设 Content-Type——MultipartRequest 自动生成 boundary。
  Future<Map<String, dynamic>> postMultipart(
    String path, {
    List<http.MultipartFile> files = const [],
    Map<String, String> fields = const {},
  }) async {
    try {
      final req = http.MultipartRequest('POST', _u(path));
      if (token != null && token!.isNotEmpty) {
        req.headers['Authorization'] = 'Bearer $token';
      }
      req.fields.addAll(fields);
      for (final f in files) {
        req.files.add(f);
      }
      final streamed = await req.send();
      final resp = await http.Response.fromStream(streamed);
      return _json(resp);
    } on ApiException {
      rethrow;
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    }
  }

  Map<String, dynamic> _json(http.Response r) {
    final body = r.body.isEmpty
        ? <String, dynamic>{}
        : jsonDecode(r.body) as Map<String, dynamic>;
    if (r.statusCode >= 400) {
      // 401：token 失效/已登出 → 全局清洗（清 token+user、断 WS、回登录页）
      if (r.statusCode == 401) _handle401();
      throw ApiException(
          r.statusCode, (body['error'] as String?) ?? (body['message'] as String?) ?? '请求失败');
    }
    return body;
  }
}

class ApiException implements Exception {
  final int status;
  final String message; // 原始后端消息（日志/调试用）
  final String userMessage; // 给用户看的友好文案
  ApiException(this.status, this.message, {String? userMessage})
      : userMessage = userMessage ?? friendlyMessage(status, message);
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
