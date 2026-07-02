import 'package:flutter/foundation.dart';

/// 安全日志：release 下静默；debug 下自动给敏感字段（token/cookie/secret/password/Bearer …）打码，
/// 防止用户 JWT / 好学 cookie 经 debugPrint 进 logcat（审计 P0-3）。
/// 全仓裸 debugPrint/print 涉及请求/认证的，统一改用 [ptLog]。
void ptLog(Object? msg) {
  if (kReleaseMode) return;
  debugPrint(_redact('$msg'));
}

/// 把字符串中的敏感凭据值打码。匹配 token=xxx / token:xxx / "token":"xxx" / Authorization: Bearer xxx。
/// 用普通字符串（非 raw）+ 双反斜杠，字符类里 \" 是字面双引号、' 用 ' 让正则引擎解析为单引号。
/// 大小写不敏感用 [RegExp.caseSensitive]=false（Dart RegExp 不支持裸 (?i) 内联标志，会抛 FormatException）。
String _redact(String s) {
  final kv = RegExp(
    "(token|authorization|secret|password|api[_-]?key|c_secure_(?:uid|pass|pass_hash|session_url))\\s*[:=]\\s*[^\\s,}\\]\"\\u0027]*",
    caseSensitive: false,
  );
  var out = s.replaceAllMapped(kv, (m) => '${m[1]}=***');
  out = out.replaceAll(RegExp(r'Bearer\s+\S+', caseSensitive: false), 'Bearer ***');
  return out;
}
