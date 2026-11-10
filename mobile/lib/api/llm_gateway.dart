// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../core/api_client.dart';

/// AI 网关配置（对齐后端 /llmgateway/config）。
///
/// 把内置 AI Agent 以 OpenAI / Anthropic 兼容协议对外暴露，供 Cursor /
/// Claude Desktop / continue.dev 等外部客户端直接驱动。
///
/// GET 返回脱敏视图：[accessTokenConfigured] 标记访问令牌是否已设置（明文令牌不回传）；
/// PUT 接受同形 body，[accessToken] 字段留空 = 保留原令牌（见后端 llm_gateway.go
/// `llmGatewayConfigReq` + `SetConfig`：access_token 非空才加密覆盖）。
class LlmGatewayConfig {
  final bool enabled;
  final bool accessTokenConfigured;
  final List<String> allowedModels; // 空列表 = 接受任意 model
  final int maxConcurrency; // 并发代理上限（0=不限）

  LlmGatewayConfig({
    required this.enabled,
    required this.accessTokenConfigured,
    required this.allowedModels,
    required this.maxConcurrency,
  });

  factory LlmGatewayConfig.fromJson(Map<String, dynamic> j) => LlmGatewayConfig(
        enabled: j['enabled'] as bool? ?? false,
        accessTokenConfigured: j['access_token_configured'] as bool? ?? false,
        allowedModels:
            (j['allowed_models'] as List?)?.map((e) => e.toString()).toList() ??
                const [],
        maxConcurrency: (j['max_concurrency'] as num?)?.toInt() ?? 4,
      );

  LlmGatewayConfig copyWith({
    bool? enabled,
    bool? accessTokenConfigured,
    List<String>? allowedModels,
    int? maxConcurrency,
  }) =>
      LlmGatewayConfig(
        enabled: enabled ?? this.enabled,
        accessTokenConfigured:
            accessTokenConfigured ?? this.accessTokenConfigured,
        allowedModels: allowedModels ?? this.allowedModels,
        maxConcurrency: maxConcurrency ?? this.maxConcurrency,
      );
}

/// AI 网关 API（GET / PUT /llmgateway/config，管理员）。
class LlmGatewayApi {
  final _api = ApiClient();

  /// 读配置（脱敏）。
  Future<LlmGatewayConfig> get() async {
    final r = await _api.get('/llmgateway/config');
    return LlmGatewayConfig.fromJson(r);
  }

  /// 写配置。`body` 中 `access_token` 仅在用户输入新令牌时携带（留空=保留原令牌）。
  /// 返回更新后的脱敏配置。
  Future<LlmGatewayConfig> update(Map<String, dynamic> body) async {
    final r = await _api.put('/llmgateway/config', body);
    return LlmGatewayConfig.fromJson(r);
  }
}
