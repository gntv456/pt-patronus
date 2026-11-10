// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'api_client.dart';

/// 任意异常 → 给用户看的一句话（#403 的单一入口）。
///
/// 为什么需要它：本仓移动端有 90+ 处 `catch (_) {}` 或 `catch (_) { _toast('…失败'); }`，
/// 写法上都无法拿到 `ApiException.userMessage`——而后端每个错误码都带着一句
/// 可操作的中文（详见 backend/internal/api/errcode/codes.go）。于是那些中文**全被丢掉**，
/// 用户只看到「刷新失败」「同步失败」。
///
/// 最典型的一处（#403 修的就是它）：`dashboard.syncOne` 在后端那条出口正是
/// 「站点凭据解不开 ⇒ 503 +『请到站点里重新保存该站点凭据；本次未向站点发请求』」
/// （#398）。移动端 `catch (_) => _toast('${s.siteName} 刷新失败')` 把这句整个吞掉，
/// 用户只知道刷新失败、不知道该去站点页重登 Cookie。
///
/// 语义：
///  - `ApiException` → 用它的 `userMessage`（后端文案优先，friendlyMessage 兜底）；
///  - 其它异常 → 不编一个原因（与后端「未知码原样带出、不许编」的同一原则），
///    返回 null，调用方继续用它自己那句兜底文案。
/// 因此它**永远只让情况变好**：拿得到真原因就用真原因，拿不到就退回调用方原有的兜底。
String? errTextOf(Object? e) {
  if (e is ApiException) {
    final m = e.userMessage.trim();
    return m.isEmpty ? null : m;
  }
  return null;
}
