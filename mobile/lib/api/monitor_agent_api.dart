// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// 从 monitor.dart 拆出（part of 主库；行为不变）。
part of 'package:ptpatronus/api/monitor.dart';

/// Agent（push 架构的上报端，关联一个 host/nas|agent 的 MonitorTarget）。对齐后端 models.Agent。
/// SecretHash 标 json:"-" 不回传；创建/重置时后端额外返回明文 plain_secret（仅一次）。
class Agent {
  final int id;
  final String name;
  final String secretHint; // 明文前 8 位（运维辨认）
  final bool enabled;
  final int targetId; // 1:1 关联 MonitorTarget
  final bool online; // WS 是否在线
  final String version;
  final String hostname;
  final String ip;
  final String lastSeenAt;
  Agent({
    required this.id,
    required this.name,
    required this.secretHint,
    required this.enabled,
    required this.targetId,
    required this.online,
    required this.version,
    required this.hostname,
    required this.ip,
    required this.lastSeenAt,
  });
  factory Agent.fromJson(Map<String, dynamic> j) => Agent(
    id: (j['id'] as num?)?.toInt() ?? 0,
    name: j['name'] as String? ?? '',
    secretHint: j['secret_hint'] as String? ?? '',
    enabled: j['enabled'] as bool? ?? true,
    targetId: (j['target_id'] as num?)?.toInt() ?? 0,
    online: j['online'] as bool? ?? false,
    version: j['version'] as String? ?? '',
    hostname: j['hostname'] as String? ?? '',
    ip: j['ip'] as String? ?? '',
    lastSeenAt: j['last_seen_at'] as String? ?? '',
  );
}

/// Agent 管理端 CRUD（对齐后端 /monitors/agents；create/delete/rotate 需 admin）。
/// create/rotate 返回明文 secret（仅此一次，需提示用户复制保存）。
class AgentApi {
  final _api = ApiClient();

  Future<List<Agent>> list() async {
    final r = await _api.get('/monitors/agents');
    final arr = (r['agents'] as List?) ?? const [];
    return arr.map((e) => Agent.fromJson(e as Map<String, dynamic>)).toList();
  }

  /// 创建 agent（后端事务化建 host/nas|agent target + agent），返回 agent 与明文 secret。
  Future<({Agent agent, String plainSecret})> create(
    String name, {
    String type = 'host',
  }) async {
    final r = await _api.post('/monitors/agents', {'name': name, 'type': type});
    return (
      agent: Agent.fromJson(
        (r['agent'] as Map?)?.cast<String, dynamic>() ?? const {},
      ),
      plainSecret: (r['plain_secret'] as String?) ?? '',
    );
  }

  Future<void> remove(int id) async => _api.delete('/monitors/agents/$id');

  /// 重置 secret（泄露后应急），返回新明文。
  Future<String> rotate(int id) async {
    final r = await _api.post('/monitors/agents/$id/rotate', {});
    return (r['plain_secret'] as String?) ?? '';
  }
}
