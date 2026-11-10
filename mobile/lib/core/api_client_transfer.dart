// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// ApiClient 的字节流/上传下载/流式扩展（从 api_client.dart 拆出，part of 主库；行为不变）。
part of 'package:ptpatronus/core/api_client.dart';

extension ApiClientTransferExt on ApiClient {
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
        String? errCode;
        try {
          final b = jsonDecode(e.response?.data ?? '') as Map<String, dynamic>;
          msg = (b['error'] as String?) ?? (b['message'] as String?) ?? msg;
          errCode = b['code'] as String?;
        } catch (_) {}
        throw ApiException(code, msg, code: errCode);
      }
      throw ApiException(0, '${e.message}', userMessage: '网络连接失败，请检查网络或后端地址');
    } catch (e) {
      throw ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址');
    } finally {
      dio.close(force: true);
    }
  }

  /// SSE 流式 GET（聚合页「加载一个显示一个」）：发送带 JWT 的 GET，按空行分帧，逐帧产出
  /// (event, data)。返回的 Stream 取消监听即中断底层请求（onCancel 关 client）——切换筛选/刷新
  /// 时 cancel 上一次流，避免乱序回灌。
  Stream<MapEntry<String, Map<String, dynamic>>> streamGet(String path) {
    final client = IOClient(
        HttpClient()..connectionTimeout = const Duration(seconds: 15));
    final ctl = StreamController<MapEntry<String, Map<String, dynamic>>>();
    Future(() async {
      try {
        final uri = _u(path);
        final blocked = _insecureBlockedReason(uri);
        if (blocked != null) {
          ctl.addError(ApiException(0, blocked, userMessage: blocked));
          return;
        }
        final req = http.Request('GET', uri)
          ..headers['Authorization'] =
              (token != null && token!.isNotEmpty) ? 'Bearer $token' : '';
        final resp = await client.send(req).timeout(const Duration(seconds: 30));
        if (resp.statusCode != 200) {
          final body = await resp.stream.bytesToString();
          if (resp.statusCode == 401) _handle401();
          throw ApiException(resp.statusCode, _sseErrMsg(body),
              code: sseErrCode(body));
        }
        String buf = '';
        await for (final chunk in resp.stream.transform(utf8.decoder)) {
          if (ctl.isClosed) break;
          buf += chunk;
          Match? m;
          while ((m = RegExp(r'\r?\n\r?\n').firstMatch(buf)) != null) {
            final frame = buf.substring(0, m!.start);
            buf = buf.substring(m.end);
            final ev = _parseSSE(frame);
            if (ev != null && !ctl.isClosed) ctl.add(ev);
          }
        }
      } catch (e) {
        if (!ctl.isClosed) {
          ctl.addError(e is ApiException
              ? e
              : ApiException(0, '$e', userMessage: '网络连接失败，请检查网络或后端地址'));
        }
      } finally {
        if (!ctl.isClosed) ctl.close();
      }
    });
    ctl.onCancel = () => client.close();
    return ctl.stream;
  }

  /// 取二进制响应（如备份导出的文件流）：跳过 JSON 解析，返回原始字节。
  /// HTTP 错误（>=400）解析后端 error 文案抛 ApiException；401 复刻 _json 清 token。
  Future<Uint8List> getBytes(String path) async {
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
      final r = await _post(_u(path), headers: _headers, body: jsonEncode(body));
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
      return r.bodyBytes;
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
}
