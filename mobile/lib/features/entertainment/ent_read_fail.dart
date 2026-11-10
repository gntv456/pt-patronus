// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import 'dart:async';

import 'package:flutter/material.dart';

// 娱乐面板的「读取失败」口径（Web 端 utils/entFail.ts 的同形实现）。
//
// 从前这 14 个游戏视图在每个读请求外面写着 `catch (_) {}`，还有 7 处写成
// `.catchError((_) => GameStats(balance: 0, …))`：站点凭据解不开、被墙、超时，
// 在手机上和「这个号真的没数据」长得一模一样（魔力值「—」、0 次、空列表），
// 而接口层这次已经把可照做的一句中文写进了错误信封（该换游戏 / 去重新保存凭据 /
// 花钱失败先核对余额），丢掉就等于什么都没告诉用户。

const Duration _mergeWindow = Duration(milliseconds: 1500);
const int _reasonMax = 90;

// 读数名 → 后端那句原因（取不到就留空，用兜底文案）。
final Map<String, String> _failures = <String, String>{};
Timer? _timer;

/// 从异常里取「程序写给人看的那一句」。移动端 ApiException.toString() 就是 userMessage，
/// 而后端可读消息优先于按状态码生成的通用文案（core/api_client.dart friendlyMessage）。
String _reasonOf(Object err) {
  var text = err.toString().trim();
  const prefix = 'Exception: ';
  if (text.startsWith(prefix)) text = text.substring(prefix.length);
  if (text.length > _reasonMax) text = '${text.substring(0, _reasonMax)}…';
  return text.trim();
}

/// 纯判定 + 文案：失败清单 → 一条提示；空清单回空串（调用方不出声）。
/// 抽成纯函数是为了让「读失败必须出声」这条判据可被单独翻红。
String entReadNoticeText(Map<String, String> failures) {
  if (failures.isEmpty) return '';
  final entries = failures.entries.toList();
  final head = entries
      .take(4)
      .map((e) => e.value.isEmpty ? e.key : '${e.key}（${e.value}）')
      .join('、');
  final joined = entries.length > 4 ? '$head 等 ${entries.length} 项' : head;
  return '游戏数据未能读取：$joined（显示的是空值，不代表站点真实结果）';
}

/// 记下一项没读到的数据；合并窗口内多项只汇出一条提示。
/// 同步取 messenger，避免跨 async  gap 用 BuildContext。
void entReadFailed(BuildContext context, String label, Object err) {
  final messenger = ScaffoldMessenger.maybeOf(context);
  if (messenger == null) return;
  final reason = _reasonOf(err);
  // 已记过原因的项不许被后来的空原因覆盖
  if (reason.isNotEmpty || !_failures.containsKey(label)) _failures[label] = reason;
  _timer?.cancel();
  _timer = Timer(_mergeWindow, () => _flush(messenger));
}

void _flush(ScaffoldMessengerState messenger) {
  _timer = null;
  final text = entReadNoticeText(Map<String, String>.of(_failures));
  _failures.clear();
  if (text.isEmpty) return;
  messenger.showSnackBar(SnackBar(content: Text(text)));
}

/// 测试用：丢弃未 flush 的合并窗口，避免串到下一个用例。
void resetEntReadFails() {
  _timer?.cancel();
  _timer = null;
  _failures.clear();
}
