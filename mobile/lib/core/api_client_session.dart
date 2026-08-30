// ApiClient 的会话/认证扩展：token 存取、baseUrl 切换、401 刷新登出。
// （从 api_client.dart 拆出，part of 主库；行为不变。）
part of 'package:ptpatronus/core/api_client.dart';

extension ApiClientSessionExt on ApiClient {
  /// 启动时从持久存储恢复 token + user 身份。
  /// token 优先读安全存储；老版本遗留在 SharedPreferences 的 token 读取后即迁移
  /// （迁完删旧键，一次性完成），迁移失败不影响本次会话（下次启动再试）。
  Future<void> loadToken() async {
    try {
      token = await ApiClient._secureStorage.read(key: ApiClient._kToken);
    } catch (_) {
      token = null; // 安全存储不可用（极少数定制 ROM）：本次会话无 token，重新登录
    }
    if (token == null || token!.isEmpty) {
      final p = await SharedPreferences.getInstance();
      final legacy = p.getString(ApiClient._kToken);
      if (legacy != null && legacy.isNotEmpty) {
        token = legacy;
        await setToken(legacy); // 写入安全存储并清 SharedPreferences 旧键
      }
    }
    final p = await SharedPreferences.getInstance();
    final us = p.getString(ApiClient._kUser);
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
    _runtimeBaseUrl = p.getString(ApiClient._kBaseUrl);
    _allowInsecureHttp = p.getBool(ApiClient._kAllowInsecureHttp) ?? false;
  }

  /// 设置并持久化后端地址（空串/null = 恢复编译期默认 Config.baseUrl）。
  Future<void> setBaseUrl(String? url) async {
    final v = (url != null && url.isNotEmpty) ? url : null;
    _runtimeBaseUrl = v;
    final p = await SharedPreferences.getInstance();
    if (v == null) {
      await p.remove(ApiClient._kBaseUrl);
    } else {
      await p.setString(ApiClient._kBaseUrl, v);
    }
  }

  Future<void> setAllowInsecureHttp(bool v) async {
    _allowInsecureHttp = v;
    final p = await SharedPreferences.getInstance();
    if (v) {
      await p.setBool(ApiClient._kAllowInsecureHttp, true);
    } else {
      await p.remove(ApiClient._kAllowInsecureHttp);
    }
  }

  /// 持久化登录用户身份（null/空=清除，与 logout 对齐）。
  Future<void> setUser(Map<String, dynamic>? u) async {
    user = u;
    final p = await SharedPreferences.getInstance();
    if (u == null || u.isEmpty) {
      await p.remove(ApiClient._kUser);
    } else {
      await p.setString(ApiClient._kUser, jsonEncode(u));
    }
  }

  /// 登录成功写入 / 登出清除（安全存储持久化；同时清 SharedPreferences 旧键防回退）。
  Future<void> setToken(String? t) async {
    token = t;
    if (t != null && t.isNotEmpty) _expired = false; // 新登录：重置 401 单飞
    final p = await SharedPreferences.getInstance();
    try {
      if (t == null || t.isEmpty) {
        await ApiClient._secureStorage.delete(key: ApiClient._kToken);
      } else {
        await ApiClient._secureStorage.write(key: ApiClient._kToken, value: t);
      }
    } catch (_) {
      // 安全存储不可用：仅内存态（重启需重新登录），宁可不可用也不落明文 SharedPreferences
    }
    await p.remove(ApiClient._kToken); // 迁移完成/登出：旧明文位置必须清空
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

  /// 统一 401 处理：清 token + user（内存+持久）、单飞、回调 UI 回登录页。
  /// 替代原先三处分散的"只清 token、不清 user、不跳登录"导致的僵尸已登录态。
  void _handle401() {
    if (_expired) return;
    _expired = true;
    token = null;
    user = null;
    // 同步清安全存储 + SharedPreferences 旧键与 user 快照
    ApiClient._secureStorage.delete(key: ApiClient._kToken).catchError((_) {});
    SharedPreferences.getInstance().then((p) {
      p.remove(ApiClient._kToken);
      p.remove(ApiClient._kUser);
    });
    ApiClient.onSessionExpired?.call();
  }

  /// 401 时先尝试一次 /auth/refresh 滑动续期（单飞互斥），成功则重放原请求；
  /// 续期失败或会话超绝对寿命 → 原有 _handle401 清洗回登录页。
  /// 后端 Refresh 已有 7 天绝对寿命钳制；此处仅做透明续期，不改变错误语义。
  Future<http.Response?> _tryRefresh() async {
    if (_refreshing) return null;
    final t = token;
    if (t == null || t.isEmpty) return null;
    _refreshing = true;
    try {
      final r = await ApiClient._sharedClient
          .post(_u('/auth/refresh'), headers: {
            'Content-Type': 'application/json',
            'Authorization': 'Bearer $t',
          })
          .timeout(ApiClient._requestTimeout);
      if (r.statusCode != 200) return null;
      final body = jsonDecode(r.body);
      if (body is! Map<String, dynamic>) return null;
      final newToken = body['token'] as String?;
      if (newToken == null || newToken.isEmpty) return null;
      token = newToken;
      try {
        await ApiClient._secureStorage.write(key: ApiClient._kToken, value: newToken);
      } catch (_) {/* 安全存储不可用则仅内存态 */}
      return r; // 调用方据 headers 里的旧 token 重放
    } catch (_) {
      return null;
    } finally {
      _refreshing = false;
    }
  }
}
