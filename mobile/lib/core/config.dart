// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

/// 后端地址配置。
///
/// 通过 `--dart-define=API_BASE_URL=...` 覆盖：
/// - 桌面（macOS/Windows/Linux/Web）：默认 http://localhost:8088
/// - Android 模拟器：用 http://10.0.2.2:8088
/// - 真机：用局域网 IP，如 http://192.168.1.10:8088
class Config {
  static const String baseUrl = String.fromEnvironment(
    'API_BASE_URL',
    defaultValue: 'http://localhost:8088',
  );

  static const String appName = 'PTPatronus';
}
