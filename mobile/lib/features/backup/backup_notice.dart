// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

import '../../api/backup.dart';

/// 明细最多显示几条（计数不受限，和后端 maxImportFailures=10 同量级，别刷屏）。
const importFailureShownLimit = 3;

/// 这次导入到底成了什么：返回给用户看的那一句（纯函数，便于用例）。
/// 对齐 Web `Backup.vue` 的口径：「导入多少」「跳过/失败多少」「这份压根没内容」
/// 是三件不同的事，塌成一句「导入完成」就是让用户体验到一次假恢复（#256）。
/// [withDownloaders]=false 用于 MoviePilot / .ptd 这两条只导站点的路径
/// （后端不给下载器维度，写出来就是「下载器 0」这种误导性数字）。
String importOutcomeNotice(ImportResult r, {bool withDownloaders = true}) {
  final counts = withDownloaders
      ? '站点 ${r.sites} / 下载器 ${r.downloaders}'
      : '站点 ${r.sites}';
  // empty 是后端算的：一个都没导进来（选错文件、空备份、格式不匹配都会回 0）。
  if (r.empty) return '这份备份没有可导入的内容（$counts）；请确认选的是本程序的备份文件';

  final parts = <String>['导入：$counts'];
  final skipped = r.sitesSkipped;
  final failed = r.downloadersFailed;
  if (skipped > 0) parts.add('跳过 $skipped 个站点（同名已存在）');
  if (failed > 0) parts.add('$failed 个下载器创建失败');
  if (r.failures.isNotEmpty) {
    final shown = r.failures.take(importFailureShownLimit).join('；');
    final more = r.failures.length > importFailureShownLimit
        ? '（共 ${r.failures.length} 条）'
        : '';
    parts.add('原因：$shown$more');
  }
  // 只有「没跳过、没失败」才配得上「完成」两个字。
  if (skipped == 0 && failed == 0 && r.failures.isEmpty) {
    return '导入完成：$counts';
  }
  return parts.join('，');
}
