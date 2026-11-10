// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// rss_account.dart —— RSS「检查」与「条目列表」的账面文案（纯函数，口径对齐 Web Rss.vue）。
import 'package:ptpatronus/api/rss.dart';

/// 点「检查」后的一句话。ok=false 必须把后端给出的中文原因说出来——
/// 回退成「新增 0 条」等于把失败渲染成「跑完了，只是没新东西」。
String rssCheckSummary(RssCheckResult r) {
  if (!r.ok) {
    final err = r.error;
    return (err == null || err.isEmpty) ? '检查失败（后端未给出原因）' : '检查失败：$err';
  }
  if (r.truncated) {
    final total = r.total;
    if (total == null) {
      return '新增 ${r.newCount} 条（本轮只检查了最近 ${r.scanned} 条，'
          '源条目总数读取失败，其余可能未查）';
    }
    return '新增 ${r.newCount} 条（源共 $total 条，本轮只检查了最近 ${r.scanned} 条，'
        '其余本轮未查）';
  }
  return '新增 ${r.newCount} 条';
}

/// 条目抽屉里的窗口提示；一次拿全时不出声。
String rssItemWindowLabel(RssItemPage p) {
  if (!p.truncated) return '';
  final total = p.total;
  if (total == null) {
    return '已显示 ${p.items.length} 条，但条目总数读取失败——可能还有更多未显示';
  }
  final rest = total - p.items.length;
  if (rest <= 0) return '';
  return '仅显示最近 ${p.items.length} 条，该订阅共 $total 条（还有 $rest 条未显示）';
}
