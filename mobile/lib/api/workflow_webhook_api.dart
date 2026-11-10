// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 workflow.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/workflow.dart';

/// 进站 Webhook：外部 POST /api/v1/webhooks/in/:token → 触发目标工作流/广播事件/通知。
/// 对齐后端 models.Webhook（has_secret 回显字段）+ api/v1/webhook.go（CRUD admin 专属）。
class Webhook {
  final int id;
  final String name;
  final String token; // URL 密钥（管理端可见，用于拼接收 URL）
  final int targetWorkflowId; // 0=不直接触发
  final bool hasSecret; // 是否设了 HMAC 签名密钥
  final bool enabled;
  final int triggerCount;
  final String? lastTriggeredAt;
  const Webhook({
    this.id = 0,
    this.name = '',
    this.token = '',
    this.targetWorkflowId = 0,
    this.hasSecret = false,
    this.enabled = false,
    this.triggerCount = 0,
    this.lastTriggeredAt,
  });
  factory Webhook.fromJson(Map<String, dynamic> j) => Webhook(
        id: (j['id'] as num?)?.toInt() ?? 0,
        name: j['name'] as String? ?? '',
        token: j['token'] as String? ?? '',
        targetWorkflowId: (j['target_workflow_id'] as num?)?.toInt() ?? 0,
        hasSecret: j['has_secret'] as bool? ?? false,
        enabled: j['enabled'] as bool? ?? false,
        triggerCount: (j['trigger_count'] as num?)?.toInt() ?? 0,
        lastTriggeredAt: j['last_triggered_at'] as String?,
      );

  /// 接收 URL：后端公开端点 /api/v1/webhooks/in/:token。
  String receiveUrl(String baseUrl) => '$baseUrl/api/v1/webhooks/in/$token';
}

/// Webhook 管理端 CRUD（create/update/delete/rotate 后端 RequireAdmin）。
/// 对齐 frontend src/api/webhook.ts：list/create/update/rotate/remove。
class WebhookApi {
  final _api = ApiClient();

  Future<List<Webhook>> list() async {
    final r = await _api.get('/webhooks');
    final list = (r['webhooks'] as List?) ?? const [];
    return list.map((e) => Webhook.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<Webhook> create(Map<String, dynamic> body) async {
    final r = await _api.postJson('/webhooks', body);
    return Webhook.fromJson(r);
  }

  Future<Webhook> update(int id, Map<String, dynamic> body) async {
    final r = await _api.put('/webhooks/$id', body);
    return Webhook.fromJson(r);
  }

  /// 重生成 token（旧 URL 立即失效）。返回新 token。
  Future<String> rotate(int id) async {
    final r = await _api.post('/webhooks/$id/rotate', {});
    return (r['token'] as String?) ?? '';
  }

  Future<void> remove(int id) => _api.delete('/webhooks/$id');
}
