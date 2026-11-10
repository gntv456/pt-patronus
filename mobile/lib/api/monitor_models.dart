// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

part of 'monitor.dart';

/// 监控设备（host/nas/camera）。对齐后端 MonitorTarget（snake_case；凭据不回传）。
class MonitorTarget {
  final int id;
  final String name;
  final String type; // host | nas | camera
  final String host;
  final int port;
  final String vendor;
  final String protocol;
  // 绑定的凭据：PUT 是全量替换，漏传 = 解绑（后端把 0 当「不用凭据」）。
  final int credentialId;
  final String rtspUrl;
  final int intervalSec;
  final int timeoutSec;
  final bool enabled;
  final String thresholds;
  final String diskPaths;
  final bool hasPrivateKey; // 私钥已配置（明文不回传）
  final bool? lastOnline;
  final String lastError;
  final String lastCheckedAt;

  MonitorTarget({
    required this.id,
    required this.name,
    required this.type,
    required this.host,
    required this.port,
    required this.vendor,
    required this.protocol,
    this.credentialId = 0,
    required this.rtspUrl,
    required this.intervalSec,
    required this.timeoutSec,
    required this.enabled,
    required this.thresholds,
    required this.diskPaths,
    required this.hasPrivateKey,
    required this.lastOnline,
    required this.lastError,
    required this.lastCheckedAt,
  });

  factory MonitorTarget.fromJson(Map<String, dynamic> j) => MonitorTarget(
    id: (j['id'] as num?)?.toInt() ?? 0,
    name: j['name'] as String? ?? '',
    type: j['type'] as String? ?? '',
    host: j['host'] as String? ?? '',
    port: j['port'] as int? ?? 0,
    vendor: j['vendor'] as String? ?? '',
    protocol: j['protocol'] as String? ?? '',
    credentialId: (j['credential_id'] as num?)?.toInt() ?? 0,
    rtspUrl: j['rtsp_url'] as String? ?? '',
    intervalSec: j['interval_sec'] as int? ?? 60,
    timeoutSec: j['timeout_sec'] as int? ?? 15,
    enabled: j['enabled'] as bool? ?? true,
    thresholds: j['thresholds'] as String? ?? '',
    diskPaths: j['disk_paths'] as String? ?? '',
    hasPrivateKey: j['has_private_key'] as bool? ?? false,
    lastOnline: j['last_online'] as bool?,
    lastError: j['last_error'] as String? ?? '',
    lastCheckedAt: j['last_checked_at'] as String? ?? '',
  );
}

/// 创建/更新入参（凭据为明文，编辑留空=不改）。
class TargetInput {
  final String name;
  final String type;
  final String host;
  final int port;
  final String vendor;
  final String protocol;
  final int credentialId;
  final String rtspUrl;
  final int intervalSec;
  final int timeoutSec;
  final String username;
  final String password;
  final String privateKey;
  final String thresholds;
  final String diskPaths;
  final bool enabled;
  TargetInput({
    required this.name,
    required this.type,
    this.host = '',
    this.port = 0,
    this.vendor = '',
    this.protocol = '',
    this.credentialId = 0,
    this.rtspUrl = '',
    this.intervalSec = 60,
    this.timeoutSec = 15,
    this.username = '',
    this.password = '',
    this.privateKey = '',
    this.thresholds = '',
    this.diskPaths = '',
    this.enabled = true,
  });

  Map<String, dynamic> toJson() => {
    'name': name,
    'type': type,
    'host': host,
    'port': port,
    'vendor': vendor,
    'protocol': protocol,
    'credential_id': credentialId,
    'rtsp_url': rtspUrl,
    'interval_sec': intervalSec,
    'timeout_sec': timeoutSec,
    'username': username,
    'password': password,
    'private_key': privateKey,
    'thresholds': thresholds,
    'disk_paths': diskPaths,
    'enabled': enabled,
  };
}
